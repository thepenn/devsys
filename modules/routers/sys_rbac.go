package routers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/model"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
	rbacsvc "github.com/thepenn/devsys/service/rbac"
)

type rbacRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newRBACRouter(services *service.Services, authMW *authmw.Middleware) *rbacRouter {
	return &rbacRouter{services: services, authMW: authMW}
}

type roleResponse struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Title   string   `json:"title"`
	Parents []string `json:"parents"`
	Builtin bool     `json:"builtin"`
	Labels  []string `json:"labels"`
}

type roleRequest struct {
	Name    string   `json:"name"`
	Title   string   `json:"title"`
	Parents []string `json:"parents"`
	Labels  []string `json:"labels"`
}

type labelResponse struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	Module  string `json:"module"`
	Builtin bool   `json:"builtin"`
}

type endpointResponse struct {
	ID      int64    `json:"id"`
	Path    string   `json:"path"`
	Method  string   `json:"method"`
	Module  string   `json:"module"`
	Remark  string   `json:"remark"`
	Updated int64    `json:"updated"`
	Labels  []string `json:"labels"`
}

type userRoleResponse struct {
	UserID int64    `json:"user_id"`
	Login  string   `json:"login"`
	Email  string   `json:"email"`
	Avatar string   `json:"avatar_url"`
	Admin  bool     `json:"admin"`
	Roles  []string `json:"roles"`
}

type userRoleAssignRequest struct {
	Roles []string `json:"roles"`
}

func (r *rbacRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	if r.services == nil || r.services.RBAC == nil {
		return nil
	}

	ws := register("/rbac")
	ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)
	ws.Filter(r.authMW.Authenticate)
	ws.Filter(r.authMW.RequireAuth)

	ws.Route(ws.GET("/roles").To(r.listRoles).
		Doc("列出所有角色").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes([]roleResponse{}).
		Returns(http.StatusOK, "roles", []roleResponse{}))

	ws.Route(ws.POST("/roles").To(r.createRole).
		Doc("创建角色").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Reads(roleRequest{}).
		Writes(roleResponse{}).
		Returns(http.StatusCreated, "created", roleResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}))

	ws.Route(ws.PUT("/roles/{id}").To(r.updateRole).
		Doc("更新角色").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Reads(roleRequest{}).
		Writes(roleResponse{}).
		Returns(http.StatusOK, "updated", roleResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.DELETE("/roles/{id}").To(r.deleteRole).
		Doc("删除角色").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Returns(http.StatusNoContent, "deleted", nil).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.GET("/labels").To(r.listLabels).
		Doc("列出所有 label (按 module 分组)").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes([]labelResponse{}).
		Returns(http.StatusOK, "labels", []labelResponse{}))

	ws.Route(ws.GET("/endpoints").To(r.listEndpoints).
		Doc("列出已自动同步的 API endpoint 目录").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes([]endpointResponse{}).
		Returns(http.StatusOK, "endpoints", []endpointResponse{}))

	ws.Route(ws.GET("/users").To(r.listUserRoles).
		Doc("列出用户及其角色").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes([]userRoleResponse{}).
		Returns(http.StatusOK, "users", []userRoleResponse{}))

	ws.Route(ws.PUT("/users/{id}/roles").To(r.assignUserRoles).
		Doc("修改单个用户的角色绑定").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemRoleWrite}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Reads(userRoleAssignRequest{}).
		Returns(http.StatusNoContent, "updated", nil).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}))

	return []*restful.WebService{ws}
}

func (r *rbacRouter) listRoles(req *restful.Request, resp *restful.Response) {
	roles, err := r.services.RBAC.ListRoles(req.Request.Context())
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	out := make([]roleResponse, 0, len(roles))
	for _, role := range roles {
		out = append(out, toRoleResponse(role))
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, out)
}

func (r *rbacRouter) createRole(req *restful.Request, resp *restful.Response) {
	var body roleRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	role, err := r.services.RBAC.CreateRole(req.Request.Context(), rbacsvc.RoleInput{
		Name:       body.Name,
		Title:      body.Title,
		Parents:    body.Parents,
		LabelNames: body.Labels,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusCreated, toRoleResponse(*role))
}

func (r *rbacRouter) updateRole(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	var body roleRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	role, err := r.services.RBAC.UpdateRole(req.Request.Context(), id, rbacsvc.RoleInput{
		Name:       body.Name,
		Title:      body.Title,
		Parents:    body.Parents,
		LabelNames: body.Labels,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}
	if role == nil {
		writeError(resp, http.StatusNotFound, gorm.ErrRecordNotFound)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, toRoleResponse(*role))
}

func (r *rbacRouter) deleteRole(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if err := r.services.RBAC.DeleteRole(req.Request.Context(), id); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *rbacRouter) listLabels(req *restful.Request, resp *restful.Response) {
	labels, err := r.services.RBAC.ListLabels(req.Request.Context())
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	out := make([]labelResponse, 0, len(labels))
	for _, l := range labels {
		out = append(out, labelResponse{
			ID:      l.ID,
			Name:    l.Name,
			Title:   l.Title,
			Module:  l.Module,
			Builtin: l.Builtin,
		})
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, out)
}

func (r *rbacRouter) listEndpoints(req *restful.Request, resp *restful.Response) {
	endpoints, err := r.services.RBAC.ListEndpoints(req.Request.Context())
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	out := make([]endpointResponse, 0, len(endpoints))
	for _, e := range endpoints {
		labels := make([]string, 0, len(e.Labels))
		for _, l := range e.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, endpointResponse{
			ID:      e.ID,
			Path:    e.Path,
			Method:  e.Method,
			Module:  e.Module,
			Remark:  e.Remark,
			Updated: e.Updated,
			Labels:  labels,
		})
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, out)
}

func (r *rbacRouter) listUserRoles(req *restful.Request, resp *restful.Response) {
	rows, err := r.services.RBAC.ListUserRoles(req.Request.Context())
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	out := make([]userRoleResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, userRoleResponse{
			UserID: row.UserID,
			Login:  row.Login,
			Email:  row.Email,
			Avatar: row.Avatar,
			Admin:  row.Admin,
			Roles:  row.RoleNames,
		})
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, out)
}

func (r *rbacRouter) assignUserRoles(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	var body userRoleAssignRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if err := r.services.RBAC.AssignUserRoles(req.Request.Context(), id, body.Roles); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func toRoleResponse(role model.Role) roleResponse {
	parents := make([]string, 0)
	if trimmed := strings.TrimSpace(role.Parents); trimmed != "" {
		for _, p := range strings.Split(trimmed, ",") {
			if v := strings.TrimSpace(p); v != "" {
				parents = append(parents, v)
			}
		}
	}
	labels := make([]string, 0, len(role.Labels))
	for _, l := range role.Labels {
		labels = append(labels, l.Name)
	}
	return roleResponse{
		ID:      role.ID,
		Name:    role.Name,
		Title:   role.Title,
		Parents: parents,
		Builtin: role.Builtin,
		Labels:  labels,
	}
}

func parseInt64Param(req *restful.Request, name string) (int64, error) {
	raw := strings.TrimSpace(req.PathParameter(name))
	if raw == "" {
		return 0, errors.New("missing path parameter: " + name)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid path parameter: " + name)
	}
	return id, nil
}
