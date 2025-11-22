package repo

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

type Service struct {
	db *store.DB
}

func New(db *store.DB) *Service {
	return &Service{db: db}
}

// Create registers a repository.
func (s *Service) Create(ctx context.Context, repo *model.Repo) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Create(repo).Error
	})
}

// Update applies changes to a repository.
func (s *Service) Update(ctx context.Context, repo *model.Repo) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Model(&model.Repo{}).Where("id = ?", repo.ID).Updates(repo).Error
	})
}

// FindByID fetches a repository by numeric id.
func (s *Service) FindByID(ctx context.Context, id int64) (*model.Repo, error) {
	var repo model.Repo
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&repo, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// FindByFullName fetches a repository by owner/name.
func (s *Service) FindByFullName(ctx context.Context, owner, name string) (*model.Repo, error) {
	var repo model.Repo
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("owner = ? AND name = ?", owner, name).
			Take(&repo).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// ListByUser returns repositories the user has access to.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]*model.Repo, error) {
	repos, _, err := s.ListByUserPaged(ctx, userID, model.ListOptions{Page: 1, PerPage: 1000}, "", nil)
	return repos, err
}

func (s *Service) ListByUserPaged(ctx context.Context, userID int64, opts model.ListOptions, search string, active *bool) ([]*model.Repo, int64, error) {
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 20
	} else if perPage > 100 {
		perPage = 100
	}

	query := s.db.GetDB().WithContext(ctx).Model(&model.Repo{}).Where("user_id = ?", userID)
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("full_name LIKE ? OR name LIKE ?", like, like)
	}
	if active != nil {
		query = query.Where("active = ?", *active)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var repos []*model.Repo
	if err := query.Order("full_name ASC").Offset((page - 1) * perPage).Limit(perPage).Find(&repos).Error; err != nil {
		return nil, 0, err
	}

	return repos, total, nil
}

type GitRepository struct {
	RemoteID      string
	Owner         string
	Name          string
	FullName      string
	AvatarURL     string
	WebURL        string
	HTTPCloneURL  string
	SSHCloneURL   string
	DefaultBranch string
	Visibility    model.RepoVisibility
	IsPrivate     bool
	ConfigPath    string
}

// SyncGitRepositories upserts repositories reported by an external forge.
// When activate is true the repositories are marked as active; otherwise the
// activation status is preserved (new repositories default to inactive).
func (s *Service) SyncGitRepositories(ctx context.Context, forgeID, userID int64, repositories []GitRepository, activate bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, repository := range repositories {
			if repository.RemoteID == "" {
				continue
			}

			remoteID := model.ForgeRemoteID(repository.RemoteID)
			trusted := model.TrustedConfiguration{}

			var existing model.Repo
			err := tx.WithContext(ctx).Where("forge_id = ? AND forge_remote_id = ?", forgeID, remoteID).Take(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = tx.WithContext(ctx).
					Where("forge_id = ? AND owner = ? AND name = ?", forgeID, repository.Owner, repository.Name).
					Take(&existing).Error
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			if existing.ID == 0 {
				newRepo := &model.Repo{
					ForgeID:                      forgeID,
					ForgeRemoteID:                remoteID,
					UserID:                       userID,
					OrgID:                        0,
					Owner:                        repository.Owner,
					Name:                         repository.Name,
					FullName:                     repository.FullName,
					Avatar:                       repository.AvatarURL,
					ForgeURL:                     repository.WebURL,
					Clone:                        repository.HTTPCloneURL,
					CloneSSH:                     repository.SSHCloneURL,
					Branch:                       repository.DefaultBranch,
					Visibility:                   repository.Visibility,
					IsSCMPrivate:                 repository.IsPrivate,
					PREnabled:                    true,
					Timeout:                      0,
					IsActive:                     activate,
					AllowPull:                    true,
					AllowDeploy:                  true,
					Config:                       repository.ConfigPath,
					Trusted:                      trusted,
					RequireApproval:              model.RequireApprovalForks,
					CancelPreviousPipelineEvents: []model.WebhookEvent{},
					NetrcTrustedPlugins:          []string{},
					Hash:                         generateRepoHash(),
				}
				if err := tx.WithContext(ctx).Create(newRepo).Error; err != nil {
					return err
				}
				continue
			}

			existing.UserID = userID
			existing.ForgeRemoteID = remoteID
			existing.Owner = repository.Owner
			existing.Name = repository.Name
			existing.FullName = repository.FullName
			existing.Avatar = repository.AvatarURL
			existing.ForgeURL = repository.WebURL
			existing.Clone = repository.HTTPCloneURL
			existing.CloneSSH = repository.SSHCloneURL
			existing.Branch = repository.DefaultBranch
			existing.Visibility = repository.Visibility
			existing.IsSCMPrivate = repository.IsPrivate
			existing.PREnabled = true
			existing.Timeout = 0
			if activate {
				existing.IsActive = true
			}
			existing.AllowPull = true
			existing.AllowDeploy = true
			existing.Config = repository.ConfigPath
			existing.Trusted = trusted
			existing.RequireApproval = model.RequireApprovalForks
			existing.CancelPreviousPipelineEvents = []model.WebhookEvent{}
			existing.NetrcTrustedPlugins = []string{}
			existing.ConfigExtensionEndpoint = ""

			if err := tx.WithContext(ctx).Save(&existing).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func generateRepoHash() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
