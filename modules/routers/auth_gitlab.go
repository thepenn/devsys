package routers

import (
	"errors"
	"net/http"
	"net/url"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
	authsvc "github.com/thepenn/devsys/service/auth"
)

type authRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
	provider string
}

func newAuthRouter(services *service.Services, authMW *authmw.Middleware, provider string) *authRouter {
	if provider == "" {
		provider = "gitlab"
	}
	return &authRouter{
		services: services,
		authMW:   authMW,
		provider: provider,
	}
}

func (r *authRouter) router(register func(path string) *restful.WebService, tags []string) []*restful.WebService {
	ws := register("/auth/" + r.provider)
	ws.Route(ws.GET("/login").To(r.login).
		Doc("OAuth login").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Writes(loginResponse{}).
		Returns(http.StatusOK, "redirect url", loginResponse{}).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusInternalServerError, "internal error", errorResponse{}))

	ws.Route(ws.GET("/callback").To(r.callback).
		Doc("OAuth callback").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Writes(authsvc.AuthResponse{}).
		Returns(http.StatusOK, "auth response", nil).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusInternalServerError, "internal error", errorResponse{}))

	ws.Route(ws.GET("/me").To(r.me).
		Doc("Get information about the authenticated user").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Writes(authsvc.UserInfo{}).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "user info", authsvc.UserInfo{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}))

	// /api/v1/auth/providers: 暴露当前激活 provider, 让前端不必把 provider
	// 名称编进 bundle (用 REACT_APP_AUTH_PROVIDER). 前端依此渲染登录按钮.
	infoWs := register("/auth")
	infoWs.Route(infoWs.GET("/providers").To(r.listProviders).
		Doc("List active OAuth providers").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Writes(providersResponse{}).
		Returns(http.StatusOK, "providers", providersResponse{}))

	return []*restful.WebService{ws, infoWs}
}

type loginResponse struct {
	State       string `json:"state"`
	RedirectURL string `json:"redirect_url"`
}

type providerInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type providersResponse struct {
	Active    string         `json:"active"`
	Providers []providerInfo `json:"providers"`
}

// providerDisplayNames maps internal provider names to user-facing labels.
var providerDisplayNames = map[string]string{
	"gitlab": "GitLab",
	"github": "GitHub",
	"gitee":  "Gitee",
	"gitea":  "Gitea",
}

func providerDisplayName(name string) string {
	if v, ok := providerDisplayNames[name]; ok {
		return v
	}
	return name
}

func (r *authRouter) listProviders(req *restful.Request, resp *restful.Response) {
	// 当前后端 auth.Service 在构造时只接受一个 provider, 所以本期只返回
	// 激活的 provider. 前端渲染单按钮 ("使用 GitLab 登录"). 后续真正落
	// 多 provider 同时启用时, 这里追加返回更多条目即可, 前端无需改动.
	body := providersResponse{
		Active: r.provider,
		Providers: []providerInfo{{
			Name:        r.provider,
			DisplayName: providerDisplayName(r.provider),
		}},
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, body)
}

func (r *authRouter) login(req *restful.Request, resp *restful.Response) {
	redirect := req.QueryParameter("redirect")
	state, url, err := r.services.Auth.BeginGitLabAuth(req.Request.Context(), redirect)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	resp.AddHeader("Location", url)
	resp.AddHeader("X-Auth-State", state)
	resp.WriteHeader(http.StatusFound)
}

func (r *authRouter) callback(req *restful.Request, resp *restful.Response) {
	code := req.QueryParameter("code")
	state := req.QueryParameter("state")
	if code == "" || state == "" {
		writeError(resp, http.StatusBadRequest, errors.New("missing code or state"))
		return
	}
	result, err := r.services.Auth.CompleteGitLabAuth(req.Request.Context(), code, state)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	if result.Redirect != "" {
		target, parseErr := url.Parse(result.Redirect)
		if parseErr == nil {
			query := target.Query()
			query.Set("token", result.Token)
			target.RawQuery = query.Encode()
			resp.AddHeader("Location", target.String())
			resp.WriteHeader(http.StatusFound)
			return
		}
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, result)
}

func (r *authRouter) me(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	info, err := r.services.Auth.CurrentUser(req.Request.Context(), claims.UserID)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	if info == nil {
		writeError(resp, http.StatusNotFound, errors.New("user not found"))
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, info)
}
