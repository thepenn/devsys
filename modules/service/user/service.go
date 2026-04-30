package user

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/cache"
	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

// roleCacheTTL 决定 RoleNames 缓存条目的存活时间. 60s 是经验值: 角色变更后
// 最坏要等 1 分钟才在 ACL 中生效, 但所有 mutation 路径 (SetUserRoles /
// assignDefaultRoles / 经 rbac.Service 的 AssignUserRoles) 都会 InvalidateRoles
// 即时清理, 因此真正受影响的窗口仅是"绕过 service 直接改库"的场景.
const roleCacheTTL = 60 * time.Second

// roleCacheKey 返回 user_id 对应的缓存键. 缓存值类型为 []string (角色名列表).
func roleCacheKey(userID int64) string {
	return "user_roles:" + strconv.FormatInt(userID, 10)
}

// Service encapsulates user related business logic.
type Service struct {
	db        *store.DB
	roleCache *cache.Cache
}

// New constructs a user service. The roleCache is optional; when nil RoleNames
// degrades gracefully to direct DB lookups.
func New(db *store.DB, roleCache *cache.Cache) *Service {
	return &Service{db: db, roleCache: roleCache}
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
	var firstLogin bool
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
			firstLogin = true
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

	// 默认角色分配规则:
	//   - 首次登录 + provider IsAdmin -> superadmin (上层 toUserInfo 也会置 admin=true)
	//   - 首次登录 + 普通用户       -> guest (人工在 UI 上后续提升)
	//   - 已有用户                  -> 不动 user_roles, 由管理员维护
	// 注意: 这里允许 IsAdmin 在每次登录时把已存在用户**晋升**(append) 为
	// superadmin, 但不会把已有的角色降级.
	if firstLogin {
		if info.IsAdmin {
			if err := s.assignDefaultRoles(ctx, result.ID, label.RoleSuperadmin); err != nil {
				return result, err
			}
		} else {
			if err := s.assignDefaultRoles(ctx, result.ID, label.RoleGuest); err != nil {
				return result, err
			}
		}
	} else if info.IsAdmin {
		if err := s.assignDefaultRoles(ctx, result.ID, label.RoleSuperadmin); err != nil {
			return result, err
		}
	}
	return result, nil
}

// assignDefaultRoles inserts (user_id, role.Name) rows that do not yet exist.
// Roles are looked up by name; unknown role names are silently skipped.
func (s *Service) assignDefaultRoles(ctx context.Context, userID int64, roleNames ...string) error {
	if userID == 0 || len(roleNames) == 0 {
		return nil
	}
	now := time.Now().Unix()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var roles []model.Role
		if err := tx.WithContext(ctx).Where("name IN ?", roleNames).Find(&roles).Error; err != nil {
			return err
		}
		for _, role := range roles {
			ur := model.UserRole{UserID: userID, RoleID: role.ID, Created: now}
			if err := tx.WithContext(ctx).
				Where("user_id = ? AND role_id = ?", userID, role.ID).
				FirstOrCreate(&ur).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.InvalidateRoles(userID)
	}
	return err
}

// RoleNames returns the role.Name strings assigned to the given user.
// Returns an empty slice when the user has no role bindings.
//
// 命中缓存时 ACL 中间件不再产生 SQL 流量; cache miss 走 DB 后回填.
// 即使 user 当前无任何角色也会缓存空 slice, 防止热点用户每次请求穿透.
func (s *Service) RoleNames(ctx context.Context, userID int64) ([]string, error) {
	if userID == 0 {
		return nil, nil
	}
	if s.roleCache != nil {
		if cached, ok := s.roleCache.Get(roleCacheKey(userID)); ok {
			if names, ok := cached.([]string); ok {
				return append([]string(nil), names...), nil
			}
		}
	}
	var names []string
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Table("user_roles").
			Joins("JOIN roles ON roles.id = user_roles.role_id").
			Where("user_roles.user_id = ?", userID).
			Order("roles.name ASC").
			Pluck("roles.name", &names).Error
	})
	if err != nil {
		return nil, err
	}
	if names == nil {
		names = []string{}
	}
	if s.roleCache != nil {
		s.roleCache.Set(roleCacheKey(userID), append([]string(nil), names...), roleCacheTTL)
	}
	return names, nil
}

// InvalidateRoles drops the cached RoleNames entry for the user. Called after
// any mutation to user_roles so subsequent ACL checks see fresh data.
func (s *Service) InvalidateRoles(userID int64) {
	if s == nil || s.roleCache == nil || userID == 0 {
		return
	}
	s.roleCache.Delete(roleCacheKey(userID))
}

// SetUserRoles replaces a user's role bindings with the supplied role name set.
// Unknown role names are ignored. Pass an empty slice to remove all bindings.
// Invalidates the role cache on success so ACL middleware sees the new set
// on the next request.
func (s *Service) SetUserRoles(ctx context.Context, userID int64, roleNames []string) error {
	if userID == 0 {
		return errors.New("user id is required")
	}
	dedup := make(map[string]struct{}, len(roleNames))
	for _, n := range roleNames {
		if n == "" {
			continue
		}
		dedup[n] = struct{}{}
	}
	wanted := make([]string, 0, len(dedup))
	for n := range dedup {
		wanted = append(wanted, n)
	}
	sort.Strings(wanted)

	now := time.Now().Unix()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		ctxTx := tx.WithContext(ctx)

		var roles []model.Role
		if len(wanted) > 0 {
			if err := ctxTx.Where("name IN ?", wanted).Find(&roles).Error; err != nil {
				return err
			}
		}
		want := make(map[int64]struct{}, len(roles))
		for _, r := range roles {
			want[r.ID] = struct{}{}
		}

		var existing []model.UserRole
		if err := ctxTx.Where("user_id = ?", userID).Find(&existing).Error; err != nil {
			return err
		}
		have := make(map[int64]struct{}, len(existing))
		for _, ur := range existing {
			have[ur.RoleID] = struct{}{}
		}

		for roleID := range want {
			if _, ok := have[roleID]; ok {
				continue
			}
			if err := ctxTx.Create(&model.UserRole{UserID: userID, RoleID: roleID, Created: now}).Error; err != nil {
				return err
			}
		}
		for roleID := range have {
			if _, ok := want[roleID]; ok {
				continue
			}
			if err := ctxTx.Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&model.UserRole{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.InvalidateRoles(userID)
	}
	return err
}

func generateUserHash() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
