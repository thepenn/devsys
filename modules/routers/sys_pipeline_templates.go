package routers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/model"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
	authsvc "github.com/thepenn/devsys/service/auth"
	templatesvc "github.com/thepenn/devsys/service/pipeline/template"
)

type pipelineTemplatesRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newPipelineTemplatesRouter(services *service.Services, authMW *authmw.Middleware) *pipelineTemplatesRouter {
	return &pipelineTemplatesRouter{services: services, authMW: authMW}
}

type pipelineTemplateSummaryResponse struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	DisplayName  string     `json:"display_name"`
	Description  string     `json:"description"`
	Kind         string     `json:"kind"`
	IsPublished  bool       `json:"is_published"`
	PublishedAt  *time.Time `json:"published_at"`
	PublishedBy  string     `json:"published_by"`
	CreatedBy    string     `json:"created_by"`
	UpdatedBy    string     `json:"updated_by"`
	Created      int64      `json:"created"`
	Updated      int64      `json:"updated"`
	ReferencedBy int64      `json:"referenced_by"`
}

type pipelineTemplateListResponse struct {
	Items   []pipelineTemplateSummaryResponse `json:"items"`
	Total   int64                             `json:"total"`
	Page    int                               `json:"page"`
	PerPage int                               `json:"per_page"`
}

type pipelineTemplateDetailResponse struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	DisplayName      string     `json:"display_name"`
	Description      string     `json:"description"`
	Kind             string     `json:"kind"`
	DraftContent     string     `json:"draft_content"`
	PublishedContent string     `json:"published_content"`
	IsPublished      bool       `json:"is_published"`
	PublishedAt      *time.Time `json:"published_at"`
	PublishedBy      string     `json:"published_by"`
	CreatedBy        string     `json:"created_by"`
	UpdatedBy        string     `json:"updated_by"`
	Created          int64      `json:"created"`
	Updated          int64      `json:"updated"`
}

type pipelineTemplateCreateRequest struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Kind         string `json:"kind"` // pipeline (默认) | step
	DraftContent string `json:"draft_content"`
}

type pipelineTemplateDraftRequest struct {
	DisplayName  *string `json:"display_name"`
	Description  *string `json:"description"`
	DraftContent *string `json:"draft_content"`
}

type pipelineTemplateRenderRequest struct {
	Variables map[string]string `json:"variables"`
	// RepoID 可选: 给项目侧 Drawer 预览用. 非 0 时把当前项目的
	// CI_REPO_* / REPO_* / BRANCH 等 vars 注入到渲染上下文, 让预览结果
	// 与真实触发一致.
	RepoID int64 `json:"repo_id,omitempty"`
}

type pipelineTemplateRenderResponse struct {
	Content string   `json:"content"`
	Missing []string `json:"missing,omitempty"`
}

func (r *pipelineTemplatesRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	if r.services == nil || r.services.PipelineTemplate == nil {
		return nil
	}

	ws := register("/pipeline-templates")
	// 不在 WebService 级别声明 Consumes; go-restful 会对没有 Content-Type
	// 的 publish / delete 请求 (axios 无 body 时不发 Content-Type 头) 直接
	// 返回 415. 改为只在真正有 body 的路由上 .Consumes(restful.MIME_JSON).
	ws.Produces(restful.MIME_JSON)
	ws.Filter(r.authMW.Authenticate)
	ws.Filter(r.authMW.RequireAuth)

	read := []string{label.PipelineTemplateRead}
	write := []string{label.PipelineTemplateWrite}

	ws.Route(ws.GET("").To(r.list).
		Doc("列出通用 pipeline 模板").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineTemplateListResponse{}).
		Returns(http.StatusOK, "templates", pipelineTemplateListResponse{}))

	ws.Route(ws.POST("").To(r.create).
		Doc("创建通用 pipeline 模板").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(pipelineTemplateCreateRequest{}).
		Writes(pipelineTemplateDetailResponse{}).
		Returns(http.StatusCreated, "created", pipelineTemplateDetailResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusConflict, "name conflict", errorResponse{}))

	ws.Route(ws.GET("/{id}").To(r.get).
		Doc("查看通用 pipeline 模板详情").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineTemplateDetailResponse{}).
		Returns(http.StatusOK, "template", pipelineTemplateDetailResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.PUT("/{id}/draft").To(r.updateDraft).
		Doc("修改通用 pipeline 模板草稿 (元数据 + draft YAML)").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(pipelineTemplateDraftRequest{}).
		Writes(pipelineTemplateDetailResponse{}).
		Returns(http.StatusOK, "updated", pipelineTemplateDetailResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	// publish 没有 body, 故意不挂 Consumes 避免 415.
	ws.Route(ws.POST("/{id}/publish").To(r.publish).
		Doc("把当前草稿发布为引用方可见的版本").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes(pipelineTemplateDetailResponse{}).
		Returns(http.StatusOK, "published", pipelineTemplateDetailResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.DELETE("/{id}").To(r.delete).
		Doc("删除通用 pipeline 模板 (仍被引用时拒绝)").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, write).
		Metadata(label.MetaModule, label.ModuleProject).
		Returns(http.StatusNoContent, "deleted", nil).
		Returns(http.StatusConflict, "in use", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.GET("/{id}/projects").To(r.referencingRepos).
		Doc("列出引用此模板的项目").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Writes([]templatesvc.ReferencingRepo{}).
		Returns(http.StatusOK, "projects", []templatesvc.ReferencingRepo{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	ws.Route(ws.POST("/{id}/render").To(r.render).
		Doc("使用变量渲染模板 (预览, 不写库)").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleProject).
		Consumes(restful.MIME_JSON).
		Reads(pipelineTemplateRenderRequest{}).
		Writes(pipelineTemplateRenderResponse{}).
		Returns(http.StatusOK, "rendered", pipelineTemplateRenderResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}))

	return []*restful.WebService{ws}
}

func (r *pipelineTemplatesRouter) list(req *restful.Request, resp *restful.Response) {
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	keyword := strings.TrimSpace(req.QueryParameter("keyword"))
	publishedOnly := strings.EqualFold(req.QueryParameter("published"), "true")
	kind := strings.TrimSpace(req.QueryParameter("kind"))

	result, err := r.services.PipelineTemplate.List(req.Request.Context(), templatesvc.ListOptions{
		Page:          page,
		PerPage:       perPage,
		OnlyPublished: publishedOnly,
		Kind:          kind,
		Keyword:       keyword,
	})
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	out := pipelineTemplateListResponse{
		Items:   make([]pipelineTemplateSummaryResponse, 0, len(result.Items)),
		Total:   result.Total,
		Page:    result.Page,
		PerPage: result.PerPage,
	}
	for _, item := range result.Items {
		out.Items = append(out.Items, pipelineTemplateSummaryResponse(item))
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, out)
}

func (r *pipelineTemplatesRouter) create(req *restful.Request, resp *restful.Response) {
	claims, _ := authmw.FromContext(req.Request.Context())
	var body pipelineTemplateCreateRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	tpl, err := r.services.PipelineTemplate.Create(req.Request.Context(), templatesvc.CreateInput{
		Name:         body.Name,
		DisplayName:  body.DisplayName,
		Description:  body.Description,
		Kind:         body.Kind,
		DraftContent: body.DraftContent,
		Actor:        actorOf(claims),
	})
	if err != nil {
		writeTemplateError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusCreated, toTemplateDetail(tpl))
}

func (r *pipelineTemplatesRouter) get(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	tpl, err := r.services.PipelineTemplate.Get(req.Request.Context(), id)
	if err != nil {
		writeTemplateError(resp, err, http.StatusInternalServerError)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, toTemplateDetail(tpl))
}

func (r *pipelineTemplatesRouter) updateDraft(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	claims, _ := authmw.FromContext(req.Request.Context())
	var body pipelineTemplateDraftRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	tpl, err := r.services.PipelineTemplate.UpdateDraft(req.Request.Context(), id, templatesvc.UpdateInput{
		DisplayName:  body.DisplayName,
		Description:  body.Description,
		DraftContent: body.DraftContent,
		Actor:        actorOf(claims),
	})
	if err != nil {
		writeTemplateError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, toTemplateDetail(tpl))
}

func (r *pipelineTemplatesRouter) publish(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	claims, _ := authmw.FromContext(req.Request.Context())
	tpl, err := r.services.PipelineTemplate.Publish(req.Request.Context(), id, actorOf(claims))
	if err != nil {
		writeTemplateError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, toTemplateDetail(tpl))
}

func (r *pipelineTemplatesRouter) delete(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if err := r.services.PipelineTemplate.Delete(req.Request.Context(), id); err != nil {
		switch {
		case errors.Is(err, templatesvc.ErrTemplateInUse):
			writeError(resp, http.StatusConflict, err)
		case errors.Is(err, templatesvc.ErrTemplateNotFound):
			writeError(resp, http.StatusNotFound, err)
		default:
			writeError(resp, http.StatusInternalServerError, err)
		}
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *pipelineTemplatesRouter) referencingRepos(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	rows, err := r.services.PipelineTemplate.ListReferencingRepos(req.Request.Context(), id)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, rows)
}

func (r *pipelineTemplatesRouter) render(req *restful.Request, resp *restful.Response) {
	id, err := parseInt64Param(req, "id")
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	var body pipelineTemplateRenderRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	// 把 body.Variables 与项目 repo 上下文合并 (repo_ctx 是基线, body 覆盖之),
	// 让预览结果与触发链路完全一致. 没传 repo_id 则只用 body.Variables.
	mergedVars := body.Variables
	if body.RepoID > 0 && r.services != nil && r.services.Repo != nil && r.services.Pipeline != nil {
		repo, repoErr := r.services.Repo.FindByID(req.Request.Context(), body.RepoID)
		if repoErr == nil && repo != nil {
			cfg, _ := r.services.Pipeline.GetPipelineConfig(req.Request.Context(), repo.ID)
			branch := strings.TrimSpace(repo.Branch)
			if branch == "" {
				branch = "main"
			}
			ctxVars := r.services.Pipeline.BuildRepoRenderContext(req.Request.Context(), repo, cfg, branch, "", "")
			merged := make(map[string]string, len(ctxVars)+len(body.Variables))
			for k, v := range ctxVars {
				merged[k] = v
			}
			for k, v := range body.Variables {
				merged[k] = v
			}
			mergedVars = merged
		}
	}
	// 用 RenderForPreview 一次拿 (rendered, missing); 凭证 fallback 在两个
	// 操作之间共享 cache, 避免预览页对同一变量两次查 DB.
	rendered, missing, err := r.services.PipelineTemplate.RenderForPreview(req.Request.Context(), id, mergedVars)
	if err != nil {
		writeTemplateError(resp, err, http.StatusBadRequest)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, pipelineTemplateRenderResponse{
		Content: rendered,
		Missing: missing,
	})
}

func writeTemplateError(resp *restful.Response, err error, fallback int) {
	switch {
	case errors.Is(err, templatesvc.ErrTemplateNotFound):
		writeError(resp, http.StatusNotFound, err)
	case errors.Is(err, templatesvc.ErrTemplateNameConflict):
		writeError(resp, http.StatusConflict, err)
	case errors.Is(err, templatesvc.ErrTemplateInUse):
		writeError(resp, http.StatusConflict, err)
	case errors.Is(err, templatesvc.ErrTemplateNotPublished),
		errors.Is(err, templatesvc.ErrTemplateDraftEmpty):
		writeError(resp, http.StatusBadRequest, err)
	default:
		writeError(resp, fallback, err)
	}
}

func toTemplateDetail(tpl *model.PipelineTemplate) pipelineTemplateDetailResponse {
	if tpl == nil {
		return pipelineTemplateDetailResponse{}
	}
	return pipelineTemplateDetailResponse{
		ID:               tpl.ID,
		Name:             tpl.Name,
		DisplayName:      tpl.DisplayName,
		Description:      tpl.Description,
		Kind:             tpl.EffectiveKind(),
		DraftContent:     tpl.DraftContent,
		PublishedContent: tpl.PublishedContent,
		IsPublished:      tpl.IsPublished(),
		PublishedAt:      tpl.PublishedAt,
		PublishedBy:      tpl.PublishedBy,
		CreatedBy:        tpl.CreatedBy,
		UpdatedBy:        tpl.UpdatedBy,
		Created:          tpl.Created,
		Updated:          tpl.Updated,
	}
}

func actorOf(claims *authsvc.SessionClaims) string {
	if claims == nil {
		return ""
	}
	if claims.Login != "" {
		return claims.Login
	}
	return strconv.FormatInt(claims.UserID, 10)
}
