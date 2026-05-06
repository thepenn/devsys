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
	authsvc "github.com/thepenn/devsys/service/auth"
	"github.com/thepenn/devsys/service"
)

type streamLogsRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newStreamLogsRouter(services *service.Services, authMW *authmw.Middleware) *streamLogsRouter {
	return &streamLogsRouter{services: services, authMW: authMW}
}

func (r *streamLogsRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	read := []string{label.ProjectRead}
	ws := register("/stream")
	ws.Filter(r.authMW.Authenticate)
	ws.Route(ws.GET("/logs/{repo_id}/{pipeline}/{step_id}").To(r.streamRepoStepLogsWoodpecker).
		Doc("Woodpecker-compatible SSE log stream (pipeline path param is run number, not database id)").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "text/event-stream", nil).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))
	return []*restful.WebService{ws}
}

func (r *streamLogsRouter) streamRepoStepLogsWoodpecker(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := streamRepoFromRequest(r.services, req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}
	pipelineNumber, err := parseInt64Param(req, "pipeline")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	stepID, err := parseInt64Param(req, "step_id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	_, step, err := r.services.Pipeline.GetRepoPipelineStepForLogStream(req.Request.Context(), repo.ID, pipelineNumber, stepID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(resp, http.StatusNotFound, errors.New("pipeline run or step not found"))
			return
		}
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	writeWoodpeckerStepLogSSE(resp, req, r.services.Pipeline, step)
}

// streamRepoFromRequest resolves repo_id the same way as repoRouter.repoFromRequest.
func streamRepoFromRequest(services *service.Services, req *restful.Request, claims *authsvc.SessionClaims) (*model.Repo, error) {
	repoIDParam := strings.TrimSpace(req.PathParameter("repo_id"))
	if repoIDParam == "" {
		return nil, errRepoNotFound
	}
	id, err := strconv.ParseInt(repoIDParam, 10, 64)
	if err != nil {
		return nil, errRepoNotFound
	}
	repo, err := services.Repo.FindByID(req.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if repo == nil || claims == nil {
		return nil, errRepoNotFound
	}
	if repo.UserID == claims.UserID {
		return repo, nil
	}
	if services == nil || services.User == nil {
		return nil, errRepoNotFound
	}
	user, err := services.User.FindByID(req.Request.Context(), claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.Admin {
		return nil, errRepoNotFound
	}
	return repo, nil
}
