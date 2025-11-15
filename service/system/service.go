package system

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

const (
	publicKeyConfigKey  = "crypto.public_key"
	privateKeyConfigKey = "crypto.private_key"
	defaultRSAKeySize   = 2048

	chunkedSecretPrefix    = "chunked:v1:"
	chunkedSecretSeparator = "::"
)

// Service manages system level configuration such as RSA key pairs.
type Service struct {
	db         *store.DB
	mu         sync.RWMutex
	publicKey  string
	privateKey *rsa.PrivateKey
}

func New(db *store.DB) (*Service, error) {
	svc := &Service{db: db}
	if err := svc.ensureKeyPair(context.Background()); err != nil {
		return nil, err
	}
	return svc, nil
}

// GetPublicKey returns the PEM encoded public key.
func (s *Service) GetPublicKey(ctx context.Context) (string, error) {
	if err := s.ensureKeyPair(ctx); err != nil {
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publicKey, nil
}

// DecryptString decrypts a base64 encoded ciphertext using the private key.
func (s *Service) DecryptString(ctx context.Context, cipherText string) (string, error) {
	if cipherText == "" {
		return "", nil
	}
	if err := s.ensureKeyPair(ctx); err != nil {
		return "", err
	}

	s.mu.RLock()
	priv := s.privateKey
	s.mu.RUnlock()

	if priv == nil {
		return "", fmt.Errorf("private key is not initialized")
	}

	raw, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", fmt.Errorf("decode cipher text: %w", err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, priv, raw)
	if err != nil {
		return "", fmt.Errorf("rsa decrypt: %w", err)
	}
	return string(plain), nil
}

func (s *Service) decryptSecretValue(ctx context.Context, cipherText string) (string, error) {
	if cipherText == "" {
		return "", nil
	}
	if strings.HasPrefix(cipherText, chunkedSecretPrefix) {
		payload := strings.TrimPrefix(cipherText, chunkedSecretPrefix)
		if payload == "" {
			return "", nil
		}
		parts := strings.Split(payload, chunkedSecretSeparator)
		var builder strings.Builder
		for _, part := range parts {
			if part == "" {
				continue
			}
			plain, err := s.DecryptString(ctx, part)
			if err != nil {
				return "", err
			}
			builder.WriteString(plain)
		}
		return builder.String(), nil
	}
	return s.DecryptString(ctx, cipherText)
}

func (s *Service) ensureKeyPair(ctx context.Context) error {
	s.mu.RLock()
	if s.privateKey != nil && s.publicKey != "" {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// double check after locking
	if s.privateKey != nil && s.publicKey != "" {
		return nil
	}

	configs := make(map[string]string)
	if err := s.db.View(func(tx *gorm.DB) error {
		var rows []model.ServerConfig
		if err := tx.WithContext(ctx).
			Where("`key` IN ?", []string{publicKeyConfigKey, privateKeyConfigKey}).
			Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			configs[row.Key] = row.Value
		}
		return nil
	}); err != nil {
		return err
	}

	pubPEM := configs[publicKeyConfigKey]
	privPEM := configs[privateKeyConfigKey]

	if pubPEM == "" || privPEM == "" {
		newPriv, newPub, err := generateKeyPair()
		if err != nil {
			return err
		}
		if err := s.persistKeyPair(ctx, newPub, newPriv); err != nil {
			return err
		}
		privPEM = newPriv
		pubPEM = newPub
	}

	privateKey, err := parseRSAPrivateKey(privPEM)
	if err != nil {
		return err
	}

	s.privateKey = privateKey
	s.publicKey = pubPEM
	return nil
}

func (s *Service) persistKeyPair(ctx context.Context, publicPEM, privatePEM string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		pub := model.ServerConfig{Key: publicKeyConfigKey, Value: publicPEM}
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value"}),
			}).Create(&pub).Error; err != nil {
			return err
		}

		priv := model.ServerConfig{Key: privateKeyConfigKey, Value: privatePEM}
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value"}),
			}).Create(&priv).Error; err != nil {
			return err
		}
		return nil
	})
}

func generateKeyPair() (privatePEM, publicPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, defaultRSAKeySize)
	if err != nil {
		return "", "", fmt.Errorf("generate rsa key: %w", err)
	}

	privBytes := x509.MarshalPKCS1PrivateKey(key)
	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	}
	privatePEM = string(pem.EncodeToMemory(privBlock))

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal public key: %w", err)
	}
	pubBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}
	publicPEM = string(pem.EncodeToMemory(pubBlock))
	return privatePEM, publicPEM, nil
}

func parseRSAPrivateKey(privatePEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return nil, fmt.Errorf("invalid private key pem")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported private key type %s", block.Type)
	}
}

// ListCertificates returns certificates matching the provided filters.
func (s *Service) ListCertificates(ctx context.Context, opts model.ListOptions, filter model.CertificateFilter) ([]*model.Certificate, int64, error) {
	var (
		page    = opts.Page
		perPage = opts.PerPage
	)

	if opts.All {
		page = 1
		perPage = 0
	}
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 && !opts.All {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	var (
		certificates []*model.Certificate
		total        int64
	)

	err := s.db.View(func(tx *gorm.DB) error {
		query := tx.WithContext(ctx).Model(&model.Certificate{})

		if t := strings.TrimSpace(filter.Type); t != "" {
			query = query.Where("type = ?", t)
		}
		if name := strings.TrimSpace(filter.Name); name != "" {
			like := "%" + name + "%"
			query = query.Where("name LIKE ?", like)
		}

		if err := query.Count(&total).Error; err != nil {
			return err
		}

		query = query.Order("updated DESC")
		if !opts.All && perPage > 0 {
			query = query.Offset((page - 1) * perPage).Limit(perPage)
		}
		return query.Find(&certificates).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return certificates, total, nil
}

// GetCertificate fetches a certificate by id.
func (s *Service) GetCertificate(ctx context.Context, id int64) (*model.Certificate, error) {
	var cert model.Certificate
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&cert, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// GetCertificateWithSecrets returns the certificate with decrypted sensitive fields.
func (s *Service) GetCertificateWithSecrets(ctx context.Context, id int64) (*model.Certificate, error) {
	cert, err := s.GetCertificate(ctx, id)
	if err != nil || cert == nil {
		return cert, err
	}
	config, err := s.decryptSensitiveConfig(ctx, cert.Config)
	if err != nil {
		return nil, err
	}
	clone := cert.Clone()
	clone.Config = config
	return clone, nil
}

// GetCertificateByName fetches a certificate by name (case-insensitive).
func (s *Service) GetCertificateByName(ctx context.Context, name string) (*model.Certificate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}

	var cert model.Certificate
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("LOWER(name) = ?", strings.ToLower(name)).
			Take(&cert).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// GetCertificateWithSecretsByName fetches a certificate by name and decrypts sensitive values.
func (s *Service) GetCertificateWithSecretsByName(ctx context.Context, name string) (*model.Certificate, error) {
	cert, err := s.GetCertificateByName(ctx, name)
	if err != nil || cert == nil {
		return cert, err
	}
	config, err := s.decryptSensitiveConfig(ctx, cert.Config)
	if err != nil {
		return nil, err
	}
	clone := cert.Clone()
	clone.Config = config
	return clone, nil
}

// CreateCertificate persists a new certificate record after validating sensitive fields.
func (s *Service) CreateCertificate(ctx context.Context, cert *model.Certificate) (*model.Certificate, error) {
	if cert == nil {
		return nil, fmt.Errorf("certificate is nil")
	}
	if cert.Config == nil {
		cert.Config = map[string]interface{}{}
	}

	cert.Name = strings.TrimSpace(cert.Name)
	cert.Type = strings.TrimSpace(cert.Type)

	if cert.Name == "" {
		return nil, fmt.Errorf("certificate name is required")
	}
	if cert.Type == "" {
		return nil, fmt.Errorf("certificate type is required")
	}

	sanitizedConfig, err := s.normalizeConfigForStorage(ctx, cert.Config, false)
	if err != nil {
		return nil, err
	}
	cert.Config = sanitizedConfig

	now := time.Now().Unix()
	cert.Created = now
	cert.Updated = now

	err = s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Create(cert).Error
	})
	if err != nil {
		return nil, err
	}
	return cert, nil
}

// UpdateCertificate updates mutable fields of a certificate.
func (s *Service) UpdateCertificate(ctx context.Context, id int64, patch model.CertificatePatch) (*model.Certificate, error) {
	var updated *model.Certificate

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cert model.Certificate
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&cert, id).Error; err != nil {
			return err
		}

		if patch.Name != nil {
			name := strings.TrimSpace(*patch.Name)
			if name == "" {
				return fmt.Errorf("certificate name is required")
			}
			cert.Name = name
		}
		if patch.Type != nil {
			typ := strings.TrimSpace(*patch.Type)
			if typ == "" {
				return fmt.Errorf("certificate type is required")
			}
			cert.Type = typ
		}
		if patch.Config != nil {
			sanitized, err := s.normalizeConfigForStorage(ctx, patch.Config, true)
			if err != nil {
				return err
			}
			cert.MergeConfig(sanitized)
		}

		cert.Updated = time.Now().Unix()

		if err := tx.WithContext(ctx).Save(&cert).Error; err != nil {
			return err
		}
		updated = cert.Clone()
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteCertificate removes a certificate by id.
func (s *Service) DeleteCertificate(ctx context.Context, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Delete(&model.Certificate{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Service) decryptSensitiveConfig(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	if len(config) == 0 {
		return map[string]interface{}{}, nil
	}
	result := make(map[string]interface{}, len(config))
	for key, val := range config {
		if model.IsSensitiveConfigKey(key) {
			strVal, ok := val.(string)
			if ok && strVal != "" {
				plain, err := s.decryptSecretValue(ctx, strVal)
				if err != nil {
					return nil, fmt.Errorf("decrypt %s: %w", key, err)
				}
				result[key] = plain
				continue
			}
		}
		result[key] = val
	}
	return result, nil
}

func (s *Service) normalizeSecretValue(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if _, err := s.decryptSecretValue(ctx, value); err == nil {
		return value, nil
	}
	return s.encryptSecretValue(ctx, value)
}

func (s *Service) encryptSecretValue(ctx context.Context, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if err := s.ensureKeyPair(ctx); err != nil {
		return "", err
	}
	s.mu.RLock()
	pub := &s.privateKey.PublicKey
	s.mu.RUnlock()
	maxChunk := pub.N.BitLen()/8 - 11
	if maxChunk <= 0 {
		return "", fmt.Errorf("invalid rsa key")
	}
	data := []byte(plain)
	if len(data) <= maxChunk {
		return encryptChunk(pub, data)
	}
	parts := make([]string, 0, (len(data)+maxChunk-1)/maxChunk)
	for len(data) > 0 {
		chunk := data
		if len(chunk) > maxChunk {
			chunk = data[:maxChunk]
		}
		cipher, err := encryptChunk(pub, chunk)
		if err != nil {
			return "", err
		}
		parts = append(parts, cipher)
		if len(data) > maxChunk {
			data = data[maxChunk:]
		} else {
			break
		}
	}
	return chunkedSecretPrefix + strings.Join(parts, chunkedSecretSeparator), nil
}

func encryptChunk(pub *rsa.PublicKey, chunk []byte) (string, error) {
	cipher, err := rsa.EncryptPKCS1v15(rand.Reader, pub, chunk)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cipher), nil
}

func (s *Service) normalizeConfigForStorage(ctx context.Context, config map[string]interface{}, skipEmptySecrets bool) (map[string]interface{}, error) {
	if len(config) == 0 {
		return map[string]interface{}{}, nil
	}
	sanitized := make(map[string]interface{}, len(config))
	for key, val := range config {
		if model.IsSensitiveConfigKey(key) {
			if val == nil {
				if skipEmptySecrets {
					continue
				}
				return nil, fmt.Errorf("%s is required", key)
			}
			strVal, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be string", key)
			}
			trimmed := strings.TrimSpace(strVal)
			switch {
			case trimmed == "":
				if skipEmptySecrets {
					continue
				}
				return nil, fmt.Errorf("%s is required", key)
			case trimmed == model.DefaultSecretMask:
				if skipEmptySecrets {
					continue
				}
				return nil, fmt.Errorf("%s value is invalid", key)
			}
			encrypted, err := s.normalizeSecretValue(ctx, trimmed)
			if err != nil {
				return nil, fmt.Errorf("encrypt %s: %w", key, err)
			}
			sanitized[key] = encrypted
			continue
		}
		switch v := val.(type) {
		case string:
			sanitized[key] = strings.TrimSpace(v)
		default:
			sanitized[key] = v
		}
	}
	return sanitized, nil
}
