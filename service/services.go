package service

import (
	"context"
	"time"

	"github.com/thepenn/devsys/internal/cache"
	"github.com/thepenn/devsys/internal/config"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/service/auth"
	pipelineService "github.com/thepenn/devsys/service/pipeline"
	"github.com/thepenn/devsys/service/pipeline/queue"
	repoService "github.com/thepenn/devsys/service/repo"
	systemService "github.com/thepenn/devsys/service/system"
	userService "github.com/thepenn/devsys/service/user"
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
