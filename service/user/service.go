package user

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"gorm.io/gorm"

	"github.com/kuzane/go-devops/internal/store"
	"github.com/kuzane/go-devops/model"
)

// Service encapsulates user related business logic.
type Service struct {
	db *store.DB
}

func New(db *store.DB) *Service {
	return &Service{db: db}
}

// Create persists a new user record.
func (s *Service) Create(ctx context.Context, user *model.User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Create(user).Error
	})
}

// FindByID retrieves a user by id.
func (s *Service) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&user, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByLogin retrieves a user by login (case sensitive).
func (s *Service) FindByLogin(ctx context.Context, login string) (*model.User, error) {
	var user model.User
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Where("login = ?", login).Take(&user).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update persists changes to a user.
func (s *Service) Update(ctx context.Context, user *model.User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Model(&model.User{}).Where("id = ?", user.ID).Updates(user).Error
	})
}

// List returns all users.
func (s *Service) List(ctx context.Context) ([]*model.User, error) {
	var users []*model.User
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Find(&users).Error
	}); err != nil {
		return nil, err
	}
	return users, nil
}

type GitUser struct {
	RemoteID string
	Login    string
	Email    string
	Avatar   string
	IsAdmin  bool
}

func (s *Service) UpsertGitUser(ctx context.Context, forgeID int64, info GitUser, token *oauth2.Token) (*model.User, error) {
	if info.RemoteID == "" {
		return nil, errors.New("git user remote id is empty")
	}
	if info.Login == "" {
		return nil, errors.New("git user login is empty")
	}

	remoteID := model.ForgeRemoteID(info.RemoteID)
	accessToken := ""
	refreshToken := ""
	expiry := int64(0)
	if token != nil {
		accessToken = token.AccessToken
		refreshToken = token.RefreshToken
		if !token.Expiry.IsZero() {
			expiry = token.Expiry.Unix()
		}
	}

	var result *model.User
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.User
		err := tx.WithContext(ctx).Where("forge_id = ? AND forge_remote_id = ?", forgeID, remoteID).Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			newUser := &model.User{
				ForgeID:       forgeID,
				ForgeRemoteID: remoteID,
				Login:         info.Login,
				Email:         info.Email,
				Avatar:        info.Avatar,
				AccessToken:   accessToken,
				RefreshToken:  refreshToken,
				Expiry:        expiry,
				Admin:         info.IsAdmin,
				Hash:          generateUserHash(),
			}
			if err := tx.WithContext(ctx).Create(newUser).Error; err != nil {
				return err
			}
			result = newUser
			return nil
		case err != nil:
			return err
		default:
			update := map[string]any{
				"login":         info.Login,
				"email":         info.Email,
				"avatar":        info.Avatar,
				"access_token":  accessToken,
				"refresh_token": refreshToken,
				"expiry":        expiry,
				"admin":         info.IsAdmin,
			}
			if err := tx.WithContext(ctx).Model(&existing).Updates(update).Error; err != nil {
				return err
			}
			existing.Login = info.Login
			existing.Email = info.Email
			existing.Avatar = info.Avatar
			existing.AccessToken = accessToken
			existing.RefreshToken = refreshToken
			existing.Expiry = expiry
			existing.Admin = info.IsAdmin
			result = &existing
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func generateUserHash() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
