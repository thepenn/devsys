package admin

import (
	"net/http"

	"github.com/emicklei/go-restful/v3"

	authmw "github.com/kuzane/go-devops/routers/middleware/auth"
	userService "github.com/kuzane/go-devops/service/user"
)

const (
	// AdminEnable marks routes that require administrator privileges.
	AdminEnable = "admin"
)

// Middleware checks route metadata to enforce administrator access.
type Middleware struct {
	users *userService.Service
}

// New creates a new admin middleware instance.
func New(users *userService.Service) *Middleware {
	return &Middleware{users: users}
}

// Middleware satisfies handler.RegisterMiddleware so the filter can be registered globally.
func (m *Middleware) Middleware() []restful.FilterFunction {
	return []restful.FilterFunction{m.Filter}
}

// Filter ensures requests hitting routes tagged with AdminEnable have admin privileges.
func (m *Middleware) Filter(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	route := req.SelectedRoute()
	if route != nil && requiresAdmin(route.Metadata()) {
		claims, ok := authmw.FromContext(req.Request.Context())
		if !ok || claims == nil {
			writeJSON(resp, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if m.users == nil {
			writeJSON(resp, http.StatusInternalServerError, map[string]string{"error": "user service unavailable"})
			return
		}

		user, err := m.users.FindByID(req.Request.Context(), claims.UserID)
		if err != nil {
			writeJSON(resp, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
			return
		}
		if user == nil {
			writeJSON(resp, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if !user.Admin {
			writeJSON(resp, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
	}

	chain.ProcessFilter(req, resp)
}

func requiresAdmin(meta map[string]interface{}) bool {
	if len(meta) == 0 {
		return false
	}
	if enabled, ok := meta[AdminEnable]; ok {
		if flag, ok := enabled.(bool); ok {
			return flag
		}
	}
	return false
}

func writeJSON(resp *restful.Response, status int, body interface{}) {
	resp.WriteHeader(status)
	_ = resp.WriteAsJson(body)
}
