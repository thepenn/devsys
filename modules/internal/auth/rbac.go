// Package auth wraps github.com/mikespook/gorbac/v2 with a database-backed
// engine that mirrors the roles / labels / role_labels tables.
//
// 数据流:
//
//	roles + role_labels + labels   -- DB
//	          |
//	          v   Init()/Rebuild()
//	   gorbac.RBAC (in-memory)
//	          |
//	          v   AnyLabel()/EffectiveLabels()
//	  ACL middleware / /me handler
//
// Rebuild() 在角色或标签关联变更后被显式调用 (例如管理后台 CRUD), 全量重读
// DB 然后构建一棵新的 gorbac 图. 这条路径走写锁; 读路径 (鉴权) 走读锁.
package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/mikespook/gorbac/v2"
	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

// RBAC is the in-memory permission engine, backed by GORM tables.
type RBAC struct {
	db *store.DB

	mu        sync.RWMutex
	rbac      *gorbac.RBAC
	effective map[string]map[string]struct{} // roleName -> set of label names
}

// NewRBAC builds an empty engine. Call Rebuild before serving traffic.
func NewRBAC(db *store.DB) *RBAC {
	return &RBAC{
		db:        db,
		rbac:      gorbac.New(),
		effective: make(map[string]map[string]struct{}),
	}
}

// Init loads roles + labels from DB and builds the gorbac graph.
// Equivalent to calling Rebuild on a fresh engine.
func (r *RBAC) Init(ctx context.Context) error {
	return r.Rebuild(ctx)
}

// Rebuild atomically replaces the in-memory graph from the DB state.
// Safe to call concurrently; only blocks readers for the swap window.
func (r *RBAC) Rebuild(ctx context.Context) error {
	roles, labelsByRole, err := r.loadRoles(ctx)
	if err != nil {
		return err
	}

	rb := gorbac.New()

	// First pass: register all roles with their direct labels (permissions).
	for _, role := range roles {
		stdRole := gorbac.NewStdRole(role.Name)
		for _, lname := range labelsByRole[role.ID] {
			if err := stdRole.Assign(gorbac.NewStdPermission(lname)); err != nil {
				return fmt.Errorf("assign label %q to role %q: %w", lname, role.Name, err)
			}
		}
		if err := rb.Add(stdRole); err != nil {
			return fmt.Errorf("add role %q: %w", role.Name, err)
		}
	}

	// Second pass: wire inheritance after every role has been added.
	for _, role := range roles {
		parents := splitParents(role.Parents)
		if len(parents) == 0 {
			continue
		}
		if err := rb.SetParents(role.Name, parents); err != nil {
			return fmt.Errorf("set parents for %q: %w", role.Name, err)
		}
	}

	// Pre-compute effective label sets per role (including inherited).
	effective := make(map[string]map[string]struct{}, len(roles))
	for _, role := range roles {
		effective[role.Name] = expandLabels(rb, role.Name)
	}

	r.mu.Lock()
	r.rbac = rb
	r.effective = effective
	r.mu.Unlock()
	return nil
}

// AnyLabel reports whether any of the supplied roles grants any of the
// supplied labels. Empty `labels` means "no ACL declared" and returns true.
// A role granted the Wildcard label always returns true.
func (r *RBAC) AnyLabel(roles []string, labels []string) bool {
	if len(labels) == 0 {
		return true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, roleName := range roles {
		set, ok := r.effective[roleName]
		if !ok {
			continue
		}
		if _, hasWildcard := set[label.Wildcard]; hasWildcard {
			return true
		}
		for _, l := range labels {
			if l == "" {
				continue
			}
			if _, ok := set[l]; ok {
				return true
			}
		}
	}
	return false
}

// EffectiveLabels returns the union of label names granted to the supplied
// roles (including inherited labels). The result is sorted for stable output.
// If the union contains the Wildcard label, returns []string{Wildcard} only.
func (r *RBAC) EffectiveLabels(roles []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	union := make(map[string]struct{})
	for _, roleName := range roles {
		set, ok := r.effective[roleName]
		if !ok {
			continue
		}
		for l := range set {
			union[l] = struct{}{}
		}
	}
	if _, ok := union[label.Wildcard]; ok {
		return []string{label.Wildcard}
	}
	out := make([]string, 0, len(union))
	for l := range union {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// HasRole reports whether a role exists in the engine. Useful for callers
// validating user input before assignment.
func (r *RBAC) HasRole(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.effective[name]
	return ok
}

// loadRoles reads roles, labels and the join table in a single read view and
// returns: ordered list of roles + roleID -> []labelName.
func (r *RBAC) loadRoles(ctx context.Context) ([]model.Role, map[int64][]string, error) {
	var roles []model.Role
	var labelRows []model.Label
	type roleLabelRow struct {
		RoleID  int64
		LabelID int64
	}
	var joins []roleLabelRow

	err := r.db.View(func(tx *gorm.DB) error {
		ctx := tx.WithContext(ctx)
		if err := ctx.Order("id ASC").Find(&roles).Error; err != nil {
			return err
		}
		if err := ctx.Find(&labelRows).Error; err != nil {
			return err
		}
		return ctx.Table("role_labels").
			Select("role_id, label_id").
			Find(&joins).Error
	})
	if err != nil {
		return nil, nil, err
	}

	labelByID := make(map[int64]string, len(labelRows))
	for _, l := range labelRows {
		labelByID[l.ID] = l.Name
	}

	out := make(map[int64][]string, len(roles))
	for _, j := range joins {
		name, ok := labelByID[j.LabelID]
		if !ok {
			continue
		}
		out[j.RoleID] = append(out[j.RoleID], name)
	}
	return roles, out, nil
}

// expandLabels walks a role + parents and returns the union of permission IDs
// (label names). Uses gorbac's Walk to follow the inheritance graph.
func expandLabels(rb *gorbac.RBAC, roleName string) map[string]struct{} {
	set := make(map[string]struct{})

	walker := func(role gorbac.Role, parents []string) error {
		std, ok := role.(*gorbac.StdRole)
		if !ok {
			return nil
		}
		for _, perm := range std.Permissions() {
			set[perm.ID()] = struct{}{}
		}
		return nil
	}

	visited := make(map[string]struct{})
	var visit func(name string) error
	visit = func(name string) error {
		if _, ok := visited[name]; ok {
			return nil
		}
		visited[name] = struct{}{}
		role, parents, err := rb.Get(name)
		if err != nil {
			if errors.Is(err, gorbac.ErrRoleNotExist) {
				return nil
			}
			return err
		}
		if err := walker(role, parents); err != nil {
			return err
		}
		for _, p := range parents {
			if err := visit(p); err != nil {
				return err
			}
		}
		return nil
	}
	_ = visit(roleName)
	return set
}

// splitParents converts the comma-separated `parents` column into a slice,
// trimming empty entries.
func splitParents(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
