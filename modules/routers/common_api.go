package routers

import (
	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/internal/config"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
)

type Routers struct {
	health            *health
	web               *webHandler
	auth              *authRouter
	repos             *repoRouter
	system            *systemRouter
	k8s               *k8sRouter
	rbac              *rbacRouter
	audit             *auditRouter
	messages          *messagesRouter
	pipelineTemplates *pipelineTemplatesRouter
	pipelineJobs      *pipelineJobsRouter
	services          *service.Services
	cfg               *config.Config
}

func NewRouters(cfg *config.Config, services *service.Services, authMW *authmw.Middleware) *Routers {
	return &Routers{
		health:            newHealth(services),
		web:               &webHandler{},
		auth:              newAuthRouter(services, authMW, cfg.Auth.Provider),
		repos:             newRepoRouter(services, authMW),
		k8s:               newK8sRouter(services, authMW),
		system:            newSystemRouter(services, authMW),
		rbac:              newRBACRouter(services, authMW),
		audit:             newAuditRouter(services, authMW),
		messages:          newMessagesRouter(services, authMW),
		pipelineTemplates: newPipelineTemplatesRouter(services, authMW),
		pipelineJobs:      newPipelineJobsRouter(services, authMW),
		services:          services,
		cfg:               cfg,
	}
}

func (r *Routers) Router(register func(string) *restful.WebService) []*restful.WebService {
	var ws []*restful.WebService

	{
		sysTags := []string{"系统"}
		ws = append(ws, r.health.router(register, sysTags)...)
		ws = append(ws, r.web.router(register, sysTags)...)
		ws = append(ws, r.system.router(register, sysTags)...)
		ws = append(ws, r.rbac.router(register, sysTags)...)
		ws = append(ws, r.audit.router(register, sysTags)...)
	}

	{
		authTags := []string{"认证"}
		ws = append(ws, r.auth.router(register, authTags)...)
	}

	{
		repoTags := []string{"仓库"}
		ws = append(ws, r.repos.router(register, repoTags)...)
	}

	{
		tplTags := []string{"通用 Pipeline"}
		ws = append(ws, r.pipelineTemplates.router(register, tplTags)...)
	}

	{
		jobTags := []string{"独立 Job"}
		ws = append(ws, r.pipelineJobs.router(register, jobTags)...)
	}

	{
		adminTags := []string{"Kubernetes"}
		ws = append(ws, r.k8s.router(register, adminTags)...)
	}

	{
		msgTags := []string{"消息"}
		ws = append(ws, r.messages.router(register, msgTags)...)
	}

	return ws
}
