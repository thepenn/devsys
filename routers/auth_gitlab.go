package routers

import (
	"errors"
	"net/http"
	"net/url"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	authmw "github.com/kuzane/go-devops/routers/middleware/auth"
	"github.com/kuzane/go-devops/service"
	authsvc "github.com/kuzane/go-devops/service/auth"
)

type authRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newAuthRouter(services *service.Services, authMW *authmw.Middleware) *authRouter {
	return &authRouter{
		services: services,
		authMW:   authMW,
	}
}

func (r *authRouter) router(register func(path string) *restful.WebService, tags []string) []*restful.WebService {
	ws := register("/auth/gitlab")
	ws.Route(ws.GET("/login").To(r.login).
		Doc("GitLab OAuth login").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Writes(loginResponse{}).
		Returns(http.StatusOK, "redirect url", loginResponse{}).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusInternalServerError, "internal error", errorResponse{}))

	ws.Route(ws.GET("/callback").To(r.callback).
		Doc("GitLab OAuth callback").
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

	return []*restful.WebService{ws}
}

type loginResponse struct {
	State       string `json:"state"`
	RedirectURL string `json:"redirect_url"`
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
