// Package audit hooks every mutating, ACL-protected request and emits an
// audit_logs row via service/audit.
//
// Filter conditions:
//   1. Skip GET / HEAD / OPTIONS - read-only and CORS preflights are noisy.
//   2. Skip routes without Metadata acl=true - public health/metrics/static.
//   3. Skip /audit/* routes themselves to avoid recursion.
//   4. Skip routes whose path starts with the API root + /auth/ - login flows
//      already write the user record, audit doesn't add value there.
package audit

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/model"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	auditService "github.com/thepenn/devsys/service/audit"
)

// Middleware enriches each protected mutating request with an audit_logs row.
type Middleware struct {
	svc *auditService.Service
}

// New constructs the middleware. svc is required; nil-safe behaviour falls
// back to a no-op filter so the middleware never crashes the request chain.
func New(svc *auditService.Service) *Middleware {
	return &Middleware{svc: svc}
}

// Middleware satisfies handler.RegisterMiddleware.
func (m *Middleware) Middleware() []restful.FilterFunction {
	return []restful.FilterFunction{m.Filter}
}

// Filter wraps every request; the audit row is recorded *after* the handler
// runs so we can capture status + duration. Non-recordable requests pass
// through with effectively zero overhead (no DB I/O, no allocations beyond
// a single map lookup).
func (m *Middleware) Filter(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	if !m.shouldRecord(req) {
		chain.ProcessFilter(req, resp)
		return
	}

	start := time.Now()
	chain.ProcessFilter(req, resp)
	duration := time.Since(start)

	if m.svc == nil {
		return
	}

	entry := model.AuditLog{
		Method:   strings.ToUpper(req.Request.Method),
		Path:     pathOrTemplate(req),
		Status:   resp.StatusCode(),
		Duration: duration.Milliseconds(),
		IP:       clientIP(req.Request),
		Summary:  summarise(req),
		Created:  start.Unix(),
	}
	if claims, ok := authmw.FromContext(req.Request.Context()); ok && claims != nil {
		entry.UserID = claims.UserID
		entry.Login = claims.Login
	}
	m.svc.Record(entry)
}

// shouldRecord returns true when the request matches a route that we want
// to keep an audit trail for.
func (m *Middleware) shouldRecord(req *restful.Request) bool {
	method := strings.ToUpper(req.Request.Method)
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	route := req.SelectedRoute()
	if route == nil {
		return false
	}
	if !boolMeta(route.Metadata(), label.MetaACL) {
		return false
	}
	path := route.Path()
	if strings.Contains(path, "/audit/") {
		return false
	}
	if strings.Contains(path, "/auth/") {
		return false
	}
	return true
}

func pathOrTemplate(req *restful.Request) string {
	if route := req.SelectedRoute(); route != nil {
		return route.Path()
	}
	return req.Request.URL.Path
}

// clientIP extracts the most upstream IP address from X-Forwarded-For when
// present (in case of reverse-proxy deployments), falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// summarise serialises path params + a few well-known query params into a
// short JSON blob. We intentionally never store the request body to avoid
// leaking secrets (passwords, tokens, certs) into audit_logs.
func summarise(req *restful.Request) string {
	out := make(map[string]any, 4)
	for k, v := range req.PathParameters() {
		out[k] = v
	}
	q := req.Request.URL.Query()
	for _, key := range []string{"page", "per_page", "search", "type", "namespace", "cluster"} {
		if v := q.Get(key); v != "" {
			out["q_"+key] = v
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	if len(b) > 1024 {
		b = b[:1024]
	}
	return string(b)
}

func boolMeta(meta map[string]interface{}, key string) bool {
	if v, ok := meta[key]; ok {
		if flag, ok := v.(bool); ok {
			return flag
		}
	}
	return false
}
