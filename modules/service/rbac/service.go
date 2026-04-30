// Package rbac is the management-API service layer for the role / label
// catalog. It owns CRUD operations and triggers the gorbac engine rebuild
// after every mutation so the in-memory graph stays consistent with the DB.
package rbac

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	internalauth "github.com/thepenn/devsys/internal/auth"
	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
	messageService "github.com/thepenn/devsys/service/message"
	userService "github.com/thepenn/devsys/service/user"
)

// Service exposes role / label / endpoint / user-role CRUD.
type Service struct {
	db       *store.DB
	rbac     *internalauth.RBAC
	users    *userService.Service
	messages *messageService.Service
}

// New constructs a Service. The RBAC engine is required so mutations can
// rebuild the in-memory graph; users provides the canonical SetUserRoles
// implementation that AssignUserRoles delegates to (gives us cache invalidation
// for free instead of duplicating the diff loop). messages is optional —
// when non-nil, AssignUserRoles emits an in-app notification to the affected
// user so they know their permissions changed.
func New(
	db *store.DB,
	engine *internalauth.RBAC,
	users *userService.Service,
	messages *messageService.Service,
) *Service {
	return &Service{db: db, rbac: engine, users: users, messages: messages}
}

// RoleInput is the payload accepted by CreateRole / UpdateRole.
type RoleInput struct {
	Name       string   `json:"name"`
	Title      string   `json:"title"`
	Parents    []string `json:"parents"`
	LabelNames []string `json:"labels"`
}

// ListRoles returns every role with its labels populated.
func (s *Service) ListRoles(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Preload("Labels").Order("id ASC").Find(&roles).Error
	})
	return roles, err
}

// GetRole fetches a single role with labels.
func (s *Service) GetRole(ctx context.Context, id int64) (*model.Role, error) {
	var role model.Role
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Preload("Labels").First(&role, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// CreateRole inserts a new role and its label bindings.
func (s *Service) CreateRole(ctx context.Context, in RoleInput) (*model.Role, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("role name is required")
	}
	if label.IsBuiltinRole(name) {
		return nil, fmt.Errorf("role name %q conflicts with built-in role", name)
	}
	now := time.Now().Unix()
	role := model.Role{
		Name:    name,
		Title:   strings.TrimSpace(in.Title),
		Parents: joinUnique(in.Parents),
		Builtin: false,
		Created: now,
		Updated: now,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(&role).Error; err != nil {
			return err
		}
		return s.replaceRoleLabelsTx(tx.WithContext(ctx), role.ID, in.LabelNames)
	})
	if err != nil {
		return nil, err
	}
	if err := s.rbac.Rebuild(ctx); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, role.ID)
}

// UpdateRole modifies title / parents / label bindings. Built-in roles allow
// all fields except `name` to change.
func (s *Service) UpdateRole(ctx context.Context, id int64, in RoleInput) (*model.Role, error) {
	now := time.Now().Unix()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.Role
		if err := tx.WithContext(ctx).First(&existing, id).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"updated": now,
		}
		if title := strings.TrimSpace(in.Title); title != "" {
			updates["title"] = title
		}
		updates["parents"] = joinUnique(in.Parents)

		if !existing.Builtin {
			if name := strings.TrimSpace(in.Name); name != "" && name != existing.Name {
				if label.IsBuiltinRole(name) {
					return fmt.Errorf("role name %q conflicts with built-in role", name)
				}
				updates["name"] = name
			}
		}

		if err := tx.WithContext(ctx).Model(&model.Role{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return s.replaceRoleLabelsTx(tx.WithContext(ctx), id, in.LabelNames)
	})
	if err != nil {
		return nil, err
	}
	if err := s.rbac.Rebuild(ctx); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, id)
}

// DeleteRole removes a role and any user/role bindings. Built-in roles are
// protected and cannot be removed.
func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.Role
		if err := tx.WithContext(ctx).First(&existing, id).Error; err != nil {
			return err
		}
		if existing.Builtin {
			return errors.New("built-in roles cannot be deleted")
		}
		if err := tx.WithContext(ctx).Exec("DELETE FROM role_labels WHERE role_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("role_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).Delete(&model.Role{}, id).Error
	})
	if err != nil {
		return err
	}
	return s.rbac.Rebuild(ctx)
}

// ListLabels returns the label catalog ordered by module + name.
func (s *Service) ListLabels(ctx context.Context) ([]model.Label, error) {
	var labels []model.Label
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Order("module ASC, name ASC").Find(&labels).Error
	})
	return labels, err
}

// ListEndpoints returns the auto-synced endpoint catalog.
func (s *Service) ListEndpoints(ctx context.Context) ([]model.Endpoint, error) {
	var endpoints []model.Endpoint
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Preload("Labels").Order("module ASC, path ASC, method ASC").Find(&endpoints).Error
	})
	return endpoints, err
}

// UserRoleAssignment is a single user's role membership snapshot.
type UserRoleAssignment struct {
	UserID    int64    `json:"user_id"`
	Login     string   `json:"login"`
	Email     string   `json:"email"`
	Avatar    string   `json:"avatar_url"`
	Admin     bool     `json:"admin"`
	RoleNames []string `json:"roles"`
}

// ListUserRoles returns every user with their role names attached.
func (s *Service) ListUserRoles(ctx context.Context) ([]UserRoleAssignment, error) {
	var users []model.User
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Order("id ASC").Find(&users).Error
	}); err != nil {
		return nil, err
	}

	type userRoleRow struct {
		UserID   int64
		RoleName string
	}
	var rows []userRoleRow
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Table("user_roles").
			Select("user_roles.user_id AS user_id, roles.name AS role_name").
			Joins("JOIN roles ON roles.id = user_roles.role_id").
			Find(&rows).Error
	}); err != nil {
		return nil, err
	}

	bucket := make(map[int64][]string, len(users))
	for _, r := range rows {
		bucket[r.UserID] = append(bucket[r.UserID], r.RoleName)
	}
	out := make([]UserRoleAssignment, 0, len(users))
	for _, u := range users {
		names := bucket[u.ID]
		sort.Strings(names)
		out = append(out, UserRoleAssignment{
			UserID:    u.ID,
			Login:     u.Login,
			Email:     u.Email,
			Avatar:    u.Avatar,
			Admin:     u.Admin,
			RoleNames: names,
		})
	}
	return out, nil
}

// AssignUserRoles delegates to user.Service.SetUserRoles. The user service
// owns the diff loop + role cache invalidation; rbac just exposes it through
// the management API surface (transitive role->label graph is unchanged so
// no gorbac rebuild is needed).
//
// 副作用: 成功后给被改动的用户发一条站内消息. 通知失败不影响主操作.
func (s *Service) AssignUserRoles(ctx context.Context, userID int64, roleNames []string) error {
	if s.users == nil {
		return errors.New("user service not configured")
	}
	if err := s.users.SetUserRoles(ctx, userID, roleNames); err != nil {
		return err
	}
	s.notifyRoleChange(ctx, userID, roleNames)
	return nil
}

// notifyRoleChange writes an info-level Message addressed to the user.
// We swallow the error: a missing message is far less serious than failing
// the parent role-assignment HTTP request after the DB has already committed.
func (s *Service) notifyRoleChange(ctx context.Context, userID int64, roleNames []string) {
	if s.messages == nil || userID == 0 {
		return
	}
	display := strings.Join(roleNames, ", ")
	if display == "" {
		display = "(无角色)"
	}
	_, err := s.messages.Create(ctx, messageService.CreateInput{
		UserID:  userID,
		Type:    model.MessageTypeInfo,
		Source:  model.MessageSourceRBAC,
		Title:   "您的角色已更新",
		Content: fmt.Sprintf("当前角色: %s", display),
	})
	if err != nil {
		log.Warn().Err(err).Int64("user_id", userID).Msg("rbac: failed to emit role-change notification")
	}
}

// replaceRoleLabelsTx clears role_labels for `roleID` and re-inserts entries
// for the supplied label names. Unknown names are skipped silently.
func (s *Service) replaceRoleLabelsTx(tx *gorm.DB, roleID int64, names []string) error {
	if err := tx.Exec("DELETE FROM role_labels WHERE role_id = ?", roleID).Error; err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	var labels []model.Label
	if err := tx.Where("name IN ?", names).Find(&labels).Error; err != nil {
		return err
	}
	seen := make(map[int64]struct{}, len(labels))
	for _, l := range labels {
		if _, ok := seen[l.ID]; ok {
			continue
		}
		seen[l.ID] = struct{}{}
		if err := tx.Exec("INSERT INTO role_labels (role_id, label_id) VALUES (?, ?)", roleID, l.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func joinUnique(items []string) string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
