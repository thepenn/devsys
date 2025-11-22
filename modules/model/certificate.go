package model

import (
	"fmt"
	"strings"

	"github.com/mitchellh/mapstructure"
)

type Certificate struct {
	ID      int64                  `json:"id"        gorm:"column:id;primaryKey;autoIncrement"`
	Name    string                 `json:"name"      gorm:"column:name;size:191;index"`
	Type    string                 `json:"type"      gorm:"column:type;size:64;index"`
	Config  map[string]interface{} `json:"config"    gorm:"column:config;serializer:json"`
	Created int64                  `json:"created"   gorm:"column:created"`
	Updated int64                  `json:"updated"   gorm:"column:updated"`
}

func (Certificate) TableName() string {
	return "certificates"
}

// GitCertificate captures the canonical fields we expect for Git authentication.
type GitCertificate struct {
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// DockerCertificate captures docker registry credentials.
type DockerCertificate struct {
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
	Repo     string `json:"repo"`
}

// MySQLCertificate holds DSN style configuration.
type MySQLCertificate struct {
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
	Port     string `json:"port"`
	Host     string `json:"host"`
	Database string `json:"database"`
}

// LDAPCertificate represents the LDAP configuration snippet.
type LDAPCertificate struct {
	Type         string `json:"type"`
	Server       string `json:"server"`
	Port         int    `json:"port"`
	BaseDN       string `json:"base_dn"`
	SearchBaseDN string `json:"search_base_dn"`
	BindDN       string `json:"bind_dn"`
	Password     string `json:"password"`
	UserAttr     string `json:"user_attr"`
	EmailAttr    string `json:"email_attr"`
	GroupAttr    string `json:"group_attr"`
}

// KubernetesCertificate stores kubeconfig content for a cluster.
type KubernetesCertificate struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Server     string `json:"server"`
	KubeConfig string `json:"kubeconfig"`
}

func (c *Certificate) decode(target interface{}) error {
	if c == nil {
		return fmt.Errorf("certificate is nil")
	}
	return mapstructure.Decode(c.Config, target)
}

func (c *Certificate) AsGitCertificate() (*GitCertificate, error) {
	if c.Type != "git" {
		return nil, fmt.Errorf("certificate type %s is not git", c.Type)
	}
	var git GitCertificate
	if err := c.decode(&git); err != nil {
		return nil, err
	}
	if git.Type == "" {
		git.Type = c.Type
	}
	return &git, nil
}

func (c *Certificate) AsDockerCertificate() (*DockerCertificate, error) {
	if c.Type != "docker" {
		return nil, fmt.Errorf("certificate type %s is not docker", c.Type)
	}
	var docker DockerCertificate
	if err := c.decode(&docker); err != nil {
		return nil, err
	}
	if docker.Type == "" {
		docker.Type = c.Type
	}
	return &docker, nil
}

func (c *Certificate) AsMySQLCertificate() (*MySQLCertificate, error) {
	if c.Type != "mysql" {
		return nil, fmt.Errorf("certificate type %s is not mysql", c.Type)
	}
	var mysql MySQLCertificate
	if err := c.decode(&mysql); err != nil {
		return nil, err
	}
	if mysql.Type == "" {
		mysql.Type = c.Type
	}
	return &mysql, nil
}

func (c *Certificate) AsLDAPCertificate() (*LDAPCertificate, error) {
	if c.Type != "ldap" {
		return nil, fmt.Errorf("certificate type %s is not ldap", c.Type)
	}
	var ldap LDAPCertificate
	if err := c.decode(&ldap); err != nil {
		return nil, err
	}
	if ldap.Type == "" {
		ldap.Type = c.Type
	}
	return &ldap, nil
}

// AsKubernetesCertificate decodes the certificate config into KubernetesCertificate.
func (c *Certificate) AsKubernetesCertificate() (*KubernetesCertificate, error) {
	if c.Type != CertificateTypeKubernetes {
		return nil, fmt.Errorf("certificate type %s is not kubernetes", c.Type)
	}
	var kube KubernetesCertificate
	if err := c.decode(&kube); err != nil {
		return nil, err
	}
	if kube.Type == "" {
		kube.Type = c.Type
	}
	if kube.Name == "" {
		kube.Name = c.Name
	}
	return &kube, nil
}

// CertificateFilter captures optional filters for listing certificates.
type CertificateFilter struct {
	Type string
	Name string
}

// CertificatePatch contains mutable fields for certificate update.
type CertificatePatch struct {
	Name   *string                `json:"name,omitempty"`
	Type   *string                `json:"type,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}

const (
	// DefaultSecretMask specifies the value used when hiding secrets.
	DefaultSecretMask = "******"
	// CertificateTypeKubernetes denotes a kubernetes cluster credential.
	CertificateTypeKubernetes = "kubernetes"
)

var sensitiveConfigKeys = map[string]struct{}{
	"password":       {},
	"token":          {},
	"access_token":   {},
	"refresh_token":  {},
	"secret":         {},
	"client_secret":  {},
	"private_key":    {},
	"ssh_key":        {},
	"api_key":        {},
	"auth_token":     {},
	"bearer_token":   {},
	"credentials":    {},
	"secret_key":     {},
	"secret_token":   {},
	"service_token":  {},
	"registry_token": {},
	"kubeconfig":     {},
}

// IsSensitiveConfigKey returns true if the key is classified as sensitive.
func IsSensitiveConfigKey(key string) bool {
	if key == "" {
		return false
	}
	_, ok := sensitiveConfigKeys[strings.ToLower(key)]
	return ok
}

// Clone creates a shallow copy of the certificate with a deep copy of the config map.
func (c *Certificate) Clone() *Certificate {
	if c == nil {
		return nil
	}
	copy := *c
	copy.Config = cloneConfigMap(c.Config)
	return &copy
}

// MaskSecrets returns a copy of the configuration map with sensitive fields replaced by mask.
// It also returns the list of keys that were masked.
func (c *Certificate) MaskSecrets(mask string) (map[string]interface{}, []string) {
	return MaskSensitiveConfig(c.Config, mask)
}

// MergeConfig merges the provided values into the certificate configuration.
// Nil values remove the corresponding key.
func (c *Certificate) MergeConfig(values map[string]interface{}) {
	if c.Config == nil {
		c.Config = map[string]interface{}{}
	}
	for key, val := range values {
		if val == nil {
			delete(c.Config, key)
			continue
		}
		c.Config[key] = val
	}
}

// MaskSensitiveConfig replaces sensitive keys with the provided mask and returns the masked map
// along with the list of keys that were masked.
func MaskSensitiveConfig(config map[string]interface{}, mask string) (map[string]interface{}, []string) {
	if len(config) == 0 {
		return map[string]interface{}{}, nil
	}
	if mask == "" {
		mask = DefaultSecretMask
	}
	masked := make(map[string]interface{}, len(config))
	var maskedKeys []string
	for key, val := range config {
		if IsSensitiveConfigKey(key) {
			switch v := val.(type) {
			case string:
				if v != "" && mask != "" {
					masked[key] = mask
				} else {
					masked[key] = v
				}
			case nil:
				masked[key] = nil
			default:
				if mask != "" {
					masked[key] = mask
				} else {
					masked[key] = v
				}
			}
			maskedKeys = append(maskedKeys, key)
			continue
		}
		masked[key] = val
	}
	return masked, maskedKeys
}

func cloneConfigMap(config map[string]interface{}) map[string]interface{} {
	if len(config) == 0 {
		return map[string]interface{}{}
	}
	clone := make(map[string]interface{}, len(config))
	for key, val := range config {
		clone[key] = val
	}
	return clone
}
