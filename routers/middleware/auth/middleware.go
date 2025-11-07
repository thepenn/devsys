package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/service/auth"
)

type ctxKey string

const userContextKey ctxKey = "auth:user"

type Middleware struct {
	service *auth.Service
}

func New(service *auth.Service) *Middleware {
	return &Middleware{service: service}
}

func (m *Middleware) Middleware() []restful.FilterFunction {
	return []restful.FilterFunction{m.Authenticate}
}

func (m *Middleware) Authenticate(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	ctx, _ := m.parseAndAttach(req.Request)
	req.Request = req.Request.WithContext(ctx)
	chain.ProcessFilter(req, resp)
}

func (m *Middleware) RequireAuth(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	ctx, user := m.parseAndAttach(req.Request)
	if user == nil {
		resp.WriteHeaderAndEntity(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	req.Request = req.Request.WithContext(ctx)
	chain.ProcessFilter(req, resp)
}

func (m *Middleware) parseAndAttach(r *http.Request) (context.Context, *auth.SessionClaims) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return r.Context(), nil
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return r.Context(), nil
	}
	claims, err := m.service.ParseToken(parts[1])
	if err != nil {
		return r.Context(), nil
	}
	ctx := context.WithValue(r.Context(), userContextKey, claims)
	return ctx, claims
}

func FromContext(ctx context.Context) (*auth.SessionClaims, bool) {
	claims, ok := ctx.Value(userContextKey).(*auth.SessionClaims)
	return claims, ok
}
