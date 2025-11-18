package pipeline

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cron "github.com/gdgvda/cron"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thepenn/devsys/internal/cache"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
	"github.com/thepenn/devsys/service/pipeline/queue"
	dockerruntime "github.com/thepenn/devsys/service/pipeline/runtime/docker"
	"github.com/thepenn/devsys/service/pipeline/spec"
	systemsvc "github.com/thepenn/devsys/service/system"
)

const pipelineCacheKey = "pipeline:%d"

var envPlaceholderRegex = regexp.MustCompile(`\$\{(?:env\.)?([A-Za-z0-9_]+)\}`)

// Service orchestrates pipeline lifecycle operations.
type Service struct {
	db                *store.DB
	queue             *queue.PipelineQueue
	cache             *cache.Cache
	workerCount       int
	cacheTTL          time.Duration
	startOnce         sync.Once
	started           atomic.Bool
	defaultTimeout    time.Duration
	executions        sync.Map
	systemSvc         *systemsvc.Service
	scheduler         *cron.Cron
	cronEntries       map[int64][]cron.ID
	cronMu            sync.Mutex
	dockerRuntime     *dockerruntime.Runtime
	dockerRuntimeOnce sync.Once
	dockerRuntimeErr  error
}

type Option func(*Service)

type PipelineRunDetail struct {
	Pipeline  *model.Pipeline
	Workflows []*model.Workflow
	Steps     []*model.Step
	Logs      map[int64][]model.LogEntry
}

type pipelineTaskPayload struct {
	PipelineID    int64              `json:"pipeline_id"`
	RepoID        int64              `json:"repo_id"`
	Branch        string             `json:"branch"`
	Commit        string             `json:"commit"`
	Steps         []pipelineTaskStep `json:"steps"`
	RunName       string             `json:"run_name"`
	RepoURL       string             `json:"repo_url"`
	RepoClone     string             `json:"repo_clone"`
	RepoBranch    string             `json:"repo_branch"`
	WorkspaceRoot string             `json:"workspace_root"`
}

type pipelineTaskStep struct {
	PID        int                     `json:"pid"`
	Name       string                  `json:"name"`
	Image      string                  `json:"image"`
	Commands   []string                `json:"commands"`
	Secrets    []string                `json:"secrets"`
	Env        map[string]string       `json:"env,omitempty"`
	Volumes    []string                `json:"volumes,omitempty"`
	Privileged bool                    `json:"privileged,omitempty"`
	Type       model.StepType          `json:"type,omitempty"`
	Approval   *pipelineApprovalConfig `json:"approval,omitempty"`
	Plugin     *pipelinePluginConfig   `json:"plugin,omitempty"`
	Conditions *pipelineStepConditions `json:"conditions,omitempty"`
}

type pipelinePluginConfig struct {
	Settings   map[string][]string `json:"settings,omitempty"`
	Volumes    []string            `json:"volumes,omitempty"`
	Privileged bool                `json:"privileged,omitempty"`
}

type pipelineApprovalConfig struct {
	Message   string                     `json:"message"`
	Approvers []string                   `json:"approvers"`
	Timeout   int64                      `json:"timeout"`
	Strategy  model.StepApprovalStrategy `json:"strategy"`
}

type pipelineStepConditions struct {
	Branches []string `json:"branches,omitempty"`
}

func (c *pipelineStepConditions) allowsBranch(branch string) bool {
	if c == nil || len(c.Branches) == 0 {
		return true
	}
	normalized := strings.TrimSpace(branch)
	for _, candidate := range c.Branches {
		if normalized == strings.TrimSpace(candidate) {
			return true
		}
	}
	return false
}

func (c *pipelineStepConditions) branchSummary() string {
	if c == nil || len(c.Branches) == 0 {
		return ""
	}
	return strings.Join(c.Branches, ", ")
}

func (step pipelineTaskStep) allowsBranch(branch string) bool {
	if step.Conditions == nil {
		return true
	}
	return step.Conditions.allowsBranch(branch)
}

type approvalResult int

const (
	approvalResultContinue approvalResult = iota
	approvalResultWait
	approvalResultRejected
	approvalResultExpired
)

type executionHandle struct {
	cancel context.CancelFunc
}

// EnvTemplate describes a default environment variable exposed to pipeline steps.
type pipelineEnvContext struct {
	repo     *model.Repo
	pipeline *model.Pipeline
	payload  pipelineTaskPayload
}

type envProvider func(*pipelineEnvContext) map[string]string

var defaultEnvProviders = []envProvider{
	providePipelineEnv,
	provideRepoEnv,
}

// WithWorkerCount overrides the number of queue workers.
func WithWorkerCount(count int) Option {
	return func(s *Service) {
		if count > 0 {
			s.workerCount = count
		}
	}
}

// WithCacheTTL sets a TTL for pipeline cache entries.
func WithCacheTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.cacheTTL = ttl
		}
	}
}

// WithTaskTimeout defines a soft timeout for pipeline execution.
func WithTaskTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.defaultTimeout = timeout
		}
	}
}

// WithSystemService wires the system service for certificate resolution.
func WithSystemService(system *systemsvc.Service) Option {
	return func(s *Service) {
		s.systemSvc = system
	}
}

func NewService(db *store.DB, q *queue.PipelineQueue, c *cache.Cache, opts ...Option) *Service {
	s := &Service{
		db:             db,
		queue:          q,
		cache:          c,
		workerCount:    runtime.NumCPU(),
		cacheTTL:       2 * time.Minute,
		defaultTimeout: 15 * time.Minute,
		cronEntries:    make(map[int64][]cron.ID),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start initialises the queue workers.
func (s *Service) Start(ctx context.Context) error {
	var startErr error
	s.startOnce.Do(func() {
		if s.queue == nil {
			startErr = fmt.Errorf("pipeline queue not configured")
			return
		}

		if err := s.queue.Start(ctx, s.workerCount, s.handleTask); err != nil {
			startErr = err
			return
		}

		scheduler := cron.New()
		s.cronMu.Lock()
		s.scheduler = scheduler
		s.cronEntries = make(map[int64][]cron.ID)
		s.cronMu.Unlock()

		if err := s.reloadCronSchedules(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to preload cron schedules")
		}

		scheduler.Start()
		go func() {
			<-ctx.Done()
			stopCtx := scheduler.Stop()
			<-stopCtx.Done()
		}()

		s.started.Store(true)
	})
	return startErr
}

// Shutdown stops background workers.
func (s *Service) Shutdown() {
	if !s.started.Load() {
		return
	}

	var scheduler *cron.Cron

	s.cronMu.Lock()
	scheduler = s.scheduler
	s.scheduler = nil
	s.cronEntries = make(map[int64][]cron.ID)
	s.cronMu.Unlock()

	if scheduler != nil {
		stopCtx := scheduler.Stop()
		<-stopCtx.Done()
	}

	if s.queue != nil {
		s.queue.Shutdown()
	}
}

// CreatePipeline persists the pipeline and related entities.
func (s *Service) CreatePipeline(ctx context.Context, pipeline *model.Pipeline, workflows []*model.Workflow, steps []*model.Step, tasks []*model.Task) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is required")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if pipeline.Number == 0 {
			if err := tx.WithContext(ctx).
				Table("repos").
				Select("id").
				Where("id = ?", pipeline.RepoID).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Take(&struct{ ID int64 }{}).Error; err != nil {
				return err
			}

			var nextNumber int64
			if err := tx.WithContext(ctx).
				Model(&model.Pipeline{}).
				Where("repo_id = ?", pipeline.RepoID).
				Select("COALESCE(MAX(number), 0)").
				Scan(&nextNumber).Error; err != nil {
				return err
			}
			pipeline.Number = nextNumber + 1
		}

		if err := tx.WithContext(ctx).Create(pipeline).Error; err != nil {
			return err
		}

		if len(workflows) > 0 {
			for _, wf := range workflows {
				wf.PipelineID = pipeline.ID
			}
			if err := tx.WithContext(ctx).Create(&workflows).Error; err != nil {
				return err
			}
		}

		if len(steps) > 0 {
			for _, step := range steps {
				step.PipelineID = pipeline.ID
			}
			if err := tx.WithContext(ctx).Create(&steps).Error; err != nil {
				return err
			}
		}

		if len(tasks) > 0 {
			for _, task := range tasks {
				task.PipelineID = pipeline.ID
				task.RepoID = pipeline.RepoID
				if strings.TrimSpace(task.Name) == "" {
					task.Name = fmt.Sprintf("pipeline-%d", pipeline.Number)
				}
			}
			if err := tx.WithContext(ctx).Create(&tasks).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if s.cache != nil && s.cacheTTL > 0 {
		s.cache.Set(fmt.Sprintf(pipelineCacheKey, pipeline.ID), pipeline, s.cacheTTL)
	}

	return nil
}

// EnqueueTask schedules a pipeline task for execution.
func (s *Service) EnqueueTask(ctx context.Context, task *model.Task) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}

	return s.queue.Enqueue(ctx, task)
}

// GetPipeline fetches a pipeline from cache or database.
func (s *Service) GetPipeline(ctx context.Context, id int64) (*model.Pipeline, error) {
	cacheKey := fmt.Sprintf(pipelineCacheKey, id)
	if s.cache != nil {
		if cached, ok := s.cache.Get(cacheKey); ok {
			if pipeline, ok := cached.(*model.Pipeline); ok {
				return pipeline, nil
			}
		}
	}

	var pipeline model.Pipeline
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&pipeline, id).Error
	})

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if s.cache != nil && s.cacheTTL > 0 {
		s.cache.Set(cacheKey, &pipeline, s.cacheTTL)
	}

	return &pipeline, nil
}

// GetPipelineConfig returns the stored pipeline configuration for a repository.
func (s *Service) GetPipelineConfig(ctx context.Context, repoID int64) (*model.RepoPipelineConfig, error) {
	var cfg model.RepoPipelineConfig
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("repo_id = ?", repoID).
			Take(&cfg).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return normalizePipelineConfig(&cfg), nil
}

// EnsurePipelineConfig guarantees a repository has a persisted pipeline configuration.
func (s *Service) EnsurePipelineConfig(ctx context.Context, repo *model.Repo) (*model.RepoPipelineConfig, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}

	cfg, err := s.GetPipelineConfig(ctx, repo.ID)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		return cfg, nil
	}

	return s.UpsertPipelineConfig(ctx, repo.ID, "")
}

// UpsertPipelineConfig creates or updates the pipeline configuration for the given repository.
func (s *Service) UpsertPipelineConfig(ctx context.Context, repoID int64, content string) (*model.RepoPipelineConfig, error) {
	now := time.Now().Unix()
	var result *model.RepoPipelineConfig

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.RepoPipelineConfig
		err := tx.WithContext(ctx).
			Where("repo_id = ?", repoID).
			Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			cfg := defaultPipelineSettings()
			cfg.RepoID = repoID
			cfg.Content = content
			cfg.Created = now
			cfg.Updated = now
			if err := tx.WithContext(ctx).Create(cfg).Error; err != nil {
				return err
			}
			result = cfg
		case err != nil:
			return err
		default:
			existing.Content = content
			existing.Updated = now
			if err := tx.WithContext(ctx).Save(&existing).Error; err != nil {
				return err
			}
			result = &existing
		}
		if err := tx.WithContext(ctx).
			Model(&model.Repo{}).
			Where("id = ?", repoID).
			Update("active", true).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	normalized := normalizePipelineConfig(result)
	s.refreshCronEntries(repoID, normalized.CronSchedules)
	return normalized, nil
}

// TriggerManualPipeline stores a pipeline record representing a manual run against the provided configuration.
func (s *Service) TriggerManualPipeline(ctx context.Context, repo *model.Repo, author string, opts model.PipelineOptions, cfg *model.RepoPipelineConfig) (*model.Pipeline, error) {
	normalizedAuthor := strings.TrimSpace(author)
	if normalizedAuthor == "" {
		normalizedAuthor = "system"
	}
	message := fmt.Sprintf("手动触发（%s）", normalizedAuthor)
	title := fmt.Sprintf("手动触发 - %s", normalizedAuthor)
	return s.triggerPipelineWithEvent(ctx, repo, cfg, opts, model.EventManual, normalizedAuthor, message, title)
}

func (s *Service) triggerPipelineWithEvent(ctx context.Context, repo *model.Repo, cfg *model.RepoPipelineConfig, opts model.PipelineOptions, event model.WebhookEvent, author, message, title string) (*model.Pipeline, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if cfg == nil || strings.TrimSpace(cfg.Content) == "" {
		return nil, fmt.Errorf("pipeline configuration missing")
	}

	normalizedAuthor := strings.TrimSpace(author)
	if normalizedAuthor == "" {
		normalizedAuthor = "system"
	}

	now := time.Now().Unix()
	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
		if branch == "" {
			branch = "main"
		}
	}

	if opts.Variables == nil {
		opts.Variables = map[string]string{}
	}

	specDef, err := spec.Parse(cfg.Content)
	if err != nil {
		return nil, err
	}

	runMessage := strings.TrimSpace(message)
	if runMessage == "" {
		runMessage = defaultPipelineMessage(event, normalizedAuthor)
	}
	runTitle := strings.TrimSpace(title)
	if runTitle == "" {
		runTitle = fmt.Sprintf("%s run", string(event))
	}

	pipeline := &model.Pipeline{
		RepoID:              repo.ID,
		Author:              normalizedAuthor,
		Event:               event,
		Status:              model.StatusPending,
		Message:             runMessage,
		Title:               runTitle,
		Created:             now,
		Updated:             now,
		Branch:              branch,
		Ref:                 fmt.Sprintf("refs/heads/%s", branch),
		Commit:              strings.TrimSpace(opts.Commit),
		AdditionalVariables: opts.Variables,
	}

	workflow := &model.Workflow{
		PID:   1,
		Name:  firstNonEmpty(specDef.Name, "default"),
		State: model.StatusPending,
	}

	steps := make([]*model.Step, 0, len(specDef.Steps))
	taskSteps := make([]pipelineTaskStep, 0, len(specDef.Steps))
	for idx, stepSpec := range specDef.Steps {
		pid := idx + 1
		stepName := stepSpec.Name
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", pid)
		}
		stepType := model.StepTypeCommands
		var approvalModel *model.StepApproval
		var approvalTaskCfg *pipelineApprovalConfig
		if stepSpec.Kind == spec.StepKindApproval {
			stepType = model.StepTypeApproval
			strategy := model.StepApprovalStrategyAny
			if stepSpec.Approval != nil && strings.ToLower(strings.TrimSpace(stepSpec.Approval.Strategy)) == string(model.StepApprovalStrategyAll) {
				strategy = model.StepApprovalStrategyAll
			}
			approvalModel = &model.StepApproval{
				Message:   "",
				Approvers: nil,
				Strategy:  strategy,
				Timeout:   0,
				State:     model.StepApprovalStatePending,
			}
			if stepSpec.Approval != nil {
				approvalModel.Message = stepSpec.Approval.Message
				if len(stepSpec.Approval.Approvers) > 0 {
					approvalModel.Approvers = append([]string{}, stepSpec.Approval.Approvers...)
				}
				if stepSpec.Approval.Timeout > 0 {
					approvalModel.Timeout = stepSpec.Approval.Timeout
				}
			}
			approvalTaskCfg = &pipelineApprovalConfig{
				Message:   approvalModel.Message,
				Approvers: append([]string{}, approvalModel.Approvers...),
				Timeout:   approvalModel.Timeout,
				Strategy:  approvalModel.Strategy,
			}
		}
		steps = append(steps, &model.Step{
			UUID:     generateRandomID("step"),
			PID:      pid,
			PPID:     workflow.PID,
			Name:     stepName,
			State:    model.StatusPending,
			Type:     stepType,
			Approval: approvalModel,
		})
		pluginCfg, err := buildPipelinePluginConfig(stepSpec)
		if err != nil {
			return nil, err
		}
		var stepEnvVars map[string]string
		if len(stepSpec.Env) > 0 {
			stepEnvVars = cloneStringMap(stepSpec.Env)
		}
		var stepConditions *pipelineStepConditions
		if stepSpec.Conditions != nil && len(stepSpec.Conditions.Branches) > 0 {
			stepConditions = &pipelineStepConditions{
				Branches: append([]string{}, stepSpec.Conditions.Branches...),
			}
		}
		taskSteps = append(taskSteps, pipelineTaskStep{
			PID:        pid,
			Name:       stepName,
			Image:      stepSpec.Image,
			Commands:   append([]string{}, stepSpec.Commands...),
			Secrets:    stepSpec.Secrets,
			Env:        stepEnvVars,
			Volumes:    append([]string{}, stepSpec.Volumes...),
			Privileged: stepSpec.Privileged,
			Type:       stepType,
			Approval:   approvalTaskCfg,
			Plugin:     pluginCfg,
			Conditions: stepConditions,
		})
	}

	task := &model.Task{
		ID:           generateRandomID("task"),
		PID:          1,
		Name:         "",
		Dependencies: []string{},
		RunOn:        []string{string(model.StatusSuccess)},
		DepStatus:    map[string]model.StatusValue{},
		Labels:       map[string]string{},
	}
	if err := task.ApplyLabelsFromRepo(repo); err != nil {
		log.Warn().Err(err).Msg("failed to apply labels to task")
	}

	if err := s.CreatePipeline(ctx, pipeline, []*model.Workflow{workflow}, steps, []*model.Task{task}); err != nil {
		return nil, err
	}

	payload := pipelineTaskPayload{
		PipelineID:    pipeline.ID,
		RepoID:        repo.ID,
		Branch:        branch,
		Commit:        pipeline.Commit,
		RunName:       workflow.Name,
		RepoURL:       repo.ForgeURL,
		RepoClone:     repo.Clone,
		RepoBranch:    repo.Branch,
		WorkspaceRoot: specDef.Workspace,
		Steps:         taskSteps,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化流水线任务失败: %w", err)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Task{}).
			Where("id = ?", task.ID).
			Update("data", payloadBytes).Error
	}); err != nil {
		return nil, err
	}
	task.Data = payloadBytes

	if err := s.EnqueueTask(ctx, task); err != nil {
		log.Error().Err(err).Int64("pipeline_id", pipeline.ID).Str("event", string(event)).Msg("failed to enqueue pipeline task")
		_ = s.db.Transaction(func(tx *gorm.DB) error {
			return tx.WithContext(ctx).
				Model(&model.Pipeline{}).
				Where("id = ?", pipeline.ID).
				Updates(map[string]any{
					"status":  model.StatusFailure,
					"message": fmt.Sprintf("failed to enqueue pipeline task: %v", err),
				}).Error
		})
		return nil, err
	}

	if settings, err := s.GetPipelineSettings(ctx, repo.ID); err != nil {
		log.Warn().Err(err).Int64("repo_id", repo.ID).Msg("failed to load pipeline settings for retention")
	} else {
		if settings == nil {
			settings = defaultPipelineSettings()
		}
		if settings.MaxRecords <= 0 {
			settings.MaxRecords = 10
		}
		if err := s.enforcePipelineRetention(ctx, repo, settings); err != nil {
			log.Warn().Err(err).Int64("repo_id", repo.ID).Msg("failed to enforce pipeline retention")
		}
	}

	return pipeline, nil
}

// ListPipelinesByRepo returns pipelines belonging to a repository ordered by creation time descending.
func (s *Service) ListPipelinesByRepo(ctx context.Context, repoID int64, page, perPage int) ([]*model.Pipeline, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	} else if perPage > 100 {
		perPage = 100
	}

	var pipelines []*model.Pipeline
	var total int64

	err := s.db.View(func(tx *gorm.DB) error {
		query := tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("repo_id = ?", repoID)
		if err := query.Count(&total).Error; err != nil {
			return err
		}
		return query.
			Order("created DESC").
			Offset((page - 1) * perPage).
			Limit(perPage).
			Find(&pipelines).Error
	})
	if err != nil {
		return nil, 0, err
	}

	for _, pipeline := range pipelines {
		if pipeline == nil {
			continue
		}
		if strings.TrimSpace(pipeline.Message) == "" {
			pipeline.Message = defaultPipelineMessage(pipeline.Event, pipeline.Author)
		}
	}
	return pipelines, total, nil
}

// GetPipelineSettings returns repository level pipeline settings.
func (s *Service) GetPipelineSettings(ctx context.Context, repoID int64) (*model.RepoPipelineConfig, error) {
	cfg, err := s.GetPipelineConfig(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		defaults := defaultPipelineSettings()
		defaults.RepoID = repoID
		return defaults, nil
	}
	return normalizePipelineConfig(cfg), nil
}

// UpsertPipelineSettings stores repository pipeline settings.
func (s *Service) UpsertPipelineSettings(ctx context.Context, repoID int64, settings model.RepoPipelineConfig) (*model.RepoPipelineConfig, error) {
	now := time.Now().Unix()
	schedules := sanitizeCronSchedules(settings.CronSchedules)
	var result *model.RepoPipelineConfig

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.RepoPipelineConfig
		err := tx.WithContext(ctx).
			Where("repo_id = ?", repoID).
			Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			cfg := defaultPipelineSettings()
			cfg.RepoID = repoID
			cfg.Content = ""
			cfg.CleanupEnabled = settings.CleanupEnabled
			cfg.RetentionDays = settings.RetentionDays
			cfg.MaxRecords = settings.MaxRecords
			cfg.DisallowParallel = settings.DisallowParallel
			cfg.Dockerfile = settings.Dockerfile
			cfg.CronSchedules = schedules
			cfg.LegacyCronEnabled = len(schedules) > 0
			if len(schedules) > 0 {
				cfg.LegacyCronSpec = schedules[0]
			} else {
				cfg.LegacyCronSpec = ""
			}
			cfg.Created = now
			cfg.Updated = now
			if err := tx.WithContext(ctx).Create(cfg).Error; err != nil {
				return err
			}
			result = cfg
		case err != nil:
			return err
		default:
			existing.CleanupEnabled = settings.CleanupEnabled
			existing.RetentionDays = settings.RetentionDays
			existing.MaxRecords = settings.MaxRecords
			existing.DisallowParallel = settings.DisallowParallel
			existing.Dockerfile = settings.Dockerfile
			existing.CronSchedules = schedules
			existing.LegacyCronEnabled = len(schedules) > 0
			if len(schedules) > 0 {
				existing.LegacyCronSpec = schedules[0]
			} else {
				existing.LegacyCronSpec = ""
			}
			existing.Updated = now
			if err := tx.WithContext(ctx).Save(&existing).Error; err != nil {
				return err
			}
			result = &existing
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return normalizePipelineConfig(result), nil
}

// GetPipelineRunDetail returns pipeline, workflow, step and log information for a specific run.
func (s *Service) GetPipelineRunDetail(ctx context.Context, repoID, pipelineID int64) (*PipelineRunDetail, error) {
	detail := &PipelineRunDetail{
		Workflows: []*model.Workflow{},
		Steps:     []*model.Step{},
		Logs:      map[int64][]model.LogEntry{},
	}

	err := s.db.View(func(tx *gorm.DB) error {
		var pipeline model.Pipeline
		if err := tx.WithContext(ctx).
			Where("id = ? AND repo_id = ?", pipelineID, repoID).
			Take(&pipeline).Error; err != nil {
			return err
		}
		detail.Pipeline = &pipeline

		var workflows []*model.Workflow
		if err := tx.WithContext(ctx).
			Where("pipeline_id = ?", pipelineID).
			Order("pid ASC").
			Find(&workflows).Error; err != nil {
			return err
		}
		detail.Workflows = workflows

		var steps []*model.Step
		if err := tx.WithContext(ctx).
			Where("pipeline_id = ?", pipelineID).
			Order("pid ASC").
			Find(&steps).Error; err != nil {
			return err
		}
		detail.Steps = steps

		if len(steps) == 0 {
			return nil
		}

		stepIDs := make([]int64, 0, len(steps))
		for _, step := range steps {
			stepIDs = append(stepIDs, step.ID)
		}

		var logs []model.LogEntry
		if err := tx.WithContext(ctx).
			Where("step_id IN ?", stepIDs).
			Order("step_id ASC, line ASC").
			Find(&logs).Error; err != nil {
			return err
		}

		for _, entry := range logs {
			detail.Logs[entry.StepID] = append(detail.Logs[entry.StepID], entry)
		}

		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (s *Service) SubmitStepApproval(ctx context.Context, repoID, pipelineID, stepID int64, actor string, action string, comment string) (*model.Step, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("审批用户无效")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "reject" {
		return nil, fmt.Errorf("无效的审批操作")
	}
	task, err := s.findPipelineTask(ctx, pipelineID)
	if err != nil {
		return nil, err
	}
	var pipeline model.Pipeline
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&pipeline, pipelineID).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	if pipeline.RepoID != repoID {
		return nil, gorm.ErrRecordNotFound
	}
	var finalAction string
	now := time.Now().Unix()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var step model.Step
		if err := tx.WithContext(ctx).
			Where("id = ? AND pipeline_id = ?", stepID, pipelineID).
			Take(&step).Error; err != nil {
			return err
		}
		if step.Type != model.StepTypeApproval {
			return fmt.Errorf("该步骤不需要审批")
		}
		if step.Approval == nil {
			return fmt.Errorf("审批配置缺失")
		}
		approval := step.Approval
		if approval.State == model.StepApprovalStateApproved {
			return fmt.Errorf("审批已通过")
		}
		if approval.State == model.StepApprovalStateRejected || approval.State == model.StepApprovalStateExpired {
			return fmt.Errorf("审批已经结束")
		}
		if approval.Timeout > 0 && approval.RequestedAt > 0 && now >= approval.RequestedAt+approval.Timeout {
			return fmt.Errorf("审批已超时")
		}
		if len(approval.Approvers) > 0 && !containsIgnoreCase(approval.Approvers, actor) {
			return fmt.Errorf("当前用户不在审批名单中")
		}
		comments := strings.TrimSpace(comment)
		approval.Decisions = upsertApprovalDecision(approval.Decisions, model.StepApprovalDecision{
			User:      actor,
			Action:    action,
			Comment:   comments,
			Timestamp: now,
		})
		updates := map[string]any{
			"approval": approval,
		}
		switch action {
		case "reject":
			approval.State = model.StepApprovalStateRejected
			approval.FinalizedBy = actor
			approval.FinalizedAt = now
			finalAction = "rejected"
			step.State = model.StatusFailure
			step.Finished = now
			if comments == "" {
				step.Error = "审批被拒绝"
			} else {
				step.Error = comments
			}
			updates["state"] = step.State
			updates["finished"] = step.Finished
			updates["error"] = step.Error
		case "approve":
			if approval.Strategy == "" {
				approval.Strategy = model.StepApprovalStrategyAny
			}
			approvedAll := len(approval.Approvers) == 0 || approval.Strategy == model.StepApprovalStrategyAny
			if approval.Strategy == model.StepApprovalStrategyAll && len(approval.Approvers) > 0 {
				approvedAll = allApproversApproved(approval.Approvers, approval.Decisions)
			}
			if approvedAll {
				approval.State = model.StepApprovalStateApproved
				approval.FinalizedBy = actor
				approval.FinalizedAt = now
				finalAction = "approved"
				step.State = model.StatusSuccess
				step.Finished = now
				updates["state"] = step.State
				updates["finished"] = step.Finished
				updates["exit_code"] = 0
				updates["error"] = ""
			} else {
				approval.State = model.StepApprovalStatePending
			}
		}
		if err := tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("id = ?", step.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		if finalAction == "approved" {
			if err := tx.WithContext(ctx).
				Model(&model.Pipeline{}).
				Where("id = ?", pipelineID).
				Updates(map[string]any{
					"status":  model.StatusRunning,
					"message": "",
					"updated": now,
				}).Error; err != nil {
				return err
			}
			if err := tx.WithContext(ctx).
				Model(&model.Workflow{}).
				Where("pipeline_id = ?", pipelineID).
				Updates(map[string]any{
					"state": model.StatusRunning,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if finalAction == "approved" {
		if err := s.resumePipelineAfterApproval(ctx, pipelineID); err != nil {
			return nil, err
		}
	} else if finalAction == "rejected" {
		message := firstNonEmpty(strings.TrimSpace(comment), "审批被拒绝")
		taskID := ""
		if task != nil {
			taskID = task.ID
		}
		if err := s.markPipelineFinished(ctx, pipelineID, model.StatusFailure, now, message, taskID); err != nil {
			return nil, err
		}
	}
	updatedStep, err := s.getStepByID(ctx, stepID)
	if err != nil {
		return nil, err
	}
	return updatedStep, nil
}

// QueueInfo returns aggregated queue information.
func (s *Service) QueueInfo() model.QueueInfo {
	stats := s.queue.Stats()
	info := model.QueueInfo{
		Pending:       make([]model.QueueTask, 0),
		WaitingOnDeps: make([]model.QueueTask, 0),
		Running:       make([]model.QueueTask, 0),
		Paused:        !stats.Running,
	}
	info.Stats.WorkerCount = stats.Workers
	info.Stats.PendingCount = stats.Pending
	info.Stats.RunningCount = stats.InFlight
	info.Stats.WaitingOnDepsCount = 0

	return info
}

func (s *Service) handleTask(ctx context.Context, task *model.Task) error {
	started := time.Now().Unix()

	var payload pipelineTaskPayload
	if len(task.Data) > 0 {
		if err := json.Unmarshal(task.Data, &payload); err != nil {
			return fmt.Errorf("解析流水线任务失败: %w", err)
		}
	}
	if payload.PipelineID == 0 {
		payload.PipelineID = task.PipelineID
	}
	if payload.Branch == "" {
		payload.Branch = "main"
	}

	log.Info().
		Str("task_id", task.ID).
		Int64("pipeline_id", payload.PipelineID).
		Msg("starting pipeline execution")

	status, err := s.getPipelineStatus(ctx, payload.PipelineID)
	if err != nil {
		return err
	}
	if status == model.StatusKilled || status == model.StatusSuccess || status == model.StatusFailure { // already finished
		_ = s.removeTaskRecord(ctx, task.ID)
		return nil
	}

	taskCtx, cancel := context.WithCancel(ctx)
	s.executions.Store(payload.PipelineID, &executionHandle{cancel: cancel})
	defer func() {
		cancel()
		s.executions.Delete(payload.PipelineID)
	}()

	if err := s.markPipelineRunning(ctx, payload.PipelineID, started); err != nil {
		return err
	}

	stepRecords, stepMap, err := s.fetchPipelineSteps(ctx, payload.PipelineID)
	if err != nil {
		return err
	}

	repo, err := s.fetchRepo(ctx, payload.RepoID)
	if err != nil {
		return err
	}

	pipelineRecord, err := s.fetchPipeline(ctx, payload.PipelineID)
	if err != nil {
		return err
	}

	settings, err := s.GetPipelineSettings(ctx, repo.ID)
	if err != nil {
		return err
	}

	allRequested := collectRequestedAliases(payload.Steps)

	certEnv, cloneOverride, resolvedSecrets := s.buildCertificateEnv(ctx, repo, settings, allRequested)

	envMap := s.buildBaseEnv(&pipelineEnvContext{
		repo:     repo,
		pipeline: pipelineRecord,
		payload:  payload,
	})
	if envMap == nil {
		envMap = make(map[string]string)
	}

	if pipelineRecord.AdditionalVariables != nil {
		for key, value := range pipelineRecord.AdditionalVariables {
			if strings.TrimSpace(key) == "" {
				continue
			}
			envMap[key] = value
		}
	}

	for key, value := range certEnv {
		envMap[key] = value
	}
	if cloneOverride != "" {
		envMap["REPO_CLONE_URL_AUTH"] = cloneOverride
	} else if strings.TrimSpace(envMap["REPO_CLONE_URL_AUTH"]) == "" {
		envMap["REPO_CLONE_URL_AUTH"] = envMap["REPO_CLONE_URL"]
	}

	var workspace string
	var workspaceRoot string
	workspaceCleanup := false
	var workspacePrepared bool
	var pipelineStatus = model.StatusSuccess
	var failureMessage string
	dockerfileInjected := false

	pipelineEnv := make(map[string]string)

	ensureDockerfile := func(force bool, logger func(string) error) error {
		if dockerfileInjected {
			return nil
		}
		if workspace == "" {
			return nil
		}
		dockerfilePath := filepath.Join(workspace, "Dockerfile")
		if info, err := os.Stat(dockerfilePath); err == nil && !info.IsDir() {
			dockerfileInjected = true
			return nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if settings == nil || strings.TrimSpace(settings.Dockerfile) == "" {
			return fmt.Errorf("未检测到仓库中的 Dockerfile，且未在系统中定义 Dockerfile")
		}
		template := settings.Dockerfile

		if !force {
			entries, err := os.ReadDir(workspace)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return nil
			}
		}

		if err := os.WriteFile(dockerfilePath, []byte(template), 0o644); err != nil {
			return fmt.Errorf("写入 Dockerfile 失败: %w", err)
		}
		dockerfileInjected = true
		if logger != nil {
			_ = logger("未检测到仓库中的 Dockerfile，已使用系统配置的 Dockerfile")
		}
		return nil
	}

	for _, execStep := range payload.Steps {
		select {
		case <-taskCtx.Done():
			pipelineStatus = model.StatusKilled
			failureMessage = "pipeline canceled"
			break
		default:
		}

		if pipelineStatus == model.StatusKilled {
			break
		}

		stepRecord, ok := stepMap[execStep.PID]
		if !ok {
			log.Warn().Int("pid", execStep.PID).Msg("step record not found, skipping")
			continue
		}

		if stepRecord.State == model.StatusSuccess || stepRecord.State == model.StatusSkipped {
			continue
		}

		currentBranch := strings.TrimSpace(firstNonEmpty(payload.Branch, pipelineRecord.Branch))
		if !execStep.allowsBranch(currentBranch) {
			summary := ""
			if execStep.Conditions != nil {
				summary = execStep.Conditions.branchSummary()
			}
			logMessage := "步骤因分支条件被跳过"
			switch {
			case summary != "" && currentBranch != "":
				logMessage = fmt.Sprintf("%s（当前分支 %s，仅在 %s 执行）", logMessage, currentBranch, summary)
			case summary != "":
				logMessage = fmt.Sprintf("%s（要求分支：%s）", logMessage, summary)
			case currentBranch != "":
				logMessage = fmt.Sprintf("%s（当前分支：%s）", logMessage, currentBranch)
			}
			if err := s.appendLogLine(ctx, stepRecord.ID, nil, logMessage); err != nil {
				return err
			}
			if err := s.setStepFinished(ctx, stepRecord.ID, model.StatusSkipped, time.Now().Unix(), nil, -1); err != nil {
				return err
			}
			continue
		}

		stepStart := time.Now().Unix()
		if err := s.setStepRunning(ctx, stepRecord.ID, stepStart); err != nil {
			return err
		}

		lineCounter := 1
		logFn := func(message string) error {
			return s.appendLogLine(ctx, stepRecord.ID, &lineCounter, message)
		}

		if strings.TrimSpace(execStep.Image) != "" {
			_ = logFn(fmt.Sprintf("镜像: %s", execStep.Image))
		}

		if execStep.Type == model.StepTypeApproval {
			result, err := s.processApprovalStep(ctx, pipelineRecord, stepRecord, execStep, logFn)
			if err != nil {
				pipelineStatus = model.StatusFailure
				failureMessage = err.Error()
				break
			}
			switch result {
			case approvalResultContinue:
				continue
			case approvalResultWait:
				pipelineStatus = model.StatusBlocked
				failureMessage = ""
				message := "等待审批"
				if execStep.Approval != nil && strings.TrimSpace(execStep.Approval.Message) != "" {
					message = execStep.Approval.Message
				}
				if err := s.markPipelineBlocked(ctx, pipelineRecord.ID, message); err != nil {
					return err
				}
				return nil
			case approvalResultRejected:
				pipelineStatus = model.StatusFailure
				if strings.TrimSpace(stepRecord.Error) != "" {
					failureMessage = stepRecord.Error
				} else {
					failureMessage = "审批已拒绝"
				}
				break
			case approvalResultExpired:
				pipelineStatus = model.StatusFailure
				if strings.TrimSpace(stepRecord.Error) != "" {
					failureMessage = stepRecord.Error
				} else {
					failureMessage = "审批已超时"
				}
				break
			}
			break
		}

		if !workspacePrepared {
			var prepareErr error
			workspace, workspaceRoot, prepareErr = s.prepareWorkspace(taskCtx, repo, pipelineRecord.ID, payload.WorkspaceRoot, envMapToSlice(envMap), logFn)
			if prepareErr != nil {
				if errors.Is(prepareErr, context.Canceled) {
					pipelineStatus = model.StatusKilled
					failureMessage = "pipeline canceled"
				} else {
					pipelineStatus = model.StatusFailure
					failureMessage = prepareErr.Error()
				}
				_ = s.setStepFinished(ctx, stepRecord.ID, statusFromPipeline(pipelineStatus), time.Now().Unix(), prepareErr, -1)
				break
			}
			workspacePrepared = true
			if settings != nil {
				workspaceCleanup = settings.CleanupEnabled
			}
			if strings.TrimSpace(payload.WorkspaceRoot) != "" {
				workspaceCleanup = false
			}
			if workspaceCleanup {
				defer os.RemoveAll(workspace)
			}

			envMap["WORKSPACE_ROOT"] = workspaceRoot
			envMap["CI_WORKSPACE_ROOT"] = workspaceRoot
			envMap["WORKSPACE"] = workspace
			envMap["CI_WORKSPACE"] = workspace
			envMap["APP_NAME"] = repo.Name
			envMap["APP_OWNER"] = repo.Owner
			envMap["REPO_CLONE_PATH"] = workspace
			if logFn != nil {
				_ = logFn(fmt.Sprintf("Workspace directory: %s", workspace))
			}
		}

		envMap["CI_STEP_NAME"] = execStep.Name
		envMap["CI_STEP_IMAGE"] = execStep.Image

		stepEnv := cloneStringMap(envMap)
		for key, value := range pipelineEnv {
			stepEnv[key] = value
		}
		placeholderEnv := cloneStringMap(pipelineEnv)

		stepSecrets := make(map[string]resolvedSecretBinding)
		for _, alias := range execStep.Secrets {
			aliasKey := strings.ToLower(strings.TrimSpace(alias))
			if aliasKey == "" {
				continue
			}
			binding, ok := resolvedSecrets[aliasKey]
			if !ok {
				err := fmt.Errorf("流水线步骤 %s 引用了未绑定的凭证 %s", execStep.Name, alias)
				_ = logFn(err.Error())
				pipelineStatus = model.StatusFailure
				failureMessage = err.Error()
				_ = s.setStepFinished(ctx, stepRecord.ID, statusFromPipeline(pipelineStatus), time.Now().Unix(), err, -1)
				break
			}
			stepSecrets[aliasKey] = binding
		}
		if pipelineStatus == model.StatusFailure {
			break
		}

		preStepEnv, postStepEnv := prepareStepEnv(execStep.Env, stepSecrets, placeholderEnv)
		for key, value := range preStepEnv {
			stepEnv[key] = value
			placeholderEnv[key] = value
		}

		pluginEnv := buildPluginEnv(execStep)
		if len(pluginEnv) > 0 {
			pluginEnv = applySecretPlaceholdersToMap(pluginEnv, stepSecrets)
			// use full step env so placeholders like ${CI_REPO_NAME} resolve
			pluginEnv = applyEnvPlaceholdersToMap(pluginEnv, stepEnv)
			for key, value := range pluginEnv {
				stepEnv[key] = value
			}
		}

		usePluginRuntime := execStep.Plugin != nil && len(execStep.Commands) == 0
		commands := append([]string{}, execStep.Commands...)
		commands = applySecretPlaceholders(commands, stepSecrets)
		maskFn := buildSecretMasker(stepSecrets)

		preHook := func(command string) error {
			if workspace == "" {
				return nil
			}
			lower := strings.ToLower(command)
			if strings.Contains(lower, "docker build") {
				return ensureDockerfile(true, logFn)
			}
			return nil
		}

		postHook := func(string) error {
			if workspace == "" {
				return nil
			}
			return ensureDockerfile(false, logFn)
		}

		if usePluginRuntime {
			exitCode, err := s.runPluginStep(taskCtx, execStep, stepEnv, workspace, execStep.Plugin, ensureDockerfile, logFn)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					pipelineStatus = model.StatusKilled
					failureMessage = "pipeline canceled"
				} else {
					pipelineStatus = model.StatusFailure
					failureMessage = err.Error()
				}
				_ = s.setStepFinished(ctx, stepRecord.ID, statusFromPipeline(pipelineStatus), time.Now().Unix(), err, exitCode)
				break
			}
			if err := s.setStepFinished(ctx, stepRecord.ID, model.StatusSuccess, time.Now().Unix(), nil, 0); err != nil {
				return err
			}
			pipelineEnv = placeholderEnv
			continue
		}

		exitCode, err := s.executeCommands(taskCtx, execStep, workspace, commands, stepEnv, logFn, maskFn, preHook, postHook)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				pipelineStatus = model.StatusKilled
				failureMessage = "pipeline canceled"
			} else {
				pipelineStatus = model.StatusFailure
				failureMessage = err.Error()
			}
			_ = s.setStepFinished(ctx, stepRecord.ID, statusFromPipeline(pipelineStatus), time.Now().Unix(), err, exitCode)
			break
		}

		postEnvValues, err := s.evaluateStepEnvCommands(taskCtx, workspace, postStepEnv, stepEnv, logFn)
		if err != nil {
			pipelineStatus = model.StatusFailure
			failureMessage = err.Error()
			_ = s.setStepFinished(ctx, stepRecord.ID, statusFromPipeline(pipelineStatus), time.Now().Unix(), err, -1)
			break
		}
		for key, value := range postEnvValues {
			stepEnv[key] = value
			placeholderEnv[key] = value
		}

		if strings.TrimSpace(pipelineRecord.Commit) == "" && workspace != "" {
			if commit, err := resolveWorkspaceCommit(taskCtx, workspace); err == nil && commit != "" {
				if err := s.updatePipelineCommit(ctx, pipelineRecord.ID, commit); err != nil {
					log.Warn().Err(err).Int64("pipeline_id", pipelineRecord.ID).Msg("failed to persist resolved commit")
				} else {
					pipelineRecord.Commit = commit
				}
				updateCommitEnv := func(target map[string]string) {
					if target == nil {
						return
					}
					target["CI_COMMIT_SHA"] = commit
					target["COMMIT_ID"] = commit
					target["COMMIT_ID_SHA"] = commit
				}
				updateCommitEnv(envMap)
				updateCommitEnv(stepEnv)
				updateCommitEnv(placeholderEnv)
				updateCommitEnv(pipelineEnv)
			}
		}

		if err := s.setStepFinished(ctx, stepRecord.ID, model.StatusSuccess, time.Now().Unix(), nil, 0); err != nil {
			return err
		}

		pipelineEnv = placeholderEnv
	}

	finished := time.Now().Unix()
	for _, step := range stepRecords {
		if step.State == model.StatusPending {
			_ = s.setStepFinished(ctx, step.ID, statusFromPipeline(pipelineStatus), finished, nil, 0)
		}
	}

	if err := s.markPipelineFinished(ctx, payload.PipelineID, pipelineStatus, finished, failureMessage, task.ID); err != nil {
		return err
	}

	if pipelineStatus == model.StatusSuccess {
		log.Info().
			Str("task_id", task.ID).
			Int64("pipeline_id", payload.PipelineID).
			Msg("pipeline execution completed")
	} else if pipelineStatus == model.StatusKilled {
		log.Warn().
			Str("task_id", task.ID).
			Int64("pipeline_id", payload.PipelineID).
			Msg("pipeline execution canceled")
	} else {
		log.Warn().
			Str("task_id", task.ID).
			Int64("pipeline_id", payload.PipelineID).
			Msg("pipeline execution failed")
	}
	return nil
}

func (s *Service) markPipelineRunning(ctx context.Context, pipelineID int64, started int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("id = ?", pipelineID).
			Updates(map[string]any{
				"status":  model.StatusRunning,
				"started": started,
				"updated": started,
			}).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).
			Model(&model.Workflow{}).
			Where("pipeline_id = ?", pipelineID).
			Updates(map[string]any{
				"state":   model.StatusRunning,
				"started": started,
			}).Error
	})
}

func (s *Service) fetchPipelineSteps(ctx context.Context, pipelineID int64) ([]model.Step, map[int]*model.Step, error) {
	var steps []model.Step
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("pipeline_id = ?", pipelineID).
			Order("pid ASC").
			Find(&steps).Error
	})
	if err != nil {
		return nil, nil, err
	}
	stepMap := make(map[int]*model.Step, len(steps))
	for i := range steps {
		stepMap[steps[i].PID] = &steps[i]
	}
	return steps, stepMap, nil
}

func (s *Service) fetchRepo(ctx context.Context, repoID int64) (*model.Repo, error) {
	var repo model.Repo
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&repo, repoID).Error
	})
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

func (s *Service) fetchPipeline(ctx context.Context, pipelineID int64) (*model.Pipeline, error) {
	var pipeline model.Pipeline
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&pipeline, pipelineID).Error
	})
	if err != nil {
		return nil, err
	}
	return &pipeline, nil
}

func (s *Service) prepareWorkspace(ctx context.Context, repo *model.Repo, pipelineID int64, workspaceRoot string, env []string, logFn func(string) error) (string, string, error) {
	if repo == nil {
		return "", "", fmt.Errorf("仓库信息缺失，无法执行构建")
	}

	rootDir := sanitizeWorkspaceRoot(workspaceRoot)
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return "", "", err
	}

	projectName := sanitizeDirName(repo.Name)
	if projectName == "" {
		projectName = fmt.Sprintf("repo-%d", repo.ID)
	}

	workspace := filepath.Join(rootDir, projectName, fmt.Sprintf("%d", pipelineID))
	if err := os.RemoveAll(workspace); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", "", err
	}
	return workspace, rootDir, nil
}

func (s *Service) executeCommands(ctx context.Context, step pipelineTaskStep, workspace string, commands []string, stepEnv map[string]string, logFn func(string) error, maskFn func(string) string, preCommand func(string) error, postCommand func(string) error) (int, error) {
	if maskFn == nil {
		maskFn = func(s string) string { return s }
	}
	if strings.TrimSpace(workspace) == "" {
		return -1, fmt.Errorf("workspace not prepared")
	}
	runner, err := s.dockerRunner()
	if err != nil {
		return -1, err
	}
	envSlice := envMapToSlice(stepEnv)
	maskedLog := func(message string) error {
		if logFn == nil {
			return nil
		}
		return logFn(maskFn(message))
	}
	cfgTemplate := dockerruntime.ContainerConfig{
		Image:      step.Image,
		Entrypoint: []string{},
		Env:        envSlice,
		WorkingDir: "/workspace",
		Volumes:    map[string]struct{}{"/workspace": {}},
		Binds:      []string{fmt.Sprintf("%s:/workspace", workspace)},
		Privileged: step.Privileged,
	}
	for _, volume := range step.Volumes {
		if strings.TrimSpace(volume) != "" {
			cfgTemplate.Binds = append(cfgTemplate.Binds, volume)
		}
	}
	var lastExitCode int
	for idx, raw := range commands {
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		displayCmd := applyEnvPlaceholderToString(cmd, stepEnv)
		if err := maskedLog(fmt.Sprintf("$ %s", displayCmd)); err != nil {
			return -1, err
		}
		if preCommand != nil {
			if err := preCommand(cmd); err != nil {
				return -1, err
			}
		}
		cfg := cfgTemplate
		cfg.Name = commandContainerName(step, stepEnv, idx)
		cfg.Cmd = []string{"/bin/sh", "-c", cmd}
		exitCode, runErr := runner.Run(ctx, cfg, func(line string) error {
			if logFn == nil {
				return nil
			}
			return logFn(maskFn(line))
		})
		lastExitCode = exitCode
		if runErr != nil {
			return lastExitCode, runErr
		}
		if postCommand != nil {
			if err := postCommand(cmd); err != nil {
				return lastExitCode, err
			}
		}
	}
	return lastExitCode, nil
}

func (s *Service) appendLogLine(ctx context.Context, stepID int64, line *int, content string) error {
	if line == nil {
		dummy := 1
		line = &dummy
	}
	entry := model.LogEntry{
		StepID:  stepID,
		Time:    time.Now().Unix(),
		Line:    *line,
		Data:    []byte(content + "\n"),
		Created: time.Now().Unix(),
		Type:    model.LogEntryStdout,
	}
	if err := s.db.GetDB().WithContext(ctx).Create(&entry).Error; err != nil {
		return err
	}
	*line++
	return nil
}

func (s *Service) setStepRunning(ctx context.Context, stepID int64, started int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("id = ?", stepID).
			Updates(map[string]any{
				"state":   model.StatusRunning,
				"started": started,
			}).Error
	})
}

func (s *Service) setStepFinished(ctx context.Context, stepID int64, status model.StatusValue, finished int64, errCause error, exitCode int) error {
	update := map[string]any{
		"state":    status,
		"finished": finished,
	}
	if errCause != nil {
		update["error"] = errCause.Error()
		update["failure"] = model.FailureFail
	}
	if errCause == nil {
		update["error"] = ""
		update["failure"] = ""
	}
	if exitCode >= 0 {
		update["exit_code"] = exitCode
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("id = ?", stepID).
			Updates(update).Error
	})
}

func (s *Service) markPipelineFinished(ctx context.Context, pipelineID int64, status model.StatusValue, finished int64, message string, taskID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		update := map[string]any{
			"status":   status,
			"finished": finished,
			"updated":  finished,
		}
		if strings.TrimSpace(message) != "" {
			update["message"] = message
		}
		if err := tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("id = ?", pipelineID).
			Updates(update).Error; err != nil {
			return err
		}

		if err := tx.WithContext(ctx).
			Model(&model.Workflow{}).
			Where("pipeline_id = ?", pipelineID).
			Updates(map[string]any{
				"state":    status,
				"finished": finished,
			}).Error; err != nil {
			return err
		}

		if taskID != "" {
			if err := tx.WithContext(ctx).Delete(&model.Task{}, "id = ?", taskID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func readCommandOutput(reader *bufio.Reader) (string, error) {
	var builder strings.Builder
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if builder.Len() == 0 {
				return "", err
			}
			return builder.String(), err
		}
		if b == '\n' || b == '\r' {
			if builder.Len() == 0 {
				continue
			}
			break
		}
		builder.WriteByte(b)
	}
	return builder.String(), nil
}

func runShellCommandCapture(ctx context.Context, dir, command string, env []string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	shell := "bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-lc", command)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	} else {
		cmd.Env = os.Environ()
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func runShellCommand(ctx context.Context, dir, command string, env []string, logFn func(string) error) error {
	shell := "bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "sh"
	}
	return runCommandWithLogging(ctx, dir, shell, []string{"-lc", command}, env, logFn)
}

func runCommandWithLogging(ctx context.Context, dir, name string, args []string, env []string, logFn func(string) error) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	} else {
		cmd.Env = os.Environ()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	stream := func(r io.Reader) {
		defer wg.Done()
		reader := bufio.NewReader(r)
		for {
			line, err := readCommandOutput(reader)
			if line != "" && logFn != nil {
				_ = logFn(line)
			}
			if err != nil {
				if !errors.Is(err, io.EOF) && logFn != nil {
					_ = logFn(fmt.Sprintf("command stream error: %v", err))
				}
				break
			}
		}
	}

	go stream(stdout)
	go stream(stderr)

	wg.Wait()
	return cmd.Wait()
}

func (s *Service) buildBaseEnv(ctx *pipelineEnvContext) map[string]string {
	env := envMapFromOS()
	for _, provider := range defaultEnvProviders {
		env = mergeEnv(env, provider(ctx))
	}
	return env
}

func mergeEnv(dst map[string]string, src map[string]string) map[string]string {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func envMapFromOS() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}
		if strings.TrimSpace(key) != "" {
			env[key] = value
		}
	}
	return env
}

func envMapToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, env[key]))
	}
	return result
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func providePipelineEnv(ctx *pipelineEnvContext) map[string]string {
	if ctx == nil || ctx.pipeline == nil {
		return nil
	}
	runName := firstNonEmpty(ctx.payload.RunName, ctx.pipeline.Title)
	branch := firstNonEmpty(ctx.payload.Branch, ctx.pipeline.Branch)
	env := map[string]string{
		"CI":                 "true",
		"CI_PIPELINE_ID":     fmt.Sprintf("%d", ctx.pipeline.ID),
		"CI_PIPELINE_NUMBER": fmt.Sprintf("%d", ctx.pipeline.Number),
		"CI_PIPELINE_NAME":   runName,
		"CI_PIPELINE_AUTHOR": ctx.pipeline.Author,
		"CI_PIPELINE_BRANCH": branch,
		"CI_COMMIT_BRANCH":   branch,
	}
	commit := strings.TrimSpace(ctx.pipeline.Commit)
	env["CI_COMMIT_SHA"] = commit
	env["COMMIT_ID"] = commit
	env["COMMIT_ID_SHA"] = commit
	return env
}

func provideRepoEnv(ctx *pipelineEnvContext) map[string]string {
	if ctx == nil || ctx.repo == nil {
		return nil
	}
	repo := ctx.repo
	cloneURL := strings.TrimSpace(repo.Clone)
	if cloneURL == "" && ctx.payload.RepoClone != "" {
		cloneURL = strings.TrimSpace(ctx.payload.RepoClone)
	}
	if cloneURL == "" {
		cloneURL = strings.TrimSpace(ctx.payload.RepoURL)
	}
	if cloneURL == "" {
		cloneURL = strings.TrimSpace(repo.ForgeURL)
	}
	env := map[string]string{
		"CI_REPO_ID":          fmt.Sprintf("%d", repo.ID),
		"CI_REPO_NAME":        repo.Name,
		"CI_REPO_OWNER":       repo.Owner,
		"CI_REPO_FULL_NAME":   repo.FullName,
		"CI_DEFAULT_BRANCH":   repo.Branch,
		"REPO_URL":            cloneURL,
		"REPO_CLONE_URL":      cloneURL,
		"REPO_CLONE_URL_AUTH": cloneURL,
		"REPO_WEB_URL":        repo.ForgeURL,
		"REPO_OWNER":          repo.Owner,
	}
	return env
}

func collectRequestedAliases(steps []pipelineTaskStep) map[string]string {
	set := make(map[string]string)
	for _, step := range steps {
		for _, alias := range step.Secrets {
			trimmed := strings.ToLower(strings.TrimSpace(alias))
			if trimmed == "" {
				continue
			}
			if _, exists := set[trimmed]; !exists {
				set[trimmed] = strings.TrimSpace(alias)
			}
		}
	}
	return set
}

type resolvedSecretBinding struct {
	Alias          string
	SanitizedAlias string
	Type           string
	Values         map[string]string
}

func applySecretPlaceholders(commands []string, bindings map[string]resolvedSecretBinding) []string {
	if len(bindings) == 0 {
		return commands
	}
	result := make([]string, len(commands))
	for idx, cmd := range commands {
		replaced := cmd
		for _, binding := range bindings {
			for key, value := range binding.Values {
				placeholder := fmt.Sprintf("${%s.%s}", binding.Alias, key)
				replaced = strings.ReplaceAll(replaced, placeholder, value)
				// also support sanitized alias usage just in case
				placeholderLowerSanitized := fmt.Sprintf("${%s.%s}", strings.ToLower(binding.SanitizedAlias), key)
				replaced = strings.ReplaceAll(replaced, placeholderLowerSanitized, value)
				placeholderUpperSanitized := fmt.Sprintf("${%s.%s}", binding.SanitizedAlias, key)
				replaced = strings.ReplaceAll(replaced, placeholderUpperSanitized, value)
			}
		}
		result[idx] = replaced
	}
	return result
}

func buildSecretMasker(bindings map[string]resolvedSecretBinding) func(string) string {
	if len(bindings) == 0 {
		return maskSensitiveValues
	}
	values := make([]string, 0)
	for _, binding := range bindings {
		for _, value := range binding.Values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return maskSensitiveValues
	}
	return func(message string) string {
		masked := message
		for _, secret := range values {
			masked = strings.ReplaceAll(masked, secret, "***")
		}
		return maskSensitiveValues(masked)
	}
}

func maskSensitiveValues(message string) string {
	lines := strings.Split(message, "\n")
	for i, line := range lines {
		lines[i] = maskSensitiveLine(line)
	}
	return strings.Join(lines, "\n")
}

func maskSensitiveLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line
	}
	if idx := strings.Index(line, "="); idx > 0 {
		key := strings.TrimSpace(line[:idx])
		value := line[idx+1:]
		if shouldMaskKey(key) {
			return fmt.Sprintf("%s=***", key)
		}
		if strings.EqualFold(key, "REPO_CLONE_URL_AUTH") {
			return fmt.Sprintf("%s=%s", key, maskCloneURL(value))
		}
		if shouldMaskValue(value) {
			return fmt.Sprintf("%s=***", key)
		}
	}
	if containsSensitiveKeyword(trimmed) {
		return sensitiveInlinePattern.ReplaceAllStringFunc(line, func(match string) string {
			if idx := strings.Index(match, "="); idx > -1 {
				return match[:idx+1] + "***"
			}
			return "***"
		})
	}
	return line
}

func shouldMaskKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret")
}

func shouldMaskValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret")
}

func containsSensitiveKeyword(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret")
}

var sensitiveInlinePattern = regexp.MustCompile(`(?i)(password|token|secret)(=[^\s]*)?`)

func maskCloneURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "***"
	}
	if parsed, err := url.Parse(value); err == nil {
		if parsed.User != nil {
			username := parsed.User.Username()
			if username != "" {
				parsed.User = url.UserPassword(username, "***")
			} else {
				parsed.User = url.User("***")
			}
			return parsed.String()
		}
	}
	if idx := strings.Index(value, "@"); idx > -1 {
		return "***@" + value[idx+1:]
	}
	return "***"
}

func sanitizeAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	alias = strings.ToUpper(alias)
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range alias {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func sanitizeDirName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	result := strings.Trim(builder.String(), "-._")
	if result == "" {
		return "project"
	}
	return result
}

func sanitizeWorkspaceRoot(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return filepath.Join(os.TempDir(), "go-devops-workspace")
	}
	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(os.TempDir(), cleaned)
	}
	return cleaned
}

func buildPipelinePluginConfig(step spec.StepSpec) (*pipelinePluginConfig, error) {
	if step.Settings == nil && len(step.Volumes) == 0 && !step.Privileged {
		return nil, nil
	}
	settings, err := normalizePluginSettings(step.Settings)
	if err != nil {
		return nil, fmt.Errorf("解析步骤 %q 的 settings 失败: %w", step.Name, err)
	}
	cfg := &pipelinePluginConfig{
		Settings:   settings,
		Volumes:    append([]string{}, step.Volumes...),
		Privileged: step.Privileged,
	}
	if len(cfg.Settings) == 0 {
		cfg.Settings = nil
	}
	if len(cfg.Volumes) == 0 {
		cfg.Volumes = nil
	}
	return cfg, nil
}

func normalizePluginSettings(raw map[string]any) (map[string][]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	result := make(map[string][]string, len(raw))
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		values, err := coerceToStringSlice(value)
		if err != nil {
			return nil, fmt.Errorf("无效的设置 %q: %w", key, err)
		}
		result[trimmedKey] = values
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func coerceToStringSlice(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return []string{""}, nil
	case string:
		return []string{v}, nil
	case []string:
		if len(v) == 0 {
			return nil, nil
		}
		out := make([]string, len(v))
		copy(out, v)
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			switch elem := item.(type) {
			case string:
				out = append(out, elem)
			case fmt.Stringer:
				out = append(out, elem.String())
			default:
				out = append(out, fmt.Sprint(elem))
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case fmt.Stringer:
		return []string{v.String()}, nil
	case bool, int, int64, float64, float32, uint, uint64, uint32, int32, int16, uint16, int8, uint8:
		return []string{fmt.Sprint(v)}, nil
	default:
		return nil, fmt.Errorf("不支持的类型 %T", value)
	}
}

func buildPluginEnv(step pipelineTaskStep) map[string]string {
	if step.Plugin == nil || len(step.Plugin.Settings) == 0 {
		return map[string]string{}
	}
	env := make(map[string]string, len(step.Plugin.Settings))
	for key, values := range step.Plugin.Settings {
		sanitized := sanitizeAlias(key)
		if sanitized == "" {
			continue
		}
		envKey := fmt.Sprintf("PLUGIN_%s", sanitized)
		env[envKey] = strings.Join(values, "\n")
	}
	return env
}

func applySecretPlaceholdersToMap(values map[string]string, bindings map[string]resolvedSecretBinding) map[string]string {
	if len(values) == 0 {
		return values
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = applySecretPlaceholderToString(value, bindings)
	}
	return result
}

func applySecretPlaceholderToString(value string, bindings map[string]resolvedSecretBinding) string {
	replaced := applySecretPlaceholders([]string{value}, bindings)
	if len(replaced) == 0 {
		return value
	}
	return replaced[0]
}

func applyEnvPlaceholders(commands []string, env map[string]string) []string {
	if len(commands) == 0 || len(env) == 0 {
		return commands
	}
	result := make([]string, len(commands))
	for i, cmd := range commands {
		result[i] = applyEnvPlaceholderToString(cmd, env)
	}
	return result
}

func applyEnvPlaceholdersToMap(values map[string]string, env map[string]string) map[string]string {
	if len(values) == 0 || len(env) == 0 {
		return values
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = applyEnvPlaceholderToString(value, env)
	}
	return result
}

func applyEnvPlaceholderToString(value string, env map[string]string) string {
	if len(env) == 0 || value == "" {
		return value
	}
	return envPlaceholderRegex.ReplaceAllStringFunc(value, func(match string) string {
		matches := envPlaceholderRegex.FindStringSubmatch(match)
		if len(matches) != 2 {
			return match
		}
		if replacement, ok := env[matches[1]]; ok {
			return replacement
		}
		return match
	})
}

func prepareStepEnv(definitions map[string]string, secrets map[string]resolvedSecretBinding, pipelineEnv map[string]string) (map[string]string, map[string]string) {
	if len(definitions) == 0 {
		return nil, nil
	}
	pre := make(map[string]string)
	post := make(map[string]string)
	for key, raw := range definitions {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		resolved := applySecretPlaceholderToString(raw, secrets)
		resolved = applyEnvPlaceholderToString(resolved, pipelineEnv)
		trimmedValue := strings.TrimSpace(resolved)
		if strings.HasPrefix(trimmedValue, "$(") && strings.HasSuffix(trimmedValue, ")") {
			post[trimmedKey] = trimmedValue
		} else {
			pre[trimmedKey] = resolved
		}
	}
	if len(pre) == 0 {
		pre = nil
	}
	if len(post) == 0 {
		post = nil
	}
	return pre, post
}

func (s *Service) evaluateStepEnvCommands(ctx context.Context, workspace string, definitions map[string]string, baseEnv map[string]string, logFn func(string) error) (map[string]string, error) {
	if len(definitions) == 0 {
		return nil, nil
	}
	results := make(map[string]string, len(definitions))
	runtimeEnv := cloneStringMap(baseEnv)
	for key, expression := range definitions {
		command := strings.TrimSpace(expression)
		if strings.HasPrefix(command, "$(") && strings.HasSuffix(command, ")") {
			command = strings.TrimSpace(command[2 : len(command)-1])
		}
		if command == "" {
			results[key] = ""
			runtimeEnv[key] = ""
			continue
		}
		output, err := runShellCommandCapture(ctx, workspace, command, envMapToSlice(runtimeEnv))
		if err != nil {
			return nil, fmt.Errorf("解析环境变量 %s 失败: %w", key, err)
		}
		value := strings.TrimSpace(output)
		results[key] = value
		runtimeEnv[key] = value
		if logFn != nil {
			_ = logFn(fmt.Sprintf("环境变量 %s 已更新", key))
		}
	}
	return results, nil
}

func pluginContainerName(step pipelineTaskStep, env map[string]string) string {
	base := sanitizeContainerName(step.Name)
	if base == "" {
		base = "plugin"
	}
	if pipelineID := strings.TrimSpace(env["CI_PIPELINE_ID"]); pipelineID != "" {
		base = fmt.Sprintf("%s-%s", base, pipelineID)
	}
	if step.PID > 0 {
		base = fmt.Sprintf("%s-%d", base, step.PID)
	}
	return sanitizeContainerName(base)
}

func commandContainerName(step pipelineTaskStep, env map[string]string, index int) string {
	base := sanitizeContainerName(step.Name)
	if base == "" {
		base = "step"
	}
	if pipelineID := strings.TrimSpace(env["CI_PIPELINE_ID"]); pipelineID != "" {
		base = fmt.Sprintf("%s-%s", base, pipelineID)
	}
	if step.PID > 0 {
		base = fmt.Sprintf("%s-%d", base, step.PID)
	}
	if index >= 0 {
		base = fmt.Sprintf("%s-c%d", base, index+1)
	}
	return sanitizeContainerName(base)
}

func (s *Service) runPluginStep(ctx context.Context, step pipelineTaskStep, stepEnv map[string]string, workspace string, pluginCfg *pipelinePluginConfig, ensureDockerfile func(bool, func(string) error) error, logFn func(string) error) (int, error) {
	if pluginCfg == nil {
		return -1, fmt.Errorf("plugin configuration missing")
	}
	if strings.TrimSpace(workspace) == "" {
		return -1, fmt.Errorf("workspace not prepared")
	}
	runner, err := s.dockerRunner()
	if err != nil {
		return -1, err
	}
	if ensureDockerfile != nil {
		if err := ensureDockerfile(true, logFn); err != nil {
			return -1, err
		}
		defer ensureDockerfile(false, logFn)
	}
	binds := []string{fmt.Sprintf("%s:/workspace", workspace)}
	for _, volume := range pluginCfg.Volumes {
		if strings.TrimSpace(volume) != "" {
			binds = append(binds, volume)
		}
	}
	cfg := dockerruntime.ContainerConfig{
		Name:       pluginContainerName(step, stepEnv),
		Image:      step.Image,
		Env:        envMapToSlice(pluginContainerEnv(stepEnv)),
		WorkingDir: "/workspace",
		Volumes:    map[string]struct{}{"/workspace": {}},
		Binds:      binds,
		Privileged: pluginCfg.Privileged,
	}
	if len(step.Commands) > 0 {
		cfg.Cmd = append([]string{}, step.Commands...)
	}
	return runner.Run(ctx, cfg, logFn)
}

func (s *Service) dockerRunner() (*dockerruntime.Runtime, error) {
	s.dockerRuntimeOnce.Do(func() {
		s.dockerRuntime, s.dockerRuntimeErr = dockerruntime.NewRuntime()
	})
	return s.dockerRuntime, s.dockerRuntimeErr
}

func sanitizeContainerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ToLower(name)
	var builder strings.Builder
	lastHyphen := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				builder.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func pluginContainerEnv(stepEnv map[string]string) map[string]string {
	env := cloneStringMap(stepEnv)
	fallbacks := []string{"/workspace"}
	override := func(key string) {
		if len(fallbacks) == 0 {
			return
		}
		env[key] = fallbacks[0]
	}
	override("WORKSPACE")
	override("CI_WORKSPACE")
	override("WORKSPACE_ROOT")
	override("CI_WORKSPACE_ROOT")
	override("REPO_CLONE_PATH")
	return env
}

func (s *Service) processApprovalStep(ctx context.Context, pipelineRecord *model.Pipeline, stepRecord *model.Step, execStep pipelineTaskStep, logFn func(string) error) (approvalResult, error) {
	approvalCfg := execStep.Approval
	approval := stepRecord.Approval
	if approval == nil {
		if approvalCfg == nil {
			return approvalResultRejected, fmt.Errorf("审批步骤缺少配置")
		}
		approval = &model.StepApproval{
			Message:   approvalCfg.Message,
			Approvers: append([]string{}, approvalCfg.Approvers...),
			Strategy:  approvalCfg.Strategy,
			Timeout:   approvalCfg.Timeout,
			State:     model.StepApprovalStatePending,
		}
	} else if approvalCfg != nil {
		if strings.TrimSpace(approval.Message) == "" && strings.TrimSpace(approvalCfg.Message) != "" {
			approval.Message = approvalCfg.Message
		}
		if approval.Timeout == 0 && approvalCfg.Timeout > 0 {
			approval.Timeout = approvalCfg.Timeout
		}
		if len(approval.Approvers) == 0 && len(approvalCfg.Approvers) > 0 {
			approval.Approvers = append([]string{}, approvalCfg.Approvers...)
		}
	}

	if approval.Strategy == "" {
		approval.Strategy = model.StepApprovalStrategyAny
	}

	switch approval.State {
	case model.StepApprovalStateApproved:
		return approvalResultContinue, nil
	case model.StepApprovalStateRejected:
		return approvalResultRejected, nil
	case model.StepApprovalStateExpired:
		return approvalResultExpired, nil
	}

	now := time.Now().Unix()
	if approval.RequestedAt == 0 {
		approval.RequestedAt = now
		approval.RequestedBy = pipelineRecord.Author
		if approval.Timeout > 0 {
			approval.ExpiresAt = approval.RequestedAt + approval.Timeout
		}
	}
	if approvalExpired(approval, now) {
		approval.State = model.StepApprovalStateExpired
		approval.FinalizedAt = now
		err := errors.New("审批已超时")
		if err := s.setStepFinished(ctx, stepRecord.ID, model.StatusFailure, now, err, -1); err != nil {
			return approvalResultExpired, err
		}
		stepRecord.State = model.StatusFailure
		stepRecord.Error = err.Error()
		if updateErr := s.updateStepApprovalData(ctx, stepRecord, approval, nil); updateErr != nil {
			return approvalResultExpired, updateErr
		}
		return approvalResultExpired, nil
	}

	if stepRecord.Started == 0 {
		stepRecord.Started = now
	}
	stepRecord.State = model.StatusBlocked
	if err := s.updateStepApprovalData(ctx, stepRecord, approval, map[string]any{
		"state":   model.StatusBlocked,
		"started": stepRecord.Started,
	}); err != nil {
		return approvalResultWait, err
	}
	if logFn != nil {
		_ = logFn("等待审批: " + firstNonEmpty(approval.Message, stepRecord.Name))
	}
	return approvalResultWait, nil
}

func approvalExpired(approval *model.StepApproval, now int64) bool {
	if approval == nil {
		return false
	}
	if approval.State != model.StepApprovalStatePending {
		return false
	}
	if approval.Timeout <= 0 {
		return false
	}
	if approval.RequestedAt == 0 {
		return false
	}
	return now >= approval.RequestedAt+approval.Timeout
}

func (s *Service) findPipelineTask(ctx context.Context, pipelineID int64) (*model.Task, error) {
	var task model.Task
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("pipeline_id = ?", pipelineID).
			Take(&task).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Service) resumePipelineAfterApproval(ctx context.Context, pipelineID int64) error {
	task, err := s.findPipelineTask(ctx, pipelineID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("未找到流水线任务，无法继续执行")
	}
	return s.queue.Enqueue(ctx, task)
}

func (s *Service) getStepByID(ctx context.Context, stepID int64) (*model.Step, error) {
	var step model.Step
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&step, stepID).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &step, nil
}

func upsertApprovalDecision(decisions []model.StepApprovalDecision, decision model.StepApprovalDecision) []model.StepApprovalDecision {
	updated := make([]model.StepApprovalDecision, 0, len(decisions)+1)
	found := false
	for _, item := range decisions {
		if strings.EqualFold(item.User, decision.User) {
			if found {
				continue
			}
			updated = append(updated, decision)
			found = true
			continue
		}
		updated = append(updated, item)
	}
	if !found {
		updated = append(updated, decision)
	}
	return updated
}

func containsIgnoreCase(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func allApproversApproved(approvers []string, decisions []model.StepApprovalDecision) bool {
	if len(approvers) == 0 {
		return true
	}
	approved := make(map[string]struct{})
	for _, decision := range decisions {
		if strings.ToLower(strings.TrimSpace(decision.Action)) != "approve" {
			continue
		}
		approved[strings.ToLower(strings.TrimSpace(decision.User))] = struct{}{}
	}
	for _, approver := range approvers {
		if _, ok := approved[strings.ToLower(strings.TrimSpace(approver))]; !ok {
			return false
		}
	}
	return true
}

func (s *Service) updateStepApprovalData(ctx context.Context, step *model.Step, approval *model.StepApproval, extra map[string]any) error {
	updates := map[string]any{
		"approval": approval,
	}
	for key, value := range extra {
		updates[key] = value
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("id = ?", step.ID).
			Updates(updates).Error
	}); err != nil {
		return err
	}
	step.Approval = approval
	if state, ok := updates["state"].(model.StatusValue); ok {
		step.State = state
	}
	if started, ok := updates["started"].(int64); ok {
		step.Started = started
	}
	if finished, ok := updates["finished"].(int64); ok {
		step.Finished = finished
	}
	if errMsg, ok := updates["error"].(string); ok {
		step.Error = errMsg
	}
	return nil
}

func (s *Service) markPipelineBlocked(ctx context.Context, pipelineID int64, message string) error {
	now := time.Now().Unix()
	updates := map[string]any{
		"status":  model.StatusBlocked,
		"updated": now,
	}
	if strings.TrimSpace(message) != "" {
		updates["message"] = message
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("id = ?", pipelineID).
			Updates(updates).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).
			Model(&model.Workflow{}).
			Where("pipeline_id = ?", pipelineID).
			Updates(map[string]any{
				"state": model.StatusBlocked,
			}).Error
	})
}

func defaultPipelineSettings() *model.RepoPipelineConfig {
	return &model.RepoPipelineConfig{
		CleanupEnabled:   false,
		RetentionDays:    7,
		MaxRecords:       10,
		Dockerfile:       "",
		DisallowParallel: false,
		CronSchedules:    []string{},
	}
}

func normalizePipelineConfig(cfg *model.RepoPipelineConfig) *model.RepoPipelineConfig {
	if cfg == nil {
		return nil
	}
	if cfg.CronSchedules == nil {
		cfg.CronSchedules = []string{}
	}
	if len(cfg.CronSchedules) == 0 && cfg.LegacyCronEnabled {
		if legacy := strings.TrimSpace(cfg.LegacyCronSpec); legacy != "" {
			cfg.CronSchedules = []string{legacy}
		}
	}
	return cfg
}

func (s *Service) reloadCronSchedules(ctx context.Context) error {
	type cronRecord struct {
		RepoID            int64    `gorm:"column:repo_id"`
		CronSchedules     []string `gorm:"column:cron_schedules;serializer:json"`
		LegacyCronEnabled bool     `gorm:"column:cron_enabled"`
		LegacyCronSpec    string   `gorm:"column:cron_spec"`
	}

	var records []cronRecord
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.RepoPipelineConfig{}).
			Select("repo_id", "cron_schedules", "cron_enabled", "cron_spec").
			Find(&records).Error
	}); err != nil {
		return err
	}

	seen := make(map[int64]struct{}, len(records))
	for _, record := range records {
		schedules := sanitizeCronSchedules(record.CronSchedules)
		if len(schedules) == 0 && record.LegacyCronEnabled {
			if legacy := strings.TrimSpace(record.LegacyCronSpec); legacy != "" {
				schedules = []string{legacy}
			}
		}
		s.refreshCronEntries(record.RepoID, schedules)
		seen[record.RepoID] = struct{}{}
	}

	s.cronMu.Lock()
	existing := make([]int64, 0, len(s.cronEntries))
	for repoID := range s.cronEntries {
		existing = append(existing, repoID)
	}
	s.cronMu.Unlock()

	for _, repoID := range existing {
		if _, ok := seen[repoID]; !ok {
			s.refreshCronEntries(repoID, nil)
		}
	}

	return nil
}

func (s *Service) refreshCronEntries(repoID int64, schedules []string) {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	if s.scheduler == nil {
		return
	}

	if ids, ok := s.cronEntries[repoID]; ok {
		for _, id := range ids {
			s.scheduler.Remove(id)
		}
		delete(s.cronEntries, repoID)
	}

	sanitized := sanitizeCronSchedules(schedules)
	if len(sanitized) == 0 {
		return
	}

	for _, spec := range sanitized {
		specCopy := spec
		entryID, err := s.scheduler.Add(specCopy, func() {
			s.runScheduledPipeline(repoID, specCopy)
		})
		if err != nil {
			log.Warn().Err(err).Int64("repo_id", repoID).Str("cron_expression", specCopy).Msg("skipping invalid cron expression")
			continue
		}
		s.cronEntries[repoID] = append(s.cronEntries[repoID], entryID)
		log.Debug().Int64("repo_id", repoID).Str("cron_expression", specCopy).Msg("registered cron pipeline schedule")
	}
}

func (s *Service) runScheduledPipeline(repoID int64, expression string) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Int64("repo_id", repoID).Str("cron_expression", expression).Msg("cron pipeline panicked")
		}
	}()

	ctx := context.Background()
	repo, err := s.fetchRepo(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Int64("repo_id", repoID).Str("cron_expression", expression).Msg("failed to load repository for cron pipeline")
		return
	}
	if repo == nil {
		log.Warn().Int64("repo_id", repoID).Str("cron_expression", expression).Msg("repository not found for cron pipeline")
		return
	}

	cfg, err := s.EnsurePipelineConfig(ctx, repo)
	if err != nil {
		log.Error().Err(err).Int64("repo_id", repoID).Str("cron_expression", expression).Msg("failed to load pipeline configuration for cron pipeline")
		return
	}

	author := firstNonEmpty(repo.Owner, "cron")
	branch := strings.TrimSpace(repo.Branch)

	opts := model.PipelineOptions{
		Branch: branch,
		Variables: map[string]string{
			"CRON_EXPRESSION":   expression,
			"CRON_TRIGGERED_AT": time.Now().UTC().Format(time.RFC3339),
			"CRON_TRIGGERED_BY": author,
		},
	}
	if branch != "" {
		opts.Variables["CRON_DEFAULT_BRANCH"] = branch
	}

	message := fmt.Sprintf("定时触发（%s）", expression)
	title := fmt.Sprintf("定时任务 - %s", expression)

	log.Info().
		Int64("repo_id", repoID).
		Str("cron_expression", expression).
		Msg("triggering scheduled pipeline")

	if _, err := s.triggerPipelineWithEvent(ctx, repo, cfg, opts, model.EventCron, author, message, title); err != nil {
		log.Error().Err(err).Int64("repo_id", repoID).Str("cron_expression", expression).Msg("failed to trigger cron pipeline")
	}
}

func sanitizeCronSchedules(schedules []string) []string {
	if len(schedules) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(schedules))
	result := make([]string, 0, len(schedules))
	for _, value := range schedules {
		trimmed := strings.TrimSpace(value)
		trimmed = strings.Trim(trimmed, "\"'")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func defaultPipelineMessage(event model.WebhookEvent, author string) string {
	name := strings.TrimSpace(author)
	switch event {
	case model.EventManual:
		if name != "" {
			return fmt.Sprintf("手动触发（%s）", name)
		}
		return "手动触发"
	case model.EventCron:
		return "定时触发"
	case model.EventPush:
		if name != "" {
			return fmt.Sprintf("代码推送触发（%s）", name)
		}
		return "代码推送触发"
	case model.EventDeploy:
		return "部署事件触发"
	case model.EventTag:
		return "创建 Tag 触发"
	case model.EventRelease:
		return "发布事件触发"
	case model.EventPull, model.EventPullMetadata, model.EventPullClosed:
		return "合并请求触发"
	default:
		return "系统触发"
	}
}

func (s *Service) enforcePipelineRetention(ctx context.Context, repo *model.Repo, settings *model.RepoPipelineConfig) error {
	if repo == nil {
		return nil
	}
	if settings == nil {
		settings = defaultPipelineSettings()
	}

	maxRecords := settings.MaxRecords
	if maxRecords <= 0 {
		// 即便不限制最大记录数量，仍然尝试清理过期的工作目录
		s.cleanupExpiredWorkspaces(ctx, repo, settings)
		return nil
	}

	var obsoleteIDs []int64
	const retentionSelectLimit = 10000
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("repo_id = ?", repo.ID).
			Order("created DESC").
			Offset(maxRecords).
			Limit(retentionSelectLimit).
			Pluck("id", &obsoleteIDs).Error
	})
	if err != nil {
		return err
	}
	if len(obsoleteIDs) == 0 {
		// 即便没有过期的 pipeline 记录，也尝试按天数清理工作目录
		s.cleanupExpiredWorkspaces(ctx, repo, settings)
		return nil
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// collect step ids for logs
		var stepIDs []int64
		if err := tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("pipeline_id IN ?", obsoleteIDs).
			Pluck("id", &stepIDs).Error; err != nil {
			return err
		}

		if len(stepIDs) > 0 {
			if err := tx.WithContext(ctx).Delete(&model.LogEntry{}, "step_id IN ?", stepIDs).Error; err != nil {
				return err
			}
		}

		if err := tx.WithContext(ctx).Delete(&model.Step{}, "pipeline_id IN ?", obsoleteIDs).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Delete(&model.Workflow{}, "pipeline_id IN ?", obsoleteIDs).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Delete(&model.Task{}, "pipeline_id IN ?", obsoleteIDs).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).Delete(&model.Pipeline{}, "id IN ?", obsoleteIDs).Error
	}); err != nil {
		return err
	}

	s.cleanupObsoleteWorkspaces(repo, settings, obsoleteIDs)
	s.cleanupExpiredWorkspaces(ctx, repo, settings)
	return nil
}

func workspaceRootCandidates(settings *model.RepoPipelineConfig) []string {
	roots := map[string]struct{}{
		sanitizeWorkspaceRoot(""): {},
	}

	if settings != nil {
		if content := strings.TrimSpace(settings.Content); content != "" {
			if specDef, err := spec.Parse(content); err == nil {
				if root := strings.TrimSpace(specDef.Workspace); root != "" {
					roots[sanitizeWorkspaceRoot(root)] = struct{}{}
				}
			} else {
				log.Debug().Err(err).Msg("failed to parse pipeline specification for workspace cleanup")
			}
		}
	}

	result := make([]string, 0, len(roots))
	for root := range roots {
		result = append(result, root)
	}
	return result
}

func (s *Service) cleanupObsoleteWorkspaces(repo *model.Repo, settings *model.RepoPipelineConfig, pipelineIDs []int64) {
	if repo == nil || len(pipelineIDs) == 0 {
		return
	}
	dirName := sanitizeDirName(repo.Name)
	seen := make(map[int64]struct{}, len(pipelineIDs))
	for _, id := range pipelineIDs {
		seen[id] = struct{}{}
	}

	for _, root := range workspaceRootCandidates(settings) {
		repoDir := filepath.Join(root, dirName)
		for id := range seen {
			path := filepath.Join(repoDir, fmt.Sprintf("%d", id))
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				log.Warn().Err(err).Str("path", path).Msg("failed to remove obsolete workspace")
			}
		}
	}
}

func (s *Service) cleanupExpiredWorkspaces(ctx context.Context, repo *model.Repo, settings *model.RepoPipelineConfig) {
	if repo == nil || settings == nil || settings.RetentionDays <= 0 {
		return
	}

	cutoff := time.Now().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
	active := s.fetchPipelineIDSet(ctx, repo.ID)
	dirName := sanitizeDirName(repo.Name)

	for _, root := range workspaceRootCandidates(settings) {
		repoDir := filepath.Join(root, dirName)
		entries, err := os.ReadDir(repoDir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Debug().Err(err).Str("path", repoDir).Msg("skip workspace cleanup for repo")
			}
			continue
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if !info.IsDir() {
				continue
			}
			path := filepath.Join(repoDir, entry.Name())
			if info.ModTime().After(cutoff) {
				continue
			}

			if id, err := strconv.ParseInt(entry.Name(), 10, 64); err == nil {
				if _, ok := active[id]; ok {
					// pipeline still tracked, skip
					continue
				}
			}

			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				log.Warn().Err(err).Str("path", path).Msg("failed to remove expired workspace")
			}
		}
	}
}

func (s *Service) fetchPipelineIDSet(ctx context.Context, repoID int64) map[int64]struct{} {
	result := make(map[int64]struct{})
	var ids []int64
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("repo_id = ?", repoID).
			Pluck("id", &ids).Error
	}); err != nil {
		log.Warn().Err(err).Int64("repo_id", repoID).Msg("failed to query existing pipeline ids for workspace cleanup")
		return result
	}
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result
}

func resolveWorkspaceCommit(ctx context.Context, dir string) (string, error) {
	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("git directory not found")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (s *Service) updatePipelineCommit(ctx context.Context, pipelineID int64, commit string) error {
	if strings.TrimSpace(commit) == "" {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("id = ?", pipelineID).
			Update("commit", commit).Error
	})
}

func addCredentialsToURL(rawURL, username, password string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.User = url.UserPassword(username, password)
	return parsed.String(), nil
}

func cloneSupportsCredentials(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func (s *Service) buildCertificateEnv(ctx context.Context, repo *model.Repo, settings *model.RepoPipelineConfig, requested map[string]string) (map[string]string, string, map[string]resolvedSecretBinding) {
	env := make(map[string]string)
	bindings := make(map[string]resolvedSecretBinding)
	if s.systemSvc == nil || repo == nil {
		return env, "", bindings
	}

	includeAll := len(requested) == 0

	var cloneOverride string
	usedSanitized := make(map[string]struct{})
	resolvedAliases := make(map[string]struct{})

	if settings != nil {
		for _, binding := range settings.LegacyCertificates {
			if binding.CertificateID == 0 {
				continue
			}
			aliasOriginal := strings.TrimSpace(binding.Alias)
			aliasKey := strings.ToLower(aliasOriginal)
			if aliasOriginal == "" {
				aliasOriginal = fmt.Sprintf("cert_%d", binding.CertificateID)
				aliasKey = strings.ToLower(aliasOriginal)
			}
			if !includeAll {
				if _, ok := requested[aliasKey]; !ok {
					continue
				}
			}

			sanitized := sanitizeAlias(aliasOriginal)
			if sanitized == "" {
				sanitized = fmt.Sprintf("CERT_%d", binding.CertificateID)
			}
			if _, exists := usedSanitized[sanitized]; exists {
				sanitized = fmt.Sprintf("%s_%d", sanitized, binding.CertificateID)
			}
			usedSanitized[sanitized] = struct{}{}

			cert, err := s.systemSvc.GetCertificateWithSecrets(ctx, binding.CertificateID)
			if err != nil {
				log.Warn().
					Err(err).
					Int64("certificate_id", binding.CertificateID).
					Msg("failed to load certificate for pipeline")
				continue
			}

			resolved := resolvedSecretBinding{
				Alias:          aliasOriginal,
				SanitizedAlias: sanitized,
				Type:           cert.Type,
				Values:         map[string]string{},
			}

			switch strings.ToLower(cert.Type) {
			case "git":
				gitCert, err := cert.AsGitCertificate()
				if err != nil {
					log.Warn().
						Err(err).
						Int64("certificate_id", binding.CertificateID).
						Msg("invalid git certificate")
					continue
				}
				env[fmt.Sprintf("%s_USERNAME", sanitized)] = gitCert.Username
				env[fmt.Sprintf("%s_PASSWORD", sanitized)] = gitCert.Password
				env[fmt.Sprintf("%s_TOKEN", sanitized)] = gitCert.Password

				resolved.Values["git.username"] = gitCert.Username
				resolved.Values["git.password"] = gitCert.Password
				resolved.Values["git.token"] = gitCert.Password

				if cloneOverride == "" && cloneSupportsCredentials(repo.Clone) {
					if cloneURL, err := addCredentialsToURL(repo.Clone, gitCert.Username, gitCert.Password); err == nil {
						cloneOverride = cloneURL
					} else {
						log.Warn().
							Err(err).
							Int64("certificate_id", binding.CertificateID).
							Msg("failed to apply credentials to clone url")
					}
				}
			case "docker":
				dockerCert, err := cert.AsDockerCertificate()
				if err != nil {
					log.Warn().
						Err(err).
						Int64("certificate_id", binding.CertificateID).
						Msg("invalid docker certificate")
					continue
				}
				env[fmt.Sprintf("%s_USERNAME", sanitized)] = dockerCert.Username
				env[fmt.Sprintf("%s_PASSWORD", sanitized)] = dockerCert.Password
				env[fmt.Sprintf("%s_REPO", sanitized)] = dockerCert.Repo

				resolved.Values["docker.username"] = dockerCert.Username
				resolved.Values["docker.password"] = dockerCert.Password
				resolved.Values["docker.repo"] = dockerCert.Repo
				resolved.Values["docker.registry"] = dockerCert.Repo
			default:
				log.Debug().
					Int64("certificate_id", binding.CertificateID).
					Str("type", cert.Type).
					Msg("certificate type not supported for pipeline environment")
				continue
			}

			bindings[aliasKey] = resolved
			resolvedAliases[aliasKey] = struct{}{}
		}
	}

	if !includeAll {
		for aliasKey, original := range requested {
			if _, ok := resolvedAliases[aliasKey]; ok {
				continue
			}
			if strings.TrimSpace(original) == "" {
				continue
			}
			cert, err := s.systemSvc.GetCertificateWithSecretsByName(ctx, original)
			if err != nil {
				log.Warn().
					Err(err).
					Str("alias", original).
					Msg("failed to resolve global certificate for pipeline")
				continue
			}
			if cert == nil {
				continue
			}
			sanitized := sanitizeAlias(original)
			if sanitized == "" {
				sanitized = fmt.Sprintf("CERT_%d", cert.ID)
			}
			if _, exists := usedSanitized[sanitized]; exists {
				sanitized = fmt.Sprintf("%s_%d", sanitized, cert.ID)
			}
			usedSanitized[sanitized] = struct{}{}

			resolved := resolvedSecretBinding{
				Alias:          original,
				SanitizedAlias: sanitized,
				Type:           cert.Type,
				Values:         map[string]string{},
			}

			switch strings.ToLower(cert.Type) {
			case "git":
				gitCert, err := cert.AsGitCertificate()
				if err != nil {
					log.Warn().
						Err(err).
						Int64("certificate_id", cert.ID).
						Str("alias", original).
						Msg("invalid global git certificate")
					continue
				}
				env[fmt.Sprintf("%s_USERNAME", sanitized)] = gitCert.Username
				env[fmt.Sprintf("%s_PASSWORD", sanitized)] = gitCert.Password
				env[fmt.Sprintf("%s_TOKEN", sanitized)] = gitCert.Password

				resolved.Values["git.username"] = gitCert.Username
				resolved.Values["git.password"] = gitCert.Password
				resolved.Values["git.token"] = gitCert.Password

				if cloneOverride == "" && cloneSupportsCredentials(repo.Clone) {
					if cloneURL, err := addCredentialsToURL(repo.Clone, gitCert.Username, gitCert.Password); err == nil {
						cloneOverride = cloneURL
					} else {
						log.Warn().
							Err(err).
							Int64("certificate_id", cert.ID).
							Str("alias", original).
							Msg("failed to apply credentials to clone url")
					}
				}
			case "docker":
				dockerCert, err := cert.AsDockerCertificate()
				if err != nil {
					log.Warn().
						Err(err).
						Int64("certificate_id", cert.ID).
						Str("alias", original).
						Msg("invalid global docker certificate")
					continue
				}
				env[fmt.Sprintf("%s_USERNAME", sanitized)] = dockerCert.Username
				env[fmt.Sprintf("%s_PASSWORD", sanitized)] = dockerCert.Password
				env[fmt.Sprintf("%s_REPO", sanitized)] = dockerCert.Repo

				resolved.Values["docker.username"] = dockerCert.Username
				resolved.Values["docker.password"] = dockerCert.Password
				resolved.Values["docker.repo"] = dockerCert.Repo
				resolved.Values["docker.registry"] = dockerCert.Repo
			default:
				log.Debug().
					Int64("certificate_id", cert.ID).
					Str("alias", original).
					Str("type", cert.Type).
					Msg("global certificate type not supported for pipeline environment")
				continue
			}

			bindings[aliasKey] = resolved
			resolvedAliases[aliasKey] = struct{}{}
		}
	}

	return env, cloneOverride, bindings
}

// CancelPipelineRun stops an in-flight pipeline and marks it as killed.
func (s *Service) CancelPipelineRun(ctx context.Context, repoID, pipelineID int64, reason string) error {
	var pipeline model.Pipeline
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("id = ? AND repo_id = ?", pipelineID, repoID).
			Take(&pipeline).Error
	})
	if err != nil {
		return err
	}

	switch pipeline.Status {
	case model.StatusSuccess, model.StatusFailure, model.StatusKilled, model.StatusError:
		return fmt.Errorf("pipeline 已结束，无法取消")
	}

	if handleAny, ok := s.executions.Load(pipelineID); ok && handleAny != nil {
		if handle, ok := handleAny.(*executionHandle); ok && handle.cancel != nil {
			handle.cancel()
		}
	}

	now := time.Now().Unix()
	cancelMessage := reason
	if strings.TrimSpace(cancelMessage) == "" {
		cancelMessage = "Pipeline cancelled by user"
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("id = ?", pipelineID).
			Updates(map[string]any{
				"status":   model.StatusKilled,
				"message":  cancelMessage,
				"finished": now,
				"updated":  now,
			}).Error; err != nil {
			return err
		}

		if err := tx.WithContext(ctx).
			Model(&model.Workflow{}).
			Where("pipeline_id = ? AND state IN ?", pipelineID, []model.StatusValue{model.StatusPending, model.StatusRunning}).
			Updates(map[string]any{
				"state":    model.StatusKilled,
				"finished": now,
			}).Error; err != nil {
			return err
		}

		if err := tx.WithContext(ctx).
			Model(&model.Step{}).
			Where("pipeline_id = ? AND state IN ?", pipelineID, []model.StatusValue{model.StatusPending, model.StatusRunning}).
			Updates(map[string]any{
				"state":    model.StatusKilled,
				"finished": now,
				"failure":  "",
				"error":    "",
			}).Error; err != nil {
			return err
		}

		return tx.WithContext(ctx).Delete(&model.Task{}, "pipeline_id = ?", pipelineID).Error
	}); err != nil {
		return err
	}

	s.executions.Delete(pipelineID)
	return nil
}

func generateRandomID(prefix string) string {
	const defaultLen = 18
	b := make([]byte, defaultLen)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	if prefix == "" {
		return id
	}
	return fmt.Sprintf("%s-%s", prefix, id)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func statusFromPipeline(status model.StatusValue) model.StatusValue {
	switch status {
	case model.StatusKilled:
		return model.StatusKilled
	case model.StatusFailure:
		return model.StatusFailure
	default:
		return model.StatusSuccess
	}
}

func (s *Service) getPipelineStatus(ctx context.Context, pipelineID int64) (model.StatusValue, error) {
	var pipeline model.Pipeline
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Select("status").
			Where("id = ?", pipelineID).
			Take(&pipeline).Error
	})
	if err != nil {
		return "", err
	}
	return pipeline.Status, nil
}

func (s *Service) removeTaskRecord(ctx context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Delete(&model.Task{}, "id = ?", taskID).Error
	})
}
