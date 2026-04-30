package service

import (
	"context"
	"time"

	internalauth "github.com/thepenn/devsys/internal/auth"
	"github.com/thepenn/devsys/internal/cache"
	"github.com/thepenn/devsys/internal/config"
	"github.com/thepenn/devsys/internal/store"
	auditService "github.com/thepenn/devsys/service/audit"
	"github.com/thepenn/devsys/service/auth"
	k8s "github.com/thepenn/devsys/service/k8s"
	messageService "github.com/thepenn/devsys/service/message"
	pipelineService "github.com/thepenn/devsys/service/pipeline"
	jobService "github.com/thepenn/devsys/service/pipeline/job"
	"github.com/thepenn/devsys/service/pipeline/queue"
	templateService "github.com/thepenn/devsys/service/pipeline/template"
	rbacService "github.com/thepenn/devsys/service/rbac"
	repoService "github.com/thepenn/devsys/service/repo"
	systemService "github.com/thepenn/devsys/service/system"
	userService "github.com/thepenn/devsys/service/user"
)

// Services aggregates the available service layer components.
type Services struct {
	DB               *store.DB
	User             *userService.Service
	Repo             *repoService.Service
	Pipeline         *pipelineService.Service
	PipelineTemplate *templateService.Service
	PipelineJob      *jobService.Service
	Auth             *auth.Service
	System           *systemService.Service
	K8s              *k8s.Service
	RBAC             *rbacService.Service
	RBACEng          *internalauth.RBAC
	Audit            *auditService.Service
	Message          *messageService.Service
}

func NewServices(db *store.DB, q *queue.PipelineQueue, cache *cache.Cache, cfg *config.Config) (*Services, error) {
	pipelineOpts := []pipelineService.Option{
		pipelineService.WithWorkerCount(cfg.Pipeline.WorkerCount),
		pipelineService.WithCacheTTL(3 * time.Minute),
	}

	rbacEngine := internalauth.NewRBAC(db)
	if err := rbacEngine.Init(context.Background()); err != nil {
		return nil, err
	}

	// 复用 InjectedCache 提供的进程内 cache 作为用户角色缓存; ACL 中间件每
	// 请求一次 RoleNames 命中缓存避免 DB 流量, 写入点会显式 InvalidateRoles.
	userSvc := userService.New(db, cache)
	repoSvc := repoService.New(db)

	systemSvc, err := systemService.New(db)
	if err != nil {
		return nil, err
	}

	if _, err := systemSvc.GetPublicKey(context.Background()); err != nil {
		return nil, err
	}

	pipelineOpts = append(pipelineOpts, pipelineService.WithSystemService(systemSvc))
	// templateSvc 持有 systemSvc 用于 ${VAR} 渲染时按变量名 fallback 到
	// 凭证仓库 (git -> token, docker -> registry URL); 项目 / Job 显式
	// 提供的 vars 优先于凭证默认值.
	templateSvc := templateService.New(db, systemSvc)
	pipelineOpts = append(pipelineOpts, pipelineService.WithTemplateService(templateSvc))
	pipelineSvc := pipelineService.NewService(db, q, cache, pipelineOpts...)
	jobSvc := jobService.New(db, pipelineSvc, systemSvc)
	// 注入 cron 调度回调; 启动时 reloadCronSchedules 会装载 PipelineJob
	// 的所有 cron 表达式并由 jobSvc.TriggerCron 接管运行.
	pipelineSvc.SetJobScheduler(jobSvc)
	authSvc, err := auth.New(cfg, db, userSvc, repoSvc, rbacEngine)
	if err != nil {
		return nil, err
	}
	k8sSvc := k8s.New(systemSvc)
	auditSvc := auditService.New(db)
	messageSvc := messageService.New(db)
	rbacSvc := rbacService.New(db, rbacEngine, userSvc, messageSvc)

	return &Services{
		DB:               db,
		User:             userSvc,
		Repo:             repoSvc,
		Pipeline:         pipelineSvc,
		PipelineTemplate: templateSvc,
		PipelineJob:      jobSvc,
		Auth:             authSvc,
		System:           systemSvc,
		K8s:              k8sSvc,
		RBAC:             rbacSvc,
		RBACEng:          rbacEngine,
		Audit:            auditSvc,
		Message:          messageSvc,
	}, nil
}
