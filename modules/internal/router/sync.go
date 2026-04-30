// Package router contains glue code that bridges the go-restful runtime with
// the persistent endpoint catalog used by the role-management UI.
//
// On boot the Sync visitor walks every registered route, picks out those that
// declare label.MetaACL=true, and upserts the (method, path, module, labels)
// tuple into the endpoints / endpoint_labels tables. Routes that disappear
// from the codebase are pruned. The catalog is purely informational: actual
// access checks happen against the route Metadata, not the DB.
package router

import (
	"context"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

// Sync implements handler.StorageRouter so the handler layer can drive the
// endpoint catalog refresh exactly once after every WebService is registered.
type Sync struct {
	db *store.DB
}

// New creates a Sync that writes to the given DB.
func New(db *store.DB) *Sync {
	return &Sync{db: db}
}

// StoreRouter satisfies handler.StorageRouter. Errors are logged but never
// returned: a stale endpoint catalog must not crash the API server.
func (s *Sync) StoreRouter(c *restful.Container) {
	if s == nil || s.db == nil || c == nil {
		return
	}
	if err := s.sync(context.Background(), c); err != nil {
		log.Error().Err(err).Msg("rbac: failed to sync endpoint catalog")
	}
}

func (s *Sync) sync(ctx context.Context, c *restful.Container) error {
	type discovered struct {
		ep     model.Endpoint
		labels []string
	}

	now := time.Now().Unix()
	var found []discovered
	for _, ws := range c.RegisteredWebServices() {
		for _, route := range ws.Routes() {
			if !boolMeta(route.Metadata, label.MetaACL) {
				continue
			}
			labels := stringSliceMeta(route.Metadata, label.MetaLabels)
			if len(labels) == 0 {
				continue
			}
			found = append(found, discovered{
				ep: model.Endpoint{
					Path:    route.Path,
					Method:  strings.ToUpper(route.Method),
					Module:  stringMeta(route.Metadata, label.MetaModule),
					Remark:  firstNonEmpty(stringMeta(route.Metadata, label.MetaRemark), route.Doc),
					Updated: now,
				},
				labels: labels,
			})
		}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		ctxTx := tx.WithContext(ctx)

		var allLabels []model.Label
		if err := ctxTx.Find(&allLabels).Error; err != nil {
			return err
		}
		labelByName := make(map[string]model.Label, len(allLabels))
		for _, l := range allLabels {
			labelByName[l.Name] = l
		}

		// Auto-create labels that appear in code but not yet in DB so newly
		// added routes can declare ad-hoc labels without a separate seed step.
		for _, d := range found {
			for _, name := range d.labels {
				if _, ok := labelByName[name]; ok {
					continue
				}
				row := model.Label{Name: name, Title: name, Module: d.ep.Module, Builtin: false}
				if err := ctxTx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "name"}},
					DoNothing: true,
				}).Create(&row).Error; err != nil {
					return err
				}
				if err := ctxTx.Where("name = ?", name).Take(&row).Error; err != nil {
					return err
				}
				labelByName[name] = row
			}
		}

		keep := make(map[int64]struct{}, len(found))
		for _, d := range found {
			ep := d.ep
			if err := ctxTx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "method"}, {Name: "path"}},
				DoUpdates: clause.AssignmentColumns([]string{"module", "remark", "updated"}),
			}).Create(&ep).Error; err != nil {
				return err
			}

			var existing model.Endpoint
			if err := ctxTx.Where("method = ? AND path = ?", ep.Method, ep.Path).Take(&existing).Error; err != nil {
				return err
			}
			keep[existing.ID] = struct{}{}

			if err := ctxTx.Exec("DELETE FROM endpoint_labels WHERE endpoint_id = ?", existing.ID).Error; err != nil {
				return err
			}
			seen := make(map[int64]struct{}, len(d.labels))
			for _, name := range d.labels {
				lab, ok := labelByName[name]
				if !ok {
					continue
				}
				if _, dup := seen[lab.ID]; dup {
					continue
				}
				seen[lab.ID] = struct{}{}
				if err := ctxTx.Exec(
					"INSERT INTO endpoint_labels (endpoint_id, label_id) VALUES (?, ?)",
					existing.ID, lab.ID,
				).Error; err != nil {
					return err
				}
			}
		}

		// Remove endpoints that no longer exist in code.
		var existing []model.Endpoint
		if err := ctxTx.Find(&existing).Error; err != nil {
			return err
		}
		for _, e := range existing {
			if _, ok := keep[e.ID]; ok {
				continue
			}
			if err := ctxTx.Exec("DELETE FROM endpoint_labels WHERE endpoint_id = ?", e.ID).Error; err != nil {
				return err
			}
			if err := ctxTx.Delete(&model.Endpoint{}, e.ID).Error; err != nil {
				return err
			}
		}

		log.Info().
			Int("registered", len(found)).
			Msg("rbac: endpoint catalog synced")
		return nil
	})
}

func boolMeta(meta map[string]interface{}, key string) bool {
	if v, ok := meta[key]; ok {
		if flag, ok := v.(bool); ok {
			return flag
		}
	}
	return false
}

func stringMeta(meta map[string]interface{}, key string) string {
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// stringSliceMeta extracts a []string from route Metadata. Tolerates both
// []string and []interface{} so callers can pass a literal slice without
// importing this package.
func stringSliceMeta(meta map[string]interface{}, key string) []string {
	v, ok := meta[key]
	if !ok {
		return nil
	}
	switch typed := v.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if s := strings.TrimSpace(typed); s != "" {
			return []string{s}
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
