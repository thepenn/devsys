package routers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"gorm.io/gorm"

	"github.com/kuzane/go-devops/model"
	authmw "github.com/kuzane/go-devops/routers/middleware/auth"
	"github.com/kuzane/go-devops/service"
	authsvc "github.com/kuzane/go-devops/service/auth"
	pipelinesvc "github.com/kuzane/go-devops/service/pipeline"
)

type repoRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

type repoListResponse struct {
	Items   []*model.Repo `json:"items"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Total   int64         `json:"total"`
}

type pipelineConfigResponse struct {
	Content   string `json:"content"`
	UpdatedAt int64  `json:"updated_at"`
}

type pipelineConfigRequest struct {
	Content string `json:"content"`
}

type pipelineRunRequest struct {
	Branch    string            `json:"branch"`
	Variables map[string]string `json:"variables"`
	Commit    string            `json:"commit"`
}

type pipelineRunResponse struct {
	ID       int64             `json:"id"`
	Number   int64             `json:"number"`
	Status   model.StatusValue `json:"status"`
	Branch   string            `json:"branch"`
	Created  int64             `json:"created"`
	Finished int64             `json:"finished"`
	Message  string            `json:"message"`
	Author   string            `json:"author"`
	Commit   string            `json:"commit"`
}

type pipelineRunListResponse struct {
	Items   []pipelineRunResponse `json:"items"`
	Page    int                   `json:"page"`
	PerPage int                   `json:"per_page"`
	Total   int64                 `json:"total"`
}

type pipelineRunDetailResponse struct {
	Pipeline  pipelineRunDetailPipeline  `json:"pipeline"`
	Workflows []pipelineWorkflowResponse `json:"workflows"`
}

type pipelineRunDetailPipeline struct {
	ID       int64             `json:"id"`
	Number   int64             `json:"number"`
	Status   model.StatusValue `json:"status"`
	Branch   string            `json:"branch"`
	Commit   string            `json:"commit"`
	Message  string            `json:"message"`
	Author   string            `json:"author"`
	Created  int64             `json:"created"`
	Started  int64             `json:"started"`
	Finished int64             `json:"finished"`
}

type pipelineWorkflowResponse struct {
	ID       int64                  `json:"id"`
	PID      int                    `json:"pid"`
	Name     string                 `json:"name"`
	State    model.StatusValue      `json:"state"`
	Started  int64                  `json:"started"`
	Finished int64                  `json:"finished"`
	Steps    []pipelineStepResponse `json:"steps"`
}

type pipelineStepResponse struct {
	ID       int64               `json:"id"`
	PID      int                 `json:"pid"`
	PPID     int                 `json:"ppid"`
	Name     string              `json:"name"`
	Type     model.StepType      `json:"type"`
	State    model.StatusValue   `json:"state"`
	Started  int64               `json:"started"`
	Finished int64               `json:"finished"`
	Logs     []pipelineStepLog   `json:"logs"`
	Approval *model.StepApproval `json:"approval,omitempty"`
}

type pipelineStepLog struct {
	Line    int    `json:"line"`
	Type    string `json:"type"`
	Time    int64  `json:"time"`
	Content string `json:"content"`
}

type approvalActionRequest struct {
	Action  string `json:"action"`
	Comment string `json:"comment"`
}

type pipelineSettingsResponse struct {
	CleanupEnabled   bool     `json:"cleanup_enabled"`
	RetentionDays    int      `json:"retention_days"`
	MaxRecords       int      `json:"max_records"`
	Dockerfile       string   `json:"dockerfile"`
	DisallowParallel bool     `json:"disallow_parallel"`
	CronSchedules    []string `json:"cron_schedules"`
}

type pipelineSettingsRequest struct {
	CleanupEnabled   bool     `json:"cleanup_enabled"`
	RetentionDays    int      `json:"retention_days"`
	MaxRecords       int      `json:"max_records"`
	Dockerfile       string   `json:"dockerfile"`
	DisallowParallel bool     `json:"disallow_parallel"`
	CronSchedules    []string `json:"cron_schedules"`
}

var errRepoNotFound = errors.New("repository not found")

func newRepoRouter(services *service.Services, authMW *authmw.Middleware) *repoRouter {
	return &repoRouter{services: services, authMW: authMW}
}

func (r *repoRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	ws := register("/repos")
	ws.Filter(r.authMW.Authenticate)

	ws.Route(ws.GET("").To(r.list).
		Doc("List repositories accessible to the current user").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Writes(repoListResponse{}).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "repository list", repoListResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}))

	ws.Route(ws.POST("/sync").To(r.sync).
		Doc("Trigger synchronization of Git repositories for the current user").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusNoContent, "sync triggered", nil).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusInternalServerError, "sync failed", errorResponse{}))

	ws.Route(ws.POST("/{repo_id}/sync").To(r.syncOne).
		Doc("Synchronize a single repository by forge remote id").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusNoContent, "sync triggered", nil).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusBadRequest, "invalid id", errorResponse{}).
		Returns(http.StatusInternalServerError, "sync failed", errorResponse{}))

	ws.Route(ws.GET("/{repo_id}/pipeline/runs").To(r.listPipelineRuns).
		Doc("List pipelines for repository").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "pipeline runs", pipelineRunListResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.GET("/{repo_id}/pipeline/runs/{pipeline_id}").To(r.getPipelineRun).
		Doc("Get detailed information for a pipeline run").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "pipeline run", pipelineRunDetailResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.POST("/{repo_id}/pipeline/runs/{pipeline_id}/steps/{step_id}/approval").To(r.submitPipelineApproval).
		Doc("Submit an approval decision for a pipeline step").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Reads(approvalActionRequest{}).
		Writes(model.Step{}).
		Returns(http.StatusOK, "step", model.Step{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.GET("/{repo_id}/pipeline/config").To(r.getPipelineConfig).
		Doc("Get pipeline configuration for repository").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "config", pipelineConfigResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.PUT("/{repo_id}/pipeline/config").To(r.updatePipelineConfig).
		Doc("Create or update pipeline configuration for repository").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Reads(pipelineConfigRequest{}).
		Returns(http.StatusOK, "config", pipelineConfigResponse{}).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.GET("/{repo_id}/pipeline/settings").To(r.getPipelineSettings).
		Doc("Get pipeline settings for repository").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusOK, "settings", pipelineSettingsResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusNotFound, "repository not found", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.PUT("/{repo_id}/pipeline/settings").To(r.updatePipelineSettings).
		Doc("Update pipeline settings for repository").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Reads(pipelineSettingsRequest{}).
		Returns(http.StatusOK, "settings", pipelineSettingsResponse{}).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.POST("/{repo_id}/pipeline/run").To(r.triggerPipeline).
		Doc("Trigger a manual pipeline run").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Reads(pipelineRunRequest{}).
		Returns(http.StatusOK, "pipeline", pipelineRunResponse{}).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.POST("/{repo_id}/pipeline/runs/{pipeline_id}/cancel").To(r.cancelPipelineRun).
		Doc("Cancel a running pipeline").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Filter(r.authMW.RequireAuth).
		Returns(http.StatusNoContent, "cancelled", nil).
		Returns(http.StatusBadRequest, "invalid request", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusNotFound, "pipeline not found", errorResponse{}).
		Returns(http.StatusConflict, "cannot cancel", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	return []*restful.WebService{ws}
}

func (r *repoRouter) list(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	search := req.QueryParameter("search")
	syncedParam := strings.TrimSpace(strings.ToLower(req.QueryParameter("synced")))

	var syncedFilter *bool
	if syncedParam != "" {
		switch syncedParam {
		case "true", "1", "yes", "synced", "active":
			v := true
			syncedFilter = &v
		case "false", "0", "no", "unsynced", "inactive":
			v := false
			syncedFilter = &v
		}
	}

	opts := model.ListOptions{Page: page, PerPage: perPage}
	repos, total, err := r.services.Repo.ListByUserPaged(req.Request.Context(), claims.UserID, opts, search, syncedFilter)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 20
	}

	result := repoListResponse{
		Items:   repos,
		Page:    opts.Page,
		PerPage: opts.PerPage,
		Total:   total,
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, result)
}

func (r *repoRouter) listPipelineRuns(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))

	items, total, err := r.services.Pipeline.ListPipelinesByRepo(req.Request.Context(), repo.ID, page, perPage)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}

	response := pipelineRunListResponse{
		Items:   make([]pipelineRunResponse, 0, len(items)),
		Page:    page,
		PerPage: perPage,
		Total:   total,
	}
	for _, item := range items {
		response.Items = append(response.Items, pipelineRunResponse{
			ID:       item.ID,
			Number:   item.Number,
			Status:   item.Status,
			Branch:   item.Branch,
			Created:  item.Created,
			Finished: item.Finished,
			Message:  item.Message,
			Author:   item.Author,
			Commit:   item.Commit,
		})
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, response)
}

func (r *repoRouter) sync(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	if err := r.services.Auth.SyncRepositories(req.Request.Context(), claims.UserID); err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *repoRouter) syncOne(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repoID := strings.TrimSpace(req.PathParameter("repo_id"))
	if repoID == "" {
		writeError(resp, http.StatusBadRequest, errors.New("missing repository id"))
		return
	}
	if err := r.services.Auth.SyncRepository(req.Request.Context(), claims.UserID, repoID); err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *repoRouter) getPipelineRun(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	pipelineParam := strings.TrimSpace(req.PathParameter("pipeline_id"))
	if pipelineParam == "" {
		writeError(resp, http.StatusBadRequest, errors.New("missing pipeline id"))
		return
	}
	pipelineID, err := strconv.ParseInt(pipelineParam, 10, 64)
	if err != nil {
		writeError(resp, http.StatusBadRequest, errors.New("invalid pipeline id"))
		return
	}

	detail, err := r.services.Pipeline.GetPipelineRunDetail(req.Request.Context(), repo.ID, pipelineID)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	if detail == nil || detail.Pipeline == nil {
		writeError(resp, http.StatusNotFound, errors.New("pipeline run not found"))
		return
	}
	decorateApprovalPermissions(detail, claims.Login)

	stepMap := make(map[int][]pipelineStepResponse)
	for _, step := range detail.Steps {
		decorateApprovalForUser(step, claims.Login)
		logs := make([]pipelineStepLog, 0, len(detail.Logs[step.ID]))
		for _, entry := range detail.Logs[step.ID] {
			logs = append(logs, pipelineStepLog{
				Line:    entry.Line,
				Type:    logTypeString(entry.Type),
				Time:    entry.Time,
				Content: string(entry.Data),
			})
		}
		stepMap[step.PPID] = append(stepMap[step.PPID], pipelineStepResponse{
			ID:       step.ID,
			PID:      step.PID,
			PPID:     step.PPID,
			Name:     step.Name,
			Type:     step.Type,
			State:    step.State,
			Started:  step.Started,
			Finished: step.Finished,
			Logs:     logs,
			Approval: step.Approval,
		})
	}

	workflows := make([]pipelineWorkflowResponse, 0, len(detail.Workflows))
	for _, wf := range detail.Workflows {
		respSteps := stepMap[wf.PID]
		workflows = append(workflows, pipelineWorkflowResponse{
			ID:       wf.ID,
			PID:      wf.PID,
			Name:     wf.Name,
			State:    wf.State,
			Started:  wf.Started,
			Finished: wf.Finished,
			Steps:    respSteps,
		})
	}

	runResp := pipelineRunDetailPipeline{
		ID:       detail.Pipeline.ID,
		Number:   detail.Pipeline.Number,
		Status:   detail.Pipeline.Status,
		Branch:   detail.Pipeline.Branch,
		Commit:   detail.Pipeline.Commit,
		Message:  detail.Pipeline.Message,
		Author:   detail.Pipeline.Author,
		Created:  detail.Pipeline.Created,
		Started:  detail.Pipeline.Started,
		Finished: detail.Pipeline.Finished,
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineRunDetailResponse{
		Pipeline:  runResp,
		Workflows: workflows,
	})
}

func (r *repoRouter) submitPipelineApproval(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}
	pipelineID, err := strconv.ParseInt(strings.TrimSpace(req.PathParameter("pipeline_id")), 10, 64)
	if err != nil {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("invalid pipeline id"))
		return
	}
	stepID, err := strconv.ParseInt(strings.TrimSpace(req.PathParameter("step_id")), 10, 64)
	if err != nil {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("invalid step id"))
		return
	}
	var body approvalActionRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	step, err := r.services.Pipeline.SubmitStepApproval(req.Request.Context(), repo.ID, pipelineID, stepID, claims.Login, body.Action, body.Comment)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else {
			errMsg := err.Error()
			lowerMsg := strings.ToLower(errMsg)
			switch {
			case strings.Contains(errMsg, "不在审批名单"):
				status = http.StatusForbidden
			case strings.Contains(errMsg, "审批用户无效"),
				strings.Contains(errMsg, "无效的审批操作"),
				strings.Contains(errMsg, "该步骤不需要审批"),
				strings.Contains(errMsg, "审批配置缺失"),
				strings.Contains(errMsg, "审批已通过"),
				strings.Contains(errMsg, "审批已经结束"),
				strings.Contains(errMsg, "审批已超时"),
				strings.Contains(lowerMsg, "invalid"),
				strings.Contains(errMsg, "无效"):
				status = http.StatusBadRequest
			}
		}
		writeError(resp, status, err)
		return
	}
	if step == nil {
		writeError(resp, http.StatusNotFound, fmt.Errorf("step not found"))
		return
	}
	decorateApprovalForUser(step, claims.Login)
	_ = resp.WriteHeaderAndEntity(http.StatusOK, step)
}

func (r *repoRouter) cancelPipelineRun(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	pipelineParam := strings.TrimSpace(req.PathParameter("pipeline_id"))
	if pipelineParam == "" {
		writeError(resp, http.StatusBadRequest, errors.New("missing pipeline id"))
		return
	}
	pipelineID, err := strconv.ParseInt(pipelineParam, 10, 64)
	if err != nil {
		writeError(resp, http.StatusBadRequest, errors.New("invalid pipeline id"))
		return
	}

	reason := strings.TrimSpace(req.QueryParameter("reason"))
	if err := r.services.Pipeline.CancelPipelineRun(req.Request.Context(), repo.ID, pipelineID, reason); err != nil {
		if strings.Contains(err.Error(), "已结束") {
			writeError(resp, http.StatusConflict, err)
			return
		}
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	resp.WriteHeader(http.StatusNoContent)
}

func (r *repoRouter) getPipelineConfig(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	cfg, err := r.services.Pipeline.EnsurePipelineConfig(req.Request.Context(), repo)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineConfigResponse{
		Content:   cfg.Content,
		UpdatedAt: cfg.Updated,
	})
}

func (r *repoRouter) updatePipelineConfig(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	var body pipelineConfigRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	cfg, err := r.services.Pipeline.UpsertPipelineConfig(req.Request.Context(), repo.ID, body.Content)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineConfigResponse{
		Content:   cfg.Content,
		UpdatedAt: cfg.Updated,
	})
}

func (r *repoRouter) triggerPipeline(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	var body pipelineRunRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	cfg, err := r.services.Pipeline.EnsurePipelineConfig(req.Request.Context(), repo)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	options := model.PipelineOptions{
		Branch:    strings.TrimSpace(body.Branch),
		Variables: body.Variables,
		Commit:    strings.TrimSpace(body.Commit),
	}
	if options.Variables == nil {
		options.Variables = make(map[string]string)
	}

	pipeline, err := r.services.Pipeline.TriggerManualPipeline(req.Request.Context(), repo, claims.Login, options, cfg)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineRunResponse{
		ID:       pipeline.ID,
		Number:   pipeline.Number,
		Status:   pipeline.Status,
		Branch:   pipeline.Branch,
		Created:  pipeline.Created,
		Finished: pipeline.Finished,
		Message:  pipeline.Message,
		Author:   pipeline.Author,
		Commit:   pipeline.Commit,
	})
}

func (r *repoRouter) getPipelineSettings(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	settings, err := r.services.Pipeline.GetPipelineSettings(req.Request.Context(), repo.ID)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	respBody := pipelineSettingsResponse{
		CleanupEnabled:   settings.CleanupEnabled,
		RetentionDays:    settings.RetentionDays,
		MaxRecords:       settings.MaxRecords,
		Dockerfile:       settings.Dockerfile,
		DisallowParallel: settings.DisallowParallel,
		CronSchedules:    append([]string{}, settings.CronSchedules...),
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, respBody)
}

func (r *repoRouter) updatePipelineSettings(req *restful.Request, resp *restful.Response) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	repo, err := r.repoFromRequest(req, claims)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRepoNotFound) {
			status = http.StatusNotFound
		}
		writeError(resp, status, err)
		return
	}

	var body pipelineSettingsRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	if body.RetentionDays < 0 {
		body.RetentionDays = 0
	}
	if body.MaxRecords <= 0 {
		body.MaxRecords = 10
	}
	if body.CronSchedules == nil {
		body.CronSchedules = []string{}
	}
	saved, err := r.services.Pipeline.UpsertPipelineSettings(req.Request.Context(), repo.ID, model.RepoPipelineConfig{
		CleanupEnabled:   body.CleanupEnabled,
		RetentionDays:    body.RetentionDays,
		MaxRecords:       body.MaxRecords,
		Dockerfile:       body.Dockerfile,
		DisallowParallel: body.DisallowParallel,
		CronSchedules:    body.CronSchedules,
	})
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	respBody := pipelineSettingsResponse{
		CleanupEnabled:   saved.CleanupEnabled,
		RetentionDays:    saved.RetentionDays,
		MaxRecords:       saved.MaxRecords,
		Dockerfile:       saved.Dockerfile,
		DisallowParallel: saved.DisallowParallel,
		CronSchedules:    append([]string{}, saved.CronSchedules...),
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, respBody)
}

func decorateApprovalPermissions(detail *pipelinesvc.PipelineRunDetail, login string) {
	if detail == nil {
		return
	}
	for _, step := range detail.Steps {
		decorateApprovalForUser(step, login)
	}
	for _, wf := range detail.Workflows {
		for _, step := range wf.Children {
			decorateApprovalForUser(step, login)
		}
	}
}

func decorateApprovalForUser(step *model.Step, login string) {
	if step == nil || step.Approval == nil {
		return
	}
	approval := step.Approval
	approval.CanApprove = false
	approval.CanReject = false
	if pending := pendingApprovers(approval); len(pending) > 0 {
		approval.PendingApprovers = pending
	} else {
		approval.PendingApprovers = nil
	}
	if strings.TrimSpace(login) == "" {
		return
	}
	if approval.State != model.StepApprovalStatePending {
		return
	}
	allowed := len(approval.Approvers) == 0 || containsIgnoreCaseSlice(approval.Approvers, login)
	if !allowed {
		return
	}
	for _, decision := range approval.Decisions {
		if strings.EqualFold(strings.TrimSpace(decision.User), login) {
			return
		}
	}
	approval.CanApprove = true
	approval.CanReject = true
	if len(approval.Approvers) > 0 {
		approval.PendingApprovers = pendingApprovers(approval)
	}
}

func containsIgnoreCaseSlice(list []string, target string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func pendingApprovers(approval *model.StepApproval) []string {
	if approval == nil || len(approval.Approvers) == 0 {
		return nil
	}
	approved := make(map[string]struct{})
	for _, decision := range approval.Decisions {
		if strings.EqualFold(strings.TrimSpace(decision.Action), "approve") {
			approved[strings.ToLower(strings.TrimSpace(decision.User))] = struct{}{}
		}
	}
	result := make([]string, 0)
	for _, approver := range approval.Approvers {
		if _, ok := approved[strings.ToLower(strings.TrimSpace(approver))]; !ok {
			result = append(result, approver)
		}
	}
	return result
}

func logTypeString(t model.LogEntryType) string {
	switch t {
	case model.LogEntryStdout:
		return "stdout"
	case model.LogEntryStderr:
		return "stderr"
	case model.LogEntryExitCode:
		return "exit_code"
	case model.LogEntryMetadata:
		return "metadata"
	case model.LogEntryProgress:
		return "progress"
	default:
		return "unknown"
	}
}

func (r *repoRouter) repoFromRequest(req *restful.Request, claims *authsvc.SessionClaims) (*model.Repo, error) {
	repoIDParam := strings.TrimSpace(req.PathParameter("repo_id"))
	if repoIDParam == "" {
		return nil, errRepoNotFound
	}
	id, err := strconv.ParseInt(repoIDParam, 10, 64)
	if err != nil {
		return nil, errRepoNotFound
	}
	repo, err := r.services.Repo.FindByID(req.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if repo == nil || claims == nil {
		return nil, errRepoNotFound
	}
	if repo.UserID == claims.UserID {
		return repo, nil
	}
	if r.services == nil || r.services.User == nil {
		return nil, errRepoNotFound
	}
	user, err := r.services.User.FindByID(req.Request.Context(), claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.Admin {
		return nil, errRepoNotFound
	}
	return repo, nil
}
