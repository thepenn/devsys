package routers

import (
	"net/http"
	"strconv"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/internal/label"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
	auditService "github.com/thepenn/devsys/service/audit"
)

type auditRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newAuditRouter(services *service.Services, authMW *authmw.Middleware) *auditRouter {
	return &auditRouter{services: services, authMW: authMW}
}

func (r *auditRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	if r.services == nil || r.services.Audit == nil {
		return nil
	}

	ws := register("/audit")
	ws.Produces(restful.MIME_JSON)
	ws.Filter(r.authMW.Authenticate)
	ws.Filter(r.authMW.RequireAuth)

	ws.Route(ws.GET("/logs").To(r.list).
		Doc("操作审计日志").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, []string{label.SystemAudit}).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes(auditService.ListResult{}).
		Returns(http.StatusOK, "OK", auditService.ListResult{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	return []*restful.WebService{ws}
}

func (r *auditRouter) list(req *restful.Request, resp *restful.Response) {
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	userID, _ := strconv.ParseInt(req.QueryParameter("user_id"), 10, 64)
	start, _ := strconv.ParseInt(req.QueryParameter("start"), 10, 64)
	end, _ := strconv.ParseInt(req.QueryParameter("end"), 10, 64)

	opts := auditService.ListOptions{
		Page:    page,
		PerPage: perPage,
		UserID:  userID,
		Login:   strings.TrimSpace(req.QueryParameter("login")),
		Method:  strings.TrimSpace(req.QueryParameter("method")),
		Path:    strings.TrimSpace(req.QueryParameter("path")),
		Start:   start,
		End:     end,
	}
	result, err := r.services.Audit.List(req.Request.Context(), opts)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, result)
}
