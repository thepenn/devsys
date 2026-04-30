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

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/model"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
	jobsvc "github.com/thepenn/devsys/service/pipeline/job"
)

type pipelineJobsRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newPipelineJobsRouter(services *service.Services, authMW *authmw.Middleware) *pipelineJobsRouter {
	return &pipelineJobsRouter{services: services, authMW: authMW}
}

type pipelineJobDetailResponse struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	DisplayName     string            `json:"display_name"`
	Description     string            `json:"description"`
	Content         string            `json:"content"`
	GitEnabled      bool              `json:"git_enabled"`
	GitCloneURL     string            `json:"git_clone_url"`
	GitBranch       string            `json:"git_branch"`
	GitCredentialID *int64            `json:"git_credential_id,omitempty"`
	Variables       map[string]string `json:"variables"`
	CronSchedules   []string          `json:"cron_schedules"`
	CreatedBy       string            `json:"created_by"`
	UpdatedBy       string            `json:"updated_by"`
	Created         int64             `json:"created"`
	Updated         int64             `json:"updated"`
}

type pipelineJobListResponse struct {
	Items   []jobsvc.JobSummary `json:"items"`
	Total   int64               `json:"total"`
	Page    int                 `json:"page"`
	PerPage int                 `json:"per_page"`
}

type pipelineJobCreateRequest struct {
	Name            string            `json:"name"`
	DisplayName     string            `json:"display_name"`
	Description     string            `json:"description"`
	Content         string            `json:"content"`
	GitEnabled      bool              `json:"git_enabled"`
	GitCloneURL     string            `json:"git_clone_url"`
	GitBranch       string            `json:"git_branch"`
	GitCredentialID *int64            `json:"git_credential_id"`
	Variables       map[string]string `json:"variables"`
	CronSchedules   []string          `json:"cron_schedules"`
}

type pipelineJobUpdateRequest struct {
	DisplayName     *string            `json:"display_name"`
	Description     *string            `json:"description"`
	Content         *string            `json:"content"`
	GitEnabled      *bool              `json:"git_enabled"`
	GitCloneURL     *string            `json:"git_clone_url"`
	GitBranch       *string            `json:"git_branch"`
	GitCredentialID *int64             `json:"git_credential_id"`
	ClearCredential bool               `json:"clear_credential"`
	Variables       *map[string]string `json:"variables"`
	// CronSchedules nil 表示不动; 指向 nil 切片 / 空切片表示清空所有调度.
	CronSchedules *[]string `json:"cron_schedules"`
}

type pipelineJobTriggerRequest struct {
	Branch    string            `json:"branch"`
	Variables map[string]string `json:"variables"`
}

type pipelineJobRunResponse struct {
	ID       int64             `json:"id"`
	Number   int64             `json:"number"`
	Status   model.StatusValue `json:"status"`
	Branch   string            `json:"branch"`
	Created  int64             `json:"created"`
	Started  int64             `json:"started"`
	Finished int64             `json:"finished"`
	Message  string            `json:"message"`
	Author   string            `json:"author"`
}

type pipelineJobRunListResponse struct {
	Items   []pipelineJobRunResponse `json:"items"`
	Page    int                      `json:"page"`
	PerPage int                      `json:"per_page"`
	Total   int64                    `json:"total"`
}

func (r *pipelineJobsRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	if r.services == nil || r.services.PipelineJob == nil {
		return nil
	}

	ws := register("/pipeline-jobs")
	// 与 sys_pipeline_templates.go 一样: 只在有 body 的路由上 .Consumes,
	// 避免无 body 的 trigger / cancel / delete 因 axios 不发 Content-Type
	// 触发 415.
	ws.Produces(restful.MIME_JSON)
	ws.Filter(r.authMW.Authenticate)
	ws.Filter(r.authMW.RequireAuth)

	read := []string{label.PipelineJobRead}
	write := []string{label.PipelineJobWrite}
	trigger := []string{label.PipelineJobTrigger}

	ws.Route(ws.GET("").To(r.list).
		Doc("列出独立 pipeline Job").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineJobListResponse{}).
		Returns(http.StatusOK, "jobs", pipelineJobListResponse{}))

	ws.Route(ws.POST("").To(r.create).
		Doc("创建独立 Job").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(pipelineJobCreateRequest{}).
		Writes(pipelineJobDetailResponse{}).
		Returns(http.StatusCreated, "created", pipelineJobDetailResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusConflict, "name conflict", errorResponse{}))

	ws.Route(ws.GET("/{id}").To(r.get).
		Doc("查看 Job 详情").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineJobDetailResponse{}).
		Returns(http.StatusOK, "job", pipelineJobDetailResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.PUT("/{id}").To(r.update).
		Doc("更新 Job").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(pipelineJobUpdateRequest{}).
		Writes(pipelineJobDetailResponse{}).
		Returns(http.StatusOK, "updated", pipelineJobDetailResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.DELETE("/{id}").To(r.delete).
		Doc("删除 Job").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Returns(http.StatusNoContent, "deleted", nil).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.POST("/{id}/run").To(r.trigger).
		Doc("立即运行 Job").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, trigger).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(pipelineJobTriggerRequest{}).
		Writes(pipelineJobRunResponse{}).
		Returns(http.StatusOK, "triggered", pipelineJobRunResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.GET("/{id}/runs").To(r.listRuns).
		Doc("列出 Job 运行历史").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineJobRunListResponse{}).
		Returns(http.StatusOK, "runs", pipelineJobRunListResponse{}))

	ws.Route(ws.GET("/{id}/runs/{run_id}").To(r.getRun).
		Doc("查看 Job 单次运行").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineRunDetailResponse{}).
		Returns(http.StatusOK, "run", pipelineRunDetailResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.POST("/{id}/runs/{run_id}/cancel").To(r.cancelRun).
		Doc("取消 Job 运行").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, trigger).
		Metadata(label.MetaModule, label.ModuleProject).
		Returns(http.StatusNoContent, "cancelled", nil).
		Returns(http.StatusConflict, "cannot cancel", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.POST("/{id}/runs/{run_id}/steps/{step_id}/approval").To(r.submitApproval).
		Doc("提交 Job 步骤审批").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, trigger).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(approvalActionRequest{}).
		Writes(model.Step{}).
		Returns(http.StatusOK, "step", model.Step{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	return []*restful.WebService{ws}
}

func (r *pipelineJobsRouter) list(req *restful.Request, resp *restful.Response) {
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	keyword := strings.TrimSpace(req.QueryParameter("keyword"))
	result, err := r.services.PipelineJob.List(req.Request.Context(), jobsvc.ListOptions{
		Page:    page,
		PerPage: perPage,
		Keyword: keyword,
	})
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineJobListResponse{
		Items:   result.Items,
		Total:   result.Total,
		Page:    result.Page,
		PerPage: result.PerPage,
	})
}

func (r *pipelineJobsRouter) create(req *restful.Request, resp *restful.Response) {
	claims, _ := authmw.FromContext(req.Request.Context())
	var body pipelineJobCreateRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	job, err := r.services.PipelineJob.Create(req.Request.Context(), jobsvc.CreateInput{
		Name:            body.Name,
		DisplayName:     body.DisplayName,
		Description:     body.Description,
		Content:         body.Content,
		GitEnabled:      body.GitEnabled,
		GitCloneURL:     body.GitCloneURL,
		GitBranch:       body.GitBranch,
		GitCredentialID: body.GitCredentialID,
		Variables:       body.Variables,
		CronSchedules:   body.CronSchedules,
		Actor:           actorOf(claims),
	})
	if err != nil {
		writeJobError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusCreated, toJobDetail(job))
}

func (r *pipelineJobsRouter) get(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	job, err := r.services.PipelineJob.Get(req.Request.Context(), id)
	if err != nil {
		writeJobError(resp, err, http.StatusInternalServerError)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, toJobDetail(job))
}

func (r *pipelineJobsRouter) update(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	claims, _ := authmw.FromContext(req.Request.Context())
	var body pipelineJobUpdateRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	job, err := r.services.PipelineJob.Update(req.Request.Context(), id, jobsvc.UpdateInput{
		DisplayName:     body.DisplayName,
		Description:     body.Description,
		Content:         body.Content,
		GitEnabled:      body.GitEnabled,
		GitCloneURL:     body.GitCloneURL,
		GitBranch:       body.GitBranch,
		GitCredentialID: body.GitCredentialID,
		ClearCredential: body.ClearCredential,
		Variables:       body.Variables,
		CronSchedules:   body.CronSchedules,
		Actor:           actorOf(claims),
	})
	if err != nil {
		writeJobError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, toJobDetail(job))
}

func (r *pipelineJobsRouter) delete(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if err := r.services.PipelineJob.Delete(req.Request.Context(), id); err != nil {
		writeJobError(resp, err, http.StatusInternalServerError)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *pipelineJobsRouter) trigger(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	claims, _ := authmw.FromContext(req.Request.Context())
	var body pipelineJobTriggerRequest
	// trigger 允许空 body, ReadEntity 失败时降级为默认值.
	_ = req.ReadEntity(&body)
	pipeline, err := r.services.PipelineJob.Trigger(req.Request.Context(), id, model.PipelineOptions{
		Branch:    body.Branch,
		Variables: body.Variables,
	}, actorOf(claims))
	if err != nil {
		writeJobError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineJobRunResponse{
		ID:       pipeline.ID,
		Number:   pipeline.Number,
		Status:   pipeline.Status,
		Branch:   pipeline.Branch,
		Created:  pipeline.Created,
		Started:  pipeline.Started,
		Finished: pipeline.Finished,
		Message:  pipeline.Message,
		Author:   pipeline.Author,
	})
}

func (r *pipelineJobsRouter) listRuns(req *restful.Request, resp *restful.Response) {
	jobID, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	items, total, err := r.services.Pipeline.ListPipelinesByJob(req.Request.Context(), jobID, page, perPage)
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
	out := pipelineJobRunListResponse{
		Items:   make([]pipelineJobRunResponse, 0, len(items)),
		Page:    page,
		PerPage: perPage,
		Total:   total,
	}
	for _, item := range items {
		out.Items = append(out.Items, pipelineJobRunResponse{
			ID:       item.ID,
			Number:   item.Number,
			Status:   item.Status,
			Branch:   item.Branch,
			Created:  item.Created,
			Started:  item.Started,
			Finished: item.Finished,
			Message:  item.Message,
			Author:   item.Author,
		})
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, out)
}

func (r *pipelineJobsRouter) getRun(req *restful.Request, resp *restful.Response) {
	jobID, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	runID, err := parseInt64Param(req, "run_id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	claims, _ := authmw.FromContext(req.Request.Context())

	detail, err := r.services.Pipeline.GetJobPipelineRunDetail(req.Request.Context(), jobID, runID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(resp, http.StatusNotFound, err)
			return
		}
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	if detail == nil || detail.Pipeline == nil {
		writeError(resp, http.StatusNotFound, errors.New("pipeline run not found"))
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, buildPipelineRunDetailResponse(detail, actorOf(claims)))
}

func (r *pipelineJobsRouter) cancelRun(req *restful.Request, resp *restful.Response) {
	jobID, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	runID, err := parseInt64Param(req, "run_id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	reason := strings.TrimSpace(req.QueryParameter("reason"))
	if err := r.services.Pipeline.CancelJobPipelineRun(req.Request.Context(), jobID, runID, reason); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(resp, http.StatusNotFound, err)
			return
		}
		if strings.Contains(err.Error(), "已结束") {
			writeError(resp, http.StatusConflict, err)
			return
		}
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *pipelineJobsRouter) submitApproval(req *restful.Request, resp *restful.Response) {
	jobID, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	runID, err := parseInt64Param(req, "run_id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	stepID, err := parseInt64Param(req, "step_id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	claims, _ := authmw.FromContext(req.Request.Context())
	var body approvalActionRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	step, err := r.services.Pipeline.SubmitJobStepApproval(req.Request.Context(), jobID, runID, stepID, actorOf(claims), body.Action, body.Comment)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(resp, http.StatusNotFound, err)
			return
		}
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if step != nil {
		decorateApprovalForUser(step, actorOf(claims))
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, step)
}

func writeJobError(resp *restful.Response, err error, fallback int) {
	switch {
	case errors.Is(err, jobsvc.ErrJobNotFound):
		writeError(resp, http.StatusNotFound, err)
	case errors.Is(err, jobsvc.ErrJobNameConflict):
		writeError(resp, http.StatusConflict, err)
	default:
		writeError(resp, fallback, err)
	}
}

func toJobDetail(job *model.PipelineJob) pipelineJobDetailResponse {
	if job == nil {
		return pipelineJobDetailResponse{}
	}
	vars := job.Variables
	if vars == nil {
		vars = map[string]string{}
	}
	cron := job.CronSchedules
	if cron == nil {
		cron = []string{}
	}
	return pipelineJobDetailResponse{
		ID:              job.ID,
		Name:            job.Name,
		DisplayName:     job.DisplayName,
		Description:     job.Description,
		Content:         job.Content,
		GitEnabled:      job.GitEnabled,
		GitCloneURL:     job.GitCloneURL,
		GitBranch:       job.GitBranch,
		GitCredentialID: job.GitCredentialID,
		Variables:       vars,
		CronSchedules:   cron,
		CreatedBy:       job.CreatedBy,
		UpdatedBy:       job.UpdatedBy,
		Created:         job.Created,
		Updated:         job.Updated,
	}
}

// _ ensures fmt is used even when no explicit Sprintf in trimmed-down builds.
var _ = fmt.Sprintf
