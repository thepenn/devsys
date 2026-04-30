// Package acl is the second-stage HTTP filter that enforces label-based RBAC
// rules declared in route Metadata. It runs immediately after the JWT auth
// filter has populated the request context.
//
// 流程:
//   1. 路由未声明 acl=true: 直接放行.
//   2. 没有 Bearer Token / 解析失败 -> 401.
//   3. 用户角色与路由 labels 求交集; 命中或 superadmin 拥有 `*` -> 放行.
//   4. 否则 -> 403.
package acl

import (
	"net/http"
	"strings"

	"github.com/emicklei/go-restful/v3"

	internalauth "github.com/thepenn/devsys/internal/auth"
	"github.com/thepenn/devsys/internal/label"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	userService "github.com/thepenn/devsys/service/user"
)

// Middleware enforces label-based ACL on routes that opt in via Metadata.
type Middleware struct {
	users *userService.Service
	rbac  *internalauth.RBAC
}

// New constructs a new ACL middleware. Both dependencies are required for
// routes that declare ACL; if either is nil ACL routes will be denied.
func New(users *userService.Service, rbac *internalauth.RBAC) *Middleware {
	return &Middleware{users: users, rbac: rbac}
}

// Middleware returns the filter chain entry; satisfies handler.RegisterMiddleware.
func (m *Middleware) Middleware() []restful.FilterFunction {
	return []restful.FilterFunction{m.Filter}
}

// Filter inspects the selected route's Metadata and either grants the
// request, rejects it with 401/403, or short-circuits when ACL is disabled.
func (m *Middleware) Filter(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	route := req.SelectedRoute()
	if route == nil {
		chain.ProcessFilter(req, resp)
		return
	}
	meta := route.Metadata()
	if !boolMeta(meta, label.MetaACL) {
		chain.ProcessFilter(req, resp)
		return
	}

	if m == nil || m.users == nil || m.rbac == nil {
		writeJSON(resp, http.StatusInternalServerError, map[string]string{"error": "rbac not initialised"})
		return
	}

	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok || claims == nil {
		writeJSON(resp, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	roles, err := m.users.RoleNames(req.Request.Context(), claims.UserID)
	if err != nil {
		writeJSON(resp, http.StatusInternalServerError, map[string]string{"error": "failed to resolve user roles"})
		return
	}

	required := stringSliceMeta(meta, label.MetaLabels)
	if len(required) == 0 {
		// ACL declared without any labels = "登录即可访问"
		chain.ProcessFilter(req, resp)
		return
	}

	if !m.rbac.AnyLabel(roles, required) {
		writeJSON(resp, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	chain.ProcessFilter(req, resp)
}

func boolMeta(meta map[string]interface{}, key string) bool {
	if v, ok := meta[key]; ok {
		if flag, ok := v.(bool); ok {
			return flag
		}
	}
	return false
}

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

func writeJSON(resp *restful.Response, status int, body interface{}) {
	resp.WriteHeader(status)
	_ = resp.WriteAsJson(body)
}
