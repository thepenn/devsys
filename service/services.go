package service

import (
	"context"
	"time"

	"github.com/kuzane/go-devops/internal/cache"
	"github.com/kuzane/go-devops/internal/config"
	"github.com/kuzane/go-devops/internal/store"
	"github.com/kuzane/go-devops/service/auth"
	pipelineService "github.com/kuzane/go-devops/service/pipeline"
	"github.com/kuzane/go-devops/service/pipeline/queue"
	repoService "github.com/kuzane/go-devops/service/repo"
	systemService "github.com/kuzane/go-devops/service/system"
	userService "github.com/kuzane/go-devops/service/user"
)

// Services aggregates the available service layer components.
type Services struct {
	User     *userService.Service
	Repo     *repoService.Service
	Pipeline *pipelineService.Service
	Auth     *auth.Service
	System   *systemService.Service
}

func NewServices(db *store.DB, q *queue.PipelineQueue, cache *cache.Cache, cfg *config.Config) (*Services, error) {
	pipelineOpts := []pipelineService.Option{
		pipelineService.WithWorkerCount(cfg.Pipeline.WorkerCount),
		pipelineService.WithCacheTTL(3 * time.Minute),
	}

	userSvc := userService.New(db)
	repoSvc := repoService.New(db)

	systemSvc, err := systemService.New(db)
	if err != nil {
		return nil, err
	}

	if _, err := systemSvc.GetPublicKey(context.Background()); err != nil {
		return nil, err
	}

	pipelineOpts = append(pipelineOpts, pipelineService.WithSystemService(systemSvc))
	pipelineSvc := pipelineService.NewService(db, q, cache, pipelineOpts...)
	authSvc, err := auth.New(cfg, db, userSvc, repoSvc)
	if err != nil {
		return nil, err
	}

	return &Services{
		User:     userSvc,
		Repo:     repoSvc,
		Pipeline: pipelineSvc,
		Auth:     authSvc,
		System:   systemSvc,
	}, nil
}
