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
	"path"
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
	templatesvc "github.com/thepenn/devsys/service/pipeline/template"
	systemsvc "github.com/thepenn/devsys/service/system"
)

const pipelineCacheKey = "pipeline:%d"

var envPlaceholderRegex = regexp.MustCompile(`\$\{(?:env\.)?([A-Za-z0-9_]+)\}`)

// dottedSecretPlaceholderRegex 抓取 ${name.field...} 这类多段占位符 (至少含一个 `.`).
// 单段 ${VAR} 由 envPlaceholderRegex 处理, 这里只关心点路径形式以便自动发现凭证引用.
var dottedSecretPlaceholderRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_-]*)(?:\.[A-Za-z_][A-Za-z0-9_-]*)+\}`)

// unresolvedPlaceholderRegex 用于运行时检测 plugin 设置里残留的 ${...} 字面量.
// 排除 $(...) 命令替换语法和 $$ 转义.
var unresolvedPlaceholderRegex = regexp.MustCompile(`\$\{[^}\s]+\}`)

// JobScheduler 由独立 Job 服务实现并通过 SetJobScheduler 注入. cron 回调
// 在不直接 import job 包的前提下回调 Job.TriggerCron, 避免 pipeline ↔ job
// 循环依赖.
type JobScheduler interface {
	TriggerCron(ctx context.Context, jobID int64, expression string) error
}

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
	templateSvc       *templatesvc.Service
	jobScheduler      JobScheduler
	scheduler         *cron.Cron
	cronEntries       map[int64][]cron.ID
	jobCronEntries    map[int64][]cron.ID
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
	Build      *pipelineBuildConfig    `json:"build,omitempty"`
	Conditions *pipelineStepConditions `json:"conditions,omitempty"`
}

type pipelinePluginConfig struct {
	Settings   map[string][]string `json:"settings,omitempty"`
	Volumes    []string            `json:"volumes,omitempty"`
	Privileged bool                `json:"privileged,omitempty"`
}

// pipelineBuildConfig 是 spec.BuildSpec 在 task payload 里的镜像; 字段一一对应,
// 引擎跑 step 时根据它生成 buildctl-daemonless.sh 命令.
type pipelineBuildConfig struct {
	Registry      string            `json:"registry,omitempty"`
	Repo          string            `json:"repo"`
	Username      string            `json:"username,omitempty"`
	Password      string            `json:"password,omitempty"`
	Dockerfile    string            `json:"dockerfile,omitempty"`
	Context       string            `json:"context,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Platforms     []string          `json:"platforms,omitempty"`
	Push          *bool             `json:"push,omitempty"`
	BuildArgs     map[string]string `json:"build_args,omitempty"`
	Target        string            `json:"target,omitempty"`
	NoCache       bool              `json:"no_cache,omitempty"`
	BuildkitImage string            `json:"buildkit_image,omitempty"`
	Privileged    bool              `json:"privileged,omitempty"`
}

// defaultBuildkitImage 是 kind=build 步骤使用的 buildkit 镜像默认值. 用
// privileged 模式 + :latest 兼容所有 Docker daemon (rootless 需要 Linux +
// Docker 20.10+ + systempaths/apparmor unconfined, Colima/Docker Desktop/旧版
// Docker 都可能拒收 systempaths=unconfined 而起不来). 用户可在 build.buildkit_image
// 显式切到 moby/buildkit:rootless 走 rootless 路径.
const defaultBuildkitImage = "moby/buildkit:latest"

// defaultBuildkitImageRootless 是显式 opt-in 走 rootless 路径时的常用镜像名,
// 仅供文档/前端模板引用; runtime 是按 image 名是否含 "rootless" 自动决定.
const defaultBuildkitImageRootless = "moby/buildkit:rootless"

type pipelineApprovalConfig struct {
	Message   string                     `json:"message"`
	Approvers []string                   `json:"approvers"`
	Timeout   int64                      `json:"timeout"`
	Strategy  model.StepApprovalStrategy `json:"strategy"`
}

// pipelineStepConditions 是 spec.StepConditions 的运行时镜像 (序列化进
// task.Data 的 JSON). 字段比 spec 简化, 只放真正会被 enforce 的几个 +
// Groups (用于 OR-of-AND 精确判定).
//
// 兼容老 task payload: 旧格式只有 Branches; allows() 在没有 Groups 时
// 退化到扁平字段 + branch-only 行为.
type pipelineStepConditions struct {
	Branches      []string `json:"branches,omitempty"`
	BranchInclude []string `json:"branch_include,omitempty"`
	BranchExclude []string `json:"branch_exclude,omitempty"`
	Events        []string `json:"events,omitempty"`
	Statuses      []string `json:"statuses,omitempty"`
	Refs          []string `json:"refs,omitempty"`
	Repos         []string `json:"repos,omitempty"`

	Groups []conditionGroup `json:"groups,omitempty"`
}

// conditionGroup 与 spec.StepConditionGroup 一一对应的运行时形态.
type conditionGroup struct {
	Branches      []string `json:"branches,omitempty"`
	BranchInclude []string `json:"branch_include,omitempty"`
	BranchExclude []string `json:"branch_exclude,omitempty"`
	Events        []string `json:"events,omitempty"`
	Statuses      []string `json:"statuses,omitempty"`
	Refs          []string `json:"refs,omitempty"`
	Repos         []string `json:"repos,omitempty"`
}

// triggerContext 描述当前触发的运行时环境, 用于 step.when 的 enforce.
// status 字段在 step dispatch 时按当前 pipeline 已有结果填 (success / failure).
type triggerContext struct {
	Branch string
	Event  string
	Ref    string
	Repo   string
	Status string
}

// allows 评估 step 是否在当前 trigger 下可执行. 决策:
//   - Groups 非空: ANY group AND-命中 -> 通过.
//   - Groups 空: 退化到扁平字段, 其中 branch / event / ref / repo / status
//     都做 AND 校验 (空字段视为通过).
func (c *pipelineStepConditions) allows(trigger triggerContext) bool {
	if c == nil {
		return true
	}
	if len(c.Groups) > 0 {
		for _, g := range c.Groups {
			if matchConditionGroup(g, trigger) {
				return true
			}
		}
		return false
	}
	// 退化路径
	flat := conditionGroup{
		Branches:      c.Branches,
		BranchInclude: c.BranchInclude,
		BranchExclude: c.BranchExclude,
		Events:        c.Events,
		Statuses:      c.Statuses,
		Refs:          c.Refs,
		Repos:         c.Repos,
	}
	return matchConditionGroup(flat, trigger)
}

func matchConditionGroup(g conditionGroup, t triggerContext) bool {
	if !matchBranch(g, t.Branch) {
		return false
	}
	if !matchAnyGlob(g.Events, t.Event, true) {
		return false
	}
	if !matchAnyGlob(g.Refs, t.Ref, false) {
		return false
	}
	if !matchAnyGlob(g.Repos, t.Repo, false) {
		return false
	}
	if !matchStatus(g.Statuses, t.Status) {
		return false
	}
	return true
}

func matchBranch(g conditionGroup, branch string) bool {
	if branch == "" && len(g.Branches) == 0 && len(g.BranchInclude) == 0 && len(g.BranchExclude) == 0 {
		return true
	}
	hasFilter := len(g.Branches) > 0 || len(g.BranchInclude) > 0 || len(g.BranchExclude) > 0
	if !hasFilter {
		return true
	}
	// exclude 优先: 命中即拒绝
	for _, pattern := range g.BranchExclude {
		if matched, _ := matchGlob(pattern, branch); matched {
			return false
		}
	}
	candidates := append([]string{}, g.Branches...)
	candidates = append(candidates, g.BranchInclude...)
	if len(candidates) == 0 {
		return true // 只有 exclude 没 include -> 未被排除即通过
	}
	for _, pattern := range candidates {
		if matched, _ := matchGlob(pattern, branch); matched {
			return true
		}
	}
	return false
}

// matchAnyGlob 在 patterns 至少一项匹配 actual 时返回 true; patterns 为空
// 视为不约束 (返回 true). exact=true 时跳过 glob 走精确比较.
func matchAnyGlob(patterns []string, actual string, exact bool) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if exact {
			if strings.TrimSpace(p) == strings.TrimSpace(actual) {
				return true
			}
		} else if matched, _ := matchGlob(p, actual); matched {
			return true
		}
	}
	return false
}

// matchStatus 与 step 当前期望状态比较; 仅当 actual 已知时 enforce.
// 默认 (Statuses 空) 仅在 success 流中执行.
func matchStatus(statuses []string, actual string) bool {
	if len(statuses) == 0 {
		// 默认行为: pipeline 已失败时跳过该 step (与 Woodpecker 默认一致),
		// 但当前 actual 大多数时候就是 "success" / 空, 这里宽松视为通过,
		// 让 handleTask 的整体 break-on-failure 逻辑接管.
		return true
	}
	if strings.TrimSpace(actual) == "" {
		// 状态尚不可知 (执行前), 命中任意 status 都允许 — 真正的过滤在
		// handleTask 推进时按结果再次评估.
		return true
	}
	for _, s := range statuses {
		if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(actual)) {
			return true
		}
	}
	return false
}

// matchGlob 用 stdlib path.Match 做 * 通配匹配. 不支持 ** doublestar.
// 空 pattern 永远不匹配 (避免误通过).
func matchGlob(pattern, actual string) (bool, error) {
	pattern = strings.TrimSpace(pattern)
	actual = strings.TrimSpace(actual)
	if pattern == "" {
		return false, nil
	}
	// 优先精确字符串比较, 命中即避免 path.Match 出错
	if pattern == actual {
		return true, nil
	}
	return path.Match(pattern, actual)
}

func (c *pipelineStepConditions) branchSummary() string {
	if c == nil {
		return ""
	}
	all := append([]string{}, c.Branches...)
	all = append(all, c.BranchInclude...)
	if len(all) == 0 {
		return ""
	}
	return strings.Join(all, ", ")
}

// allowsBranch 兼容旧调用点; 内部转 allows(triggerContext{Branch: branch}).
func (c *pipelineStepConditions) allowsBranch(branch string) bool {
	return c.allows(triggerContext{Branch: branch})
}

func (step pipelineTaskStep) allowsBranch(branch string) bool {
	if step.Conditions == nil {
		return true
	}
	return step.Conditions.allowsBranch(branch)
}

// allowsTrigger 是 allowsBranch 的多维版本; 在 handleTask 里推荐使用此函数.
func (step pipelineTaskStep) allowsTrigger(trigger triggerContext) bool {
	if step.Conditions == nil {
		return true
	}
	return step.Conditions.allows(trigger)
}

// buildPipelineStepConditions 把 spec.StepConditions (含 Groups) 转为
// 运行时 pipelineStepConditions, 序列化进 task payload. 没有任何条件时
// 返回 nil 让 step.allowsXxx 直接通过.
func buildPipelineStepConditions(in *spec.StepConditions) *pipelineStepConditions {
	if in == nil {
		return nil
	}
	hasFlat := len(in.Branches) > 0 || len(in.BranchInclude) > 0 || len(in.BranchExclude) > 0 ||
		len(in.Events) > 0 || len(in.Statuses) > 0 || len(in.Refs) > 0 || len(in.Repos) > 0
	if !hasFlat && len(in.Groups) == 0 {
		return nil
	}
	out := &pipelineStepConditions{
		Branches:      append([]string{}, in.Branches...),
		BranchInclude: append([]string{}, in.BranchInclude...),
		BranchExclude: append([]string{}, in.BranchExclude...),
		Events:        append([]string{}, in.Events...),
		Statuses:      append([]string{}, in.Statuses...),
		Refs:          append([]string{}, in.Refs...),
		Repos:         append([]string{}, in.Repos...),
	}
	for _, g := range in.Groups {
		out.Groups = append(out.Groups, conditionGroup{
			Branches:      append([]string{}, g.Branches...),
			BranchInclude: append([]string{}, g.BranchInclude...),
			BranchExclude: append([]string{}, g.BranchExclude...),
			Events:        append([]string{}, g.Events...),
			Statuses:      append([]string{}, g.Statuses...),
			Refs:          append([]string{}, g.Refs...),
			Repos:         append([]string{}, g.Repos...),
		})
	}
	return out
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

// WithTemplateService 注入通用 pipeline 模板服务. 当 RepoPipelineConfig.Source
// 为 template 时, triggerPipelineWithEvent 会通过它把模板的 PublishedContent
// 渲染成最终待 spec.Parse 的 YAML.
func WithTemplateService(template *templatesvc.Service) Option {
	return func(s *Service) {
		s.templateSvc = template
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
		jobCronEntries: make(map[int64][]cron.ID),
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

		// 启动时把当前生效的 workspace root 打出来. 默认走 ${HOME}/.devsys-workspace,
		// 是否被 PIPELINE_WORKSPACE_ROOT env / spec.workspace / payload.WorkspaceRoot
		// 覆盖一目了然, 出 bind mount 不通这种问题时第一时间能定位.
		log.Info().
			Str("workspace_root", defaultWorkspaceRoot()).
			Str("env_override", os.Getenv("PIPELINE_WORKSPACE_ROOT")).
			Msg("pipeline workspace root resolved")

		scheduler := cron.New()
		s.cronMu.Lock()
		s.scheduler = scheduler
		s.cronEntries = make(map[int64][]cron.ID)
		s.jobCronEntries = make(map[int64][]cron.ID)
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
	s.jobCronEntries = make(map[int64][]cron.ID)
	s.cronMu.Unlock()

	if scheduler != nil {
		stopCtx := scheduler.Stop()
		<-stopCtx.Done()
	}

	if s.queue != nil {
		s.queue.Shutdown()
	}
}

// CreatePipeline persists the pipeline and related entities. Number 自动按
// owner (repo / job) 范围分配, 与 (repo_id, job_id, number) 唯一索引一致.
func (s *Service) CreatePipeline(ctx context.Context, pipeline *model.Pipeline, workflows []*model.Workflow, steps []*model.Step, tasks []*model.Task) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is required")
	}
	if pipeline.OwnerKind == "" {
		pipeline.OwnerKind = model.PipelineOwnerRepo
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if pipeline.Number == 0 {
			switch pipeline.OwnerKind {
			case model.PipelineOwnerJob:
				if err := tx.WithContext(ctx).
					Table("pipeline_jobs").
					Select("id").
					Where("id = ?", pipeline.JobID).
					Clauses(clause.Locking{Strength: "UPDATE"}).
					Take(&struct{ ID int64 }{}).Error; err != nil {
					return err
				}
				var nextNumber int64
				if err := tx.WithContext(ctx).
					Model(&model.Pipeline{}).
					Where("job_id = ? AND owner_kind = ?", pipeline.JobID, model.PipelineOwnerJob).
					Select("COALESCE(MAX(number), 0)").
					Scan(&nextNumber).Error; err != nil {
					return err
				}
				pipeline.Number = nextNumber + 1
			default:
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
					Where("repo_id = ? AND (owner_kind = ? OR owner_kind = '' OR owner_kind IS NULL)", pipeline.RepoID, model.PipelineOwnerRepo).
					Select("COALESCE(MAX(number), 0)").
					Scan(&nextNumber).Error; err != nil {
					return err
				}
				pipeline.Number = nextNumber + 1
			}
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

// PipelineConfigInput 描述一次 UpsertPipelineConfigSource 的入参.
//   - Source 默认 inline:
//       - inline:   直接存 cfg.Content.
//       - template: 必须提供 TemplateID; Content 仍可作为切换快照保留.
//       - compose:  必须提供非空 ComposeSteps; TemplateID / Variables 不使用.
type PipelineConfigInput struct {
	Source       string
	Content      string
	TemplateID   *int64
	Variables    map[string]string
	ComposeSteps []model.ComposeStepRef
}

// UpsertPipelineConfig 兼容旧调用 (inline + content) 的快捷方法; 内部转发到
// UpsertPipelineConfigSource. 保留入口避免大面积修改 router / 自动 ensure 路径.
func (s *Service) UpsertPipelineConfig(ctx context.Context, repoID int64, content string) (*model.RepoPipelineConfig, error) {
	return s.UpsertPipelineConfigSource(ctx, repoID, PipelineConfigInput{
		Source:  model.PipelineConfigSourceInline,
		Content: content,
	})
}

// UpsertPipelineConfigSource 是更通用的版本, 支持 inline / template / compose
// 三种来源. 切换 source 时旧字段处理:
//   - 切到 template: 保留 Content 作为快照, 清空 ComposeSteps; 写入新 TemplateID/Variables.
//   - 切到 compose:  保留 Content 作为快照, 清空 TemplateID/Variables; 写入新 ComposeSteps.
//   - 切到 inline:   写入新 Content, 清空 TemplateID/Variables/ComposeSteps.
func (s *Service) UpsertPipelineConfigSource(ctx context.Context, repoID int64, in PipelineConfigInput) (*model.RepoPipelineConfig, error) {
	source := strings.TrimSpace(in.Source)
	switch source {
	case model.PipelineConfigSourceTemplate, model.PipelineConfigSourceCompose:
		// ok
	default:
		source = model.PipelineConfigSourceInline
	}
	switch source {
	case model.PipelineConfigSourceTemplate:
		if in.TemplateID == nil || *in.TemplateID <= 0 {
			return nil, fmt.Errorf("template id is required when source is template")
		}
		if s.templateSvc != nil {
			if _, err := s.templateSvc.Get(ctx, *in.TemplateID); err != nil {
				return nil, err
			}
		}
	case model.PipelineConfigSourceCompose:
		if len(in.ComposeSteps) == 0 {
			return nil, fmt.Errorf("compose_steps is required when source is compose")
		}
		// 校验每个 ref 都指向已存在的 step 模板; 容错不强校验已发布 (允许
		// 用户先存配置, 模板后再发布).
		if s.templateSvc != nil {
			for idx, ref := range in.ComposeSteps {
				if ref.StepTemplateID <= 0 {
					return nil, fmt.Errorf("compose ref #%d: step_template_id missing", idx+1)
				}
				tpl, err := s.templateSvc.Get(ctx, ref.StepTemplateID)
				if err != nil {
					return nil, fmt.Errorf("compose ref #%d (id=%d): %w", idx+1, ref.StepTemplateID, err)
				}
				if tpl.EffectiveKind() != model.PipelineTemplateKindStep {
					return nil, fmt.Errorf("compose ref #%d (%s): template kind must be step", idx+1, tpl.Name)
				}
			}
		}
	}

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
			cfg.Source = source
			cfg.Content = in.Content
			switch source {
			case model.PipelineConfigSourceTemplate:
				cfg.TemplateID = in.TemplateID
				cfg.TemplateVariables = cloneStringMapSafe(in.Variables)
			case model.PipelineConfigSourceCompose:
				cfg.ComposeSteps = append([]model.ComposeStepRef{}, in.ComposeSteps...)
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
			existing.Source = source
			switch source {
			case model.PipelineConfigSourceTemplate:
				existing.TemplateID = in.TemplateID
				existing.TemplateVariables = cloneStringMapSafe(in.Variables)
				existing.ComposeSteps = nil
				// Content 保留为切换前的快照, 不覆盖
			case model.PipelineConfigSourceCompose:
				existing.TemplateID = nil
				existing.TemplateVariables = nil
				existing.ComposeSteps = append([]model.ComposeStepRef{}, in.ComposeSteps...)
				// Content 保留为切换前的快照, 不覆盖
			default: // inline
				existing.Content = in.Content
				existing.TemplateID = nil
				existing.TemplateVariables = nil
				existing.ComposeSteps = nil
			}
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

func cloneStringMapSafe(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// resolvePipelineForTrigger 把 RepoPipelineConfig 转成最终待 BuildAndEnqueueRun
// 消费的形态. 返回 (yamlContent, specOverride, err); 三种来源各自对应一种返回:
//
//   - inline:   返回 (cfg.Content, nil, nil) — 由 BuildAndEnqueueRun 内部 spec.Parse.
//   - template: 合并 repo_ctx + cfg.TemplateVariables 后调 templateSvc.Resolve,
//               返回 (renderedYAML, nil, nil); cfg.TemplateVariables 优先级最高.
//   - compose:  合并 repo_ctx 后调 templateSvc.ResolveCompose, 已经预解析为
//               *spec.PipelineSpec, 返回 ("", spec, nil) 让 BuildAndEnqueueRun
//               跳过 spec.Parse 直接使用.
//
// repo / branch / commit / author 用于构造渲染上下文 (CI_REPO_FULL_NAME /
// REPO_CLONE_URL_AUTH / CI_PIPELINE_BRANCH 等), 让模板里 ${VAR} 能解析到
// 当前项目的真实值, 而不是空串.
func (s *Service) resolvePipelineForTrigger(
	ctx context.Context,
	repo *model.Repo,
	cfg *model.RepoPipelineConfig,
	branch, commit, author string,
) (string, *spec.PipelineSpec, error) {
	if cfg == nil {
		return "", nil, fmt.Errorf("pipeline configuration missing")
	}
	switch cfg.EffectiveSource() {
	case model.PipelineConfigSourceTemplate:
		if s.templateSvc == nil {
			return "", nil, fmt.Errorf("pipeline template service not configured")
		}
		if cfg.TemplateID == nil || *cfg.TemplateID <= 0 {
			return "", nil, fmt.Errorf("pipeline template id missing")
		}
		// repo_ctx 是基线, cfg.TemplateVariables 覆盖之 (项目自定义优先).
		mergedVars := s.BuildRepoRenderContext(ctx, repo, cfg, branch, commit, author)
		for k, v := range cfg.TemplateVariables {
			mergedVars[k] = v
		}
		yamlContent, err := s.templateSvc.Resolve(ctx, *cfg.TemplateID, mergedVars)
		if err != nil {
			return "", nil, fmt.Errorf("解析通用 pipeline 模板失败: %w", err)
		}
		return yamlContent, nil, nil
	case model.PipelineConfigSourceCompose:
		if s.templateSvc == nil {
			return "", nil, fmt.Errorf("pipeline template service not configured")
		}
		if len(cfg.ComposeSteps) == 0 {
			return "", nil, fmt.Errorf("compose_steps is empty")
		}
		// compose 模式只用 repo_ctx 作为全局 vars; per-ref 的 Variables
		// 在 ResolveCompose 内部按片段覆盖.
		globalVars := s.BuildRepoRenderContext(ctx, repo, cfg, branch, commit, author)
		specDef, err := s.templateSvc.ResolveCompose(ctx, cfg.ComposeSteps, globalVars)
		if err != nil {
			return "", nil, fmt.Errorf("组装步骤模板失败: %w", err)
		}
		return "", specDef, nil
	default: // inline
		return cfg.Content, nil, nil
	}
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

// BuildAndEnqueueInput 描述一次 pipeline run 创建所需的全部输入. Repo 触发
// 与独立 Job 触发都通过 BuildAndEnqueueRun 走同一份 spec→Pipeline+Workflow+
// Step+Task 的构造逻辑, 仅在 owner 信息 (RepoID/JobID/OwnerKind) 和 git 元
// 数据 (RepoURL/RepoClone/RepoBranch, 可空) 上不同.
type BuildAndEnqueueInput struct {
	OwnerKind string // model.PipelineOwnerRepo | PipelineOwnerJob
	RepoID    int64  // job 触发时为 0
	JobID     int64  // repo 触发时为 0
	Event     model.WebhookEvent
	Author    string
	Message   string
	Title     string
	Branch    string
	Commit    string
	Variables map[string]string
	YAML      string
	// SpecOverride 非 nil 时跳过 spec.Parse(YAML), 直接使用传入的
	// PipelineSpec. 用于 source=compose 这类已经在外部完成解析+合并的场景.
	SpecOverride *spec.PipelineSpec
	// Git 元数据; 如果留空, payload 里 RepoClone="" 让 handleTask 跳过 clone.
	RepoURL    string
	RepoClone  string
	RepoBranch string
	// 注入到 task.Labels 的额外 kv (比如 repo full_name / job name).
	ExtraLabels map[string]string
	// 触发完成后做 retention 清理的回调; 可选 (job 当前不做 retention).
	AfterEnqueue func(pipeline *model.Pipeline)
}

func (s *Service) triggerPipelineWithEvent(ctx context.Context, repo *model.Repo, cfg *model.RepoPipelineConfig, opts model.PipelineOptions, event model.WebhookEvent, author, message, title string) (*model.Pipeline, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if cfg == nil {
		return nil, fmt.Errorf("pipeline configuration missing")
	}

	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
		if branch == "" {
			branch = "main"
		}
	}
	commit := strings.TrimSpace(opts.Commit)
	normalizedAuthor := strings.TrimSpace(author)
	if normalizedAuthor == "" {
		normalizedAuthor = "system"
	}

	yamlContent, specOverride, err := s.resolvePipelineForTrigger(ctx, repo, cfg, branch, commit, normalizedAuthor)
	if err != nil {
		return nil, err
	}
	if specOverride == nil && strings.TrimSpace(yamlContent) == "" {
		return nil, fmt.Errorf("pipeline configuration missing")
	}

	extraLabels := map[string]string{
		"repo": repo.FullName,
		"org":  fmt.Sprintf("%d", repo.OrgID),
	}

	return s.BuildAndEnqueueRun(ctx, BuildAndEnqueueInput{
		OwnerKind:    model.PipelineOwnerRepo,
		RepoID:       repo.ID,
		Event:        event,
		Author:       author,
		Message:      message,
		Title:        title,
		Branch:       branch,
		Commit:       commit,
		Variables:    opts.Variables,
		YAML:         yamlContent,
		SpecOverride: specOverride,
		RepoURL:      repo.ForgeURL,
		RepoClone:    repo.Clone,
		RepoBranch:   repo.Branch,
		ExtraLabels:  extraLabels,
		AfterEnqueue: func(pipeline *model.Pipeline) {
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
		},
	})
}

// BuildAndEnqueueRun 是 repo / job 共用的 pipeline run 构造与入队逻辑.
// 调用方只需准备好已渲染过 ${VAR} 的 YAML 与 owner 元数据;
// 也可以直接传 SpecOverride 跳过 YAML 解析 (compose 模式).
func (s *Service) BuildAndEnqueueRun(ctx context.Context, in BuildAndEnqueueInput) (*model.Pipeline, error) {
	if in.SpecOverride == nil && strings.TrimSpace(in.YAML) == "" {
		return nil, fmt.Errorf("pipeline yaml is empty")
	}
	ownerKind := in.OwnerKind
	if ownerKind == "" {
		ownerKind = model.PipelineOwnerRepo
	}

	normalizedAuthor := strings.TrimSpace(in.Author)
	if normalizedAuthor == "" {
		normalizedAuthor = "system"
	}

	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = "main"
	}

	variables := in.Variables
	if variables == nil {
		variables = map[string]string{}
	}

	var (
		specDef *spec.PipelineSpec
		err     error
	)
	if in.SpecOverride != nil {
		specDef = in.SpecOverride
	} else {
		specDef, err = spec.Parse(in.YAML)
		if err != nil {
			return nil, err
		}
	}

	event := in.Event
	if string(event) == "" {
		event = model.EventManual
	}

	runMessage := strings.TrimSpace(in.Message)
	if runMessage == "" {
		runMessage = defaultPipelineMessage(event, normalizedAuthor)
	}
	runTitle := strings.TrimSpace(in.Title)
	if runTitle == "" {
		runTitle = fmt.Sprintf("%s run", string(event))
	}

	now := time.Now().Unix()
	pipeline := &model.Pipeline{
		RepoID:              in.RepoID,
		JobID:               in.JobID,
		OwnerKind:           ownerKind,
		Author:              normalizedAuthor,
		Event:               event,
		Status:              model.StatusPending,
		Message:             runMessage,
		Title:               runTitle,
		Created:             now,
		Updated:             now,
		Branch:              branch,
		Ref:                 fmt.Sprintf("refs/heads/%s", branch),
		Commit:              strings.TrimSpace(in.Commit),
		AdditionalVariables: variables,
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
		var buildCfg *pipelineBuildConfig
		if stepSpec.Kind == spec.StepKindBuild && stepSpec.Build != nil {
			stepType = model.StepTypeBuild
			b := stepSpec.Build
			buildCfg = &pipelineBuildConfig{
				Registry:      b.Registry,
				Repo:          b.Repo,
				Username:      b.Username,
				Password:      b.Password,
				Dockerfile:    b.Dockerfile,
				Context:       b.Context,
				Tags:          append([]string{}, b.Tags...),
				Platforms:     append([]string{}, b.Platforms...),
				Push:          b.Push,
				Target:        b.Target,
				NoCache:       b.NoCache,
				BuildkitImage: b.BuildkitImage,
				Privileged:    b.Privileged,
			}
			if len(b.BuildArgs) > 0 {
				buildCfg.BuildArgs = make(map[string]string, len(b.BuildArgs))
				for k, v := range b.BuildArgs {
					buildCfg.BuildArgs[k] = v
				}
			}
		}
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
		stepConditions := buildPipelineStepConditions(stepSpec.Conditions)
		stepImage := stepSpec.Image
		if buildCfg != nil {
			img := strings.TrimSpace(buildCfg.BuildkitImage)
			if img == "" {
				img = defaultBuildkitImage
			}
			stepImage = img
		}
		taskSteps = append(taskSteps, pipelineTaskStep{
			PID:        pid,
			Name:       stepName,
			Image:      stepImage,
			Commands:   append([]string{}, stepSpec.Commands...),
			Secrets:    stepSpec.Secrets,
			Env:        stepEnvVars,
			Volumes:    append([]string{}, stepSpec.Volumes...),
			Privileged: stepSpec.Privileged,
			Type:       stepType,
			Approval:   approvalTaskCfg,
			Plugin:     pluginCfg,
			Build:      buildCfg,
			Conditions: stepConditions,
		})
	}

	taskLabels := map[string]string{}
	for k, v := range in.ExtraLabels {
		taskLabels[k] = v
	}
	task := &model.Task{
		ID:           generateRandomID("task"),
		PID:          1,
		Name:         "",
		Dependencies: []string{},
		RunOn:        []string{string(model.StatusSuccess)},
		DepStatus:    map[string]model.StatusValue{},
		Labels:       taskLabels,
	}

	if err := s.CreatePipeline(ctx, pipeline, []*model.Workflow{workflow}, steps, []*model.Task{task}); err != nil {
		return nil, err
	}

	payload := pipelineTaskPayload{
		PipelineID:    pipeline.ID,
		RepoID:        in.RepoID,
		Branch:        branch,
		Commit:        pipeline.Commit,
		RunName:       workflow.Name,
		RepoURL:       in.RepoURL,
		RepoClone:     in.RepoClone,
		RepoBranch:    in.RepoBranch,
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

	if in.AfterEnqueue != nil {
		in.AfterEnqueue(pipeline)
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
		// 显式排除 job 触发的行 (owner_kind='job'), 避免历史数据回填后
		// 把 RepoID=0 的 Job 混进来.
		query := tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Where("repo_id = ? AND (owner_kind = ? OR owner_kind = '' OR owner_kind IS NULL)", repoID, model.PipelineOwnerRepo)
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

// ListPipelinesByJob 与 ListPipelinesByRepo 同形态, 仅 where 条件换成 job_id +
// owner_kind=job. 用于独立 Job 的运行历史 Tab.
func (s *Service) ListPipelinesByJob(ctx context.Context, jobID int64, page, perPage int) ([]*model.Pipeline, int64, error) {
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
			Where("job_id = ? AND owner_kind = ?", jobID, model.PipelineOwnerJob)
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
	return s.getRunDetailWhere(ctx, pipelineID, "id = ? AND repo_id = ? AND (owner_kind = ? OR owner_kind = '' OR owner_kind IS NULL)", []any{pipelineID, repoID, model.PipelineOwnerRepo})
}

// GetJobPipelineRunDetail 是 GetPipelineRunDetail 的 Job 变体, 按 job_id 鉴权.
func (s *Service) GetJobPipelineRunDetail(ctx context.Context, jobID, pipelineID int64) (*PipelineRunDetail, error) {
	return s.getRunDetailWhere(ctx, pipelineID, "id = ? AND job_id = ? AND owner_kind = ?", []any{pipelineID, jobID, model.PipelineOwnerJob})
}

func (s *Service) getRunDetailWhere(ctx context.Context, pipelineID int64, whereClause string, whereArgs []any) (*PipelineRunDetail, error) {
	detail := &PipelineRunDetail{
		Workflows: []*model.Workflow{},
		Steps:     []*model.Step{},
		Logs:      map[int64][]model.LogEntry{},
	}

	err := s.db.View(func(tx *gorm.DB) error {
		var pipeline model.Pipeline
		if err := tx.WithContext(ctx).
			Where(whereClause, whereArgs...).
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
	return s.submitStepApproval(ctx, model.PipelineOwnerRepo, repoID, pipelineID, stepID, actor, action, comment)
}

// SubmitJobStepApproval 是 SubmitStepApproval 的 Job 变体, 按 job_id 鉴权.
func (s *Service) SubmitJobStepApproval(ctx context.Context, jobID, pipelineID, stepID int64, actor string, action string, comment string) (*model.Step, error) {
	return s.submitStepApproval(ctx, model.PipelineOwnerJob, jobID, pipelineID, stepID, actor, action, comment)
}

func (s *Service) submitStepApproval(ctx context.Context, ownerKind string, ownerID int64, pipelineID, stepID int64, actor string, action string, comment string) (*model.Step, error) {
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
	if !pipelineOwnerMatches(&pipeline, ownerKind, ownerID) {
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

	// payload.RepoID == 0 表示这是一次独立 Job 触发, 没有 repo 行可拉.
	// 构造一个仅有 payload 字段的合成 repo, 让下游 prepareWorkspace /
	// provideRepoEnv 等逻辑无差别工作; 不会写库.
	var repo *model.Repo
	if payload.RepoID > 0 {
		repo, err = s.fetchRepo(ctx, payload.RepoID)
		if err != nil {
			return err
		}
	} else {
		repo = syntheticJobRepo(payload)
	}

	pipelineRecord, err := s.fetchPipeline(ctx, payload.PipelineID)
	if err != nil {
		return err
	}

	// Job 没有 repo_pipeline_configs 行, 用默认配置避免 GetPipelineSettings
	// 触发的二次 ensure 写入一个 repo_id=0 的脏行.
	var settings *model.RepoPipelineConfig
	if repo.ID > 0 {
		settings, err = s.GetPipelineSettings(ctx, repo.ID)
		if err != nil {
			return err
		}
	} else {
		settings = defaultPipelineSettings()
	}

	s.discoverCertAliasesFromSettings(ctx, payload.Steps)
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
			_ = logger(fmt.Sprintf("未检测到仓库中的 Dockerfile, 已使用系统配置的 Dockerfile (%d 字节, 写入 %s)",
				len(template), dockerfilePath))
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
		// 多维 when 评估: branch + event + ref + repo + (status 在 step
		// dispatch 前基本未知, 留空让默认行为接管). repo 用 task.Labels
		// 里 ApplyLabelsFromRepo 写进去的 "repo" full_name.
		trigger := triggerContext{
			Branch: currentBranch,
			Event:  string(pipelineRecord.Event),
			Ref:    pipelineRecord.Ref,
			Repo:   task.Labels["repo"],
		}
		if !execStep.allowsTrigger(trigger) {
			summary := ""
			if execStep.Conditions != nil {
				summary = execStep.Conditions.branchSummary()
			}
			logMessage := "步骤因 when 条件被跳过"
			switch {
			case summary != "" && currentBranch != "":
				logMessage = fmt.Sprintf("%s（当前分支 %s，要求 %s）", logMessage, currentBranch, summary)
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
			if isDockerPluginImage(execStep.Image) {
				autofillDockerPluginEnv(pluginEnv, stepSecrets)
			}
			if leftover := findUnresolvedPlaceholders(pluginEnv); len(leftover) > 0 {
				err := fmt.Errorf("步骤 %q 的 plugin 设置存在未解析的占位符 %v; 请检查 step 是否声明了 certificate: <name>, 凭证名是否与凭证管理中一致, 类型是否匹配", execStep.Name, leftover)
				_ = logFn(err.Error())
				pipelineStatus = model.StatusFailure
				failureMessage = err.Error()
				_ = s.setStepFinished(ctx, stepRecord.ID, statusFromPipeline(pipelineStatus), time.Now().Unix(), err, -1)
				break
			}
			for key, value := range pluginEnv {
				stepEnv[key] = value
			}
		}

		useBuildRuntime := execStep.Type == model.StepTypeBuild && execStep.Build != nil
		usePluginRuntime := !useBuildRuntime && execStep.Plugin != nil && len(execStep.Commands) == 0
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

		if useBuildRuntime {
			exitCode, err := s.runBuildStep(taskCtx, execStep, stepEnv, workspace, stepSecrets, ensureDockerfile, logFn)
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
		} else if usePluginRuntime {
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

// pipelineOwnerMatches 校验 pipeline 行是否属于指定 owner; 兼容老数据
// (owner_kind 为空当作 repo 处理).
func pipelineOwnerMatches(p *model.Pipeline, ownerKind string, ownerID int64) bool {
	if p == nil {
		return false
	}
	switch ownerKind {
	case model.PipelineOwnerJob:
		return p.OwnerKind == model.PipelineOwnerJob && p.JobID == ownerID
	default:
		// repo 触发: owner_kind 可能为空 (老数据) 也算 repo.
		if p.OwnerKind != "" && p.OwnerKind != model.PipelineOwnerRepo {
			return false
		}
		return p.RepoID == ownerID
	}
}

// syntheticJobRepo 构造一个 ID=0 的临时 Repo, 用于独立 Job 的 handleTask 路径.
// 字段从 task payload 取, 让 provideRepoEnv / prepareWorkspace 等不依赖 DB
// 的逻辑可以无差别复用. 不会被持久化.
func syntheticJobRepo(payload pipelineTaskPayload) *model.Repo {
	name := strings.TrimSpace(payload.RunName)
	if name == "" {
		name = fmt.Sprintf("job-%d", payload.PipelineID)
	}
	return &model.Repo{
		ID:       0,
		Name:     name,
		FullName: name,
		Owner:    "system",
		Branch:   strings.TrimSpace(payload.RepoBranch),
		Clone:    strings.TrimSpace(payload.RepoClone),
		ForgeURL: strings.TrimSpace(payload.RepoURL),
	}
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
		// repo.ID == 0 表示独立 Job, 没有 repo 行; 用 pipeline id 兜底.
		projectName = fmt.Sprintf("job-%d", pipelineID)
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
	envSlice := envMapToSlice(applyContainerEnvDefaults(stepEnv))
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
		if status == model.StatusFailure && strings.TrimSpace(message) != "" {
			errorsJSON, err := json.Marshal([]*model.PipelineError{{
				Type:    model.PipelineErrorTypeGeneric,
				Message: message,
			}})
			if err != nil {
				return err
			}
			update["errors"] = string(errorsJSON)
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

// hostOnlyEnvKeys 是默认要从 docker container env 里剥掉的 key 集合: 都是宿主机
// 进程会带、但容器里要么路径不存在要么会引发奇怪 bug 的 env. 用户在 step.env /
// pipelineEnv / 凭证里显式覆盖的同名 key 会在 stepEnv 里盖住, 不受这里影响.
//
//   - TMPDIR / TMP / TEMP : 宿主机临时目录, 容器里 stat 失败 (BuildKit 直接挂)
//   - HOME / PWD / OLDPWD : 宿主用户目录, git/cargo/npm 都会被带跑偏
//   - PATH                : 宿主路径在容器里 ENOENT, 容器自己的默认 PATH 已够用
//   - USER / LOGNAME / SHELL : 容器里身份不一样, 强行给会触发部分工具检查失败
//   - LANG                : 容器 locale 通常没装这些, 输出乱码 / warn
//   - 各种 macOS / launchd 内部前缀: __CF*, Apple_*, XPC_*, COMMAND_MODE,
//     SSH_AUTH_SOCK, TERM_PROGRAM*, TERM_SESSION_*, _, "0" 等
var hostOnlyEnvKeys = map[string]struct{}{
	"TMPDIR": {}, "TMP": {}, "TEMP": {},
	"HOME": {}, "PWD": {}, "OLDPWD": {},
	"PATH":         {},
	"USER":         {},
	"LOGNAME":      {},
	"SHELL":        {},
	"LANG":         {},
	"COMMAND_MODE": {},
	"SSH_AUTH_SOCK": {},
	"_":             {},
	"0":             {},
}

// hostOnlyEnvPrefixes 是按前缀匹配剥掉的 env, 主要是 macOS / locale / terminal
// 相关内部 env, 容器里完全用不上.
var hostOnlyEnvPrefixes = []string{
	"LC_",
	"XPC_",
	"__CF",
	"Apple_",
	"TERM_PROGRAM",
	"TERM_SESSION_",
}

// gitSafeDirectoryEnv 返回 git 官方支持的进程级 config 注入: 把 safe.directory=*
// 设到容器 env 里, 让 step 容器内的 git (典型的 alpine/git clone step) 不再因为
// host bind mount 透传宿主 UID 跟容器 root UID 不一致而拒绝操作 (CVE-2022-24765
// 加固).
//
// 这条 env 只对 git 有效, 其它工具忽略, 安全且自包含: 不动用户的 ~/.gitconfig,
// 不依赖 image 内置 git 配置, 不污染任何持久状态.
//
// 文档: https://git-scm.com/docs/git-config#ENVIRONMENT (GIT_CONFIG_COUNT / KEY / VALUE).
func gitSafeDirectoryEnv() map[string]string {
	return map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "safe.directory",
		"GIT_CONFIG_VALUE_0": "*",
	}
}

// applyContainerEnvDefaults 在 containerSafeEnv 剥完宿主专属 env 之后, 再补上
// 引擎需要默认注入的 env (目前只有 git safe.directory). 拆出来是为了让三处
// 容器入口共用同一份默认值. 用户在 step.env / pipelineEnv 显式覆盖的同名 key
// 会保留原值, 不被默认值盖.
func applyContainerEnvDefaults(env map[string]string) map[string]string {
	out := containerSafeEnv(env)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range gitSafeDirectoryEnv() {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

// containerSafeEnv 返回一份过滤过的副本, 把 hostOnlyEnvKeys / hostOnlyEnvPrefixes
// 命中的 key 都剥掉, 给 docker container 用. 调用方需要手动把容器需要的 env
// (DOCKER_CONFIG / TMPDIR=/tmp 等) 显式塞回去.
func containerSafeEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			continue
		}
		if _, drop := hostOnlyEnvKeys[trimmed]; drop {
			continue
		}
		skip := false
		for _, p := range hostOnlyEnvPrefixes {
			if strings.HasPrefix(trimmed, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out[k] = v
	}
	return out
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

// BuildRepoRenderContext 给「模板渲染前」提供已知的 repo + 触发参数变量,
// 让 ${CI_REPO_FULL_NAME / REPO_CLONE_URL_AUTH / CI_PIPELINE_BRANCH ...}
// 这种占位符在 source=template 流程下能被解析成项目实际值, 而不是
// 落空 -> ""。pipeline.ID / pipeline.Number / pipeline_name 在 CreatePipeline
// 之前未知, 这里不输出; 用户若想在 commands 字符串里用, 可依赖执行器
// 后续 provideXxxEnv 注入的 env vars.
//
// settings 可空 (代表使用默认 repo 设置), 仅用于 buildCertificateEnv 拿
// REPO_CLONE_URL_AUTH 的认证 URL.
//
// 暴露成 public 方法是为了让路由层 /pipeline-templates/:id/render 在拿到
// 项目 repo_id 时也能给预览接口注入同一份上下文, 保证预览结果与真实
// 触发完全一致.
func (s *Service) BuildRepoRenderContext(ctx context.Context, repo *model.Repo, settings *model.RepoPipelineConfig, branch, commit, author string) map[string]string {
	if repo == nil {
		return map[string]string{}
	}
	cloneURL := strings.TrimSpace(repo.Clone)
	if cloneURL == "" {
		cloneURL = strings.TrimSpace(repo.ForgeURL)
	}
	cloneAuth := cloneURL
	// 复用 buildCertificateEnv: 第三参数 nil 表示返回 settings.LegacyCertificates
	// 全集; 我们只关心其副产物 cloneOverride (带凭证的 clone URL).
	if s != nil && repo.ID > 0 && settings != nil {
		if _, override, _ := s.buildCertificateEnv(ctx, repo, settings, nil); strings.TrimSpace(override) != "" {
			cloneAuth = override
		}
	}
	out := map[string]string{
		"CI":                  "true",
		"CI_REPO_ID":          fmt.Sprintf("%d", repo.ID),
		"CI_REPO_NAME":        repo.Name,
		"CI_REPO_OWNER":       repo.Owner,
		"CI_REPO_FULL_NAME":   repo.FullName,
		"CI_DEFAULT_BRANCH":   repo.Branch,
		"CI_PIPELINE_BRANCH":  branch,
		"CI_COMMIT_BRANCH":    branch,
		"CI_PIPELINE_AUTHOR":  author,
		"CI_COMMIT_SHA":       commit,
		"COMMIT_ID":           commit,
		"COMMIT_ID_SHA":       commit,
		"REPO_URL":            cloneURL,
		"REPO_CLONE_URL":      cloneURL,
		"REPO_CLONE_URL_AUTH": cloneAuth,
		"REPO_WEB_URL":        repo.ForgeURL,
		"REPO_OWNER":          repo.Owner,
		"BRANCH":              branch,
	}
	return out
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

// defaultWorkspaceRoot 决定控制进程在没显式 payload.WorkspaceRoot / 没设
// PIPELINE_WORKSPACE_ROOT env 时的兜底路径.
//
// 走 ${HOME}/.devsys-workspace 而不是 /tmp/...: macOS 下 Colima 只默认共享
// $HOME 与 /tmp/colima, 不含 /tmp 自身. Docker Desktop / OrbStack 默认共享
// /Users 全树. ${HOME}/... 是三家 runtime 默认 file-sharing 列表里都包含的
// 最大公约数, Mac 控制进程和 Linux VM docker daemon 看的是同一份 fs, bind
// mount 的内容双向都见, 不会出现 BuildKit 看不到 controller 写入的 Dockerfile
// 这种问题.
//
// 容器化部署 (devsys 跑在自己的镜像里): 镜像在 ENV 里把 PIPELINE_WORKSPACE_ROOT
// 显式置成 /var/lib/devsys-workspace, 让用户用 host volume 挂同名路径即可.
func defaultWorkspaceRoot() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".devsys-workspace")
	}
	// 拿不到 home (极少数, 例如裸 root 进程没有 HOME env): 退回到平台安全路径.
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "devsys-workspace")
	}
	return "/var/lib/devsys-workspace"
}

func sanitizeWorkspaceRoot(root string) string {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		if env := strings.TrimSpace(os.Getenv("PIPELINE_WORKSPACE_ROOT")); env != "" {
			trimmed = env
		}
	}
	if trimmed == "" {
		return defaultWorkspaceRoot()
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

// findUnresolvedPlaceholders 扫描 env value 里残留的 ${...} 字面量, 返回
// "envKey=${placeholder}" 形式的列表, 已去重并排序便于错误消息稳定.
// 用于 plugin 步骤运行前的 fail-fast 校验; commands 不走这条路径 (允许 shell 用 ${VAR}).
func findUnresolvedPlaceholders(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		matches := unresolvedPlaceholderRegex.FindAllString(env[key], -1)
		for _, m := range matches {
			label := fmt.Sprintf("%s=%s", key, m)
			if _, dup := seen[label]; dup {
				continue
			}
			seen[label] = struct{}{}
			out = append(out, label)
		}
	}
	return out
}

// extractCertAliasesFromString 抓出字符串里所有 ${name.xxx...} 占位符的首段 name.
// 用于自动发现 step settings/env 里隐式引用的凭证名.
func extractCertAliasesFromString(s string) []string {
	if s == "" {
		return nil
	}
	matches := dottedSecretPlaceholderRegex.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// extractCertAliasesFromStep 综合扫 step.Plugin.Settings 与 step.Env 的所有 value.
func extractCertAliasesFromStep(step pipelineTaskStep) []string {
	seen := map[string]struct{}{}
	add := func(name string) {
		if name == "" {
			return
		}
		seen[name] = struct{}{}
	}
	if step.Plugin != nil {
		for _, values := range step.Plugin.Settings {
			for _, v := range values {
				for _, n := range extractCertAliasesFromString(v) {
					add(n)
				}
			}
		}
	}
	for _, v := range step.Env {
		for _, n := range extractCertAliasesFromString(v) {
			add(n)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// discoverCertAliasesFromSettings 把 step.Plugin.Settings / step.Env 里隐式引用的
// 凭证名补到 step.Secrets, 减少模板编写者忘写 certificate: 字段时的 401 噩梦.
// 仅当 systemSvc 能查到同名凭证才追加, 避免把误写的普通变量名 ($MY_VAR.x) 当凭证.
// 结果会去重, 与已有 secrets 大小写不敏感比较.
func (s *Service) discoverCertAliasesFromSettings(ctx context.Context, steps []pipelineTaskStep) {
	if s == nil || s.systemSvc == nil || len(steps) == 0 {
		return
	}
	exists := map[string]bool{}
	for i := range steps {
		candidates := extractCertAliasesFromStep(steps[i])
		if len(candidates) == 0 {
			continue
		}
		existing := map[string]struct{}{}
		for _, alias := range steps[i].Secrets {
			existing[strings.ToLower(strings.TrimSpace(alias))] = struct{}{}
		}
		for _, name := range candidates {
			key := strings.ToLower(name)
			if _, ok := existing[key]; ok {
				continue
			}
			ok, cached := exists[key]
			if !cached {
				cert, err := s.systemSvc.GetCertificateByName(ctx, name)
				ok = err == nil && cert != nil
				exists[key] = ok
			}
			if !ok {
				continue
			}
			steps[i].Secrets = append(steps[i].Secrets, name)
			existing[key] = struct{}{}
		}
	}
}

// normalizeDockerRepo 去掉 docker registry URL 上常见的 http(s):// 前缀和尾部 /,
// docker login / push 都不接受这类形式, 但用户在凭证管理填值时很容易顺手粘进去.
func normalizeDockerRepo(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(s), prefix) {
			s = s[len(prefix):]
			break
		}
	}
	s = strings.TrimRight(s, "/")
	return s
}

// isDockerPluginImage 简单匹配常见的 docker registry 推送插件镜像名,
// 用于决定是否走 PLUGIN_USERNAME/PASSWORD/REGISTRY 自动注入.
func isDockerPluginImage(image string) bool {
	img := strings.ToLower(strings.TrimSpace(image))
	if img == "" {
		return false
	}
	if idx := strings.Index(img, ":"); idx > 0 {
		img = img[:idx]
	}
	switch {
	case strings.HasPrefix(img, "woodpeckerci/plugin-docker"):
		return true
	case strings.HasPrefix(img, "plugins/docker"):
		return true
	case strings.Contains(img, "docker-buildx"):
		return true
	}
	return false
}

// autofillDockerPluginEnv 在 step 用 docker registry 推送插件且声明了 docker 凭证的
// 场景下, 把 PLUGIN_USERNAME/PLUGIN_PASSWORD/PLUGIN_REGISTRY 自动从凭证里填上,
// 让用户的 settings: 只需写 repo / dockerfile / tags 即可. 已显式提供的 PLUGIN_* 不覆盖.
// 仅消费第一个 docker 类型 binding (按 alias 字典序), 多 docker 凭证场景仍可显式 ${alias.docker.*} 指明.
func autofillDockerPluginEnv(pluginEnv map[string]string, stepSecrets map[string]resolvedSecretBinding) {
	if pluginEnv == nil || len(stepSecrets) == 0 {
		return
	}
	keys := make([]string, 0, len(stepSecrets))
	for k, b := range stepSecrets {
		if strings.ToLower(strings.TrimSpace(b.Type)) == "docker" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	sort.Strings(keys)
	binding := stepSecrets[keys[0]]
	set := func(key, value string) {
		if value == "" {
			return
		}
		if existing, ok := pluginEnv[key]; ok {
			trimmed := strings.TrimSpace(existing)
			// 把残留 ${...} 字面量也视作"未提供", 用凭证值覆盖.
			if trimmed != "" && !unresolvedPlaceholderRegex.MatchString(trimmed) {
				return
			}
		}
		pluginEnv[key] = value
	}
	set("PLUGIN_USERNAME", binding.Values["docker.username"])
	set("PLUGIN_PASSWORD", binding.Values["docker.password"])
	set("PLUGIN_REGISTRY", binding.Values["docker.registry"])
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

// runBuildStep 跑一个 kind=build 步骤. 在容器内启 moby/buildkit 的 daemonless
// 模式 (buildctl-daemonless.sh build ...) 拉 Dockerfile 推镜像, 不需要 dockerd.
//
// 流程:
//  1. registry/username/password 解析: 显式 build.* 优先, 否则从 stepSecrets 找
//     首个 type=docker 的凭证回填. 任一缺失则 fail-fast 给出清晰提示.
//  2. 在 host 侧 workspace_root/.devsys-buildkit/<step_name>/ 写一份临时
//     ~/.docker/config.json (含 base64(user:pass) 的 auths.<registry>), step
//     结束时 RemoveAll. 文件 mode 0600.
//  3. 拼 buildctl 参数: --frontend dockerfile.v0 --local context=/workspace/<ctx>
//     --local dockerfile=<dockerfile_dir> --opt filename=<dockerfile_basename>
//     --opt platform=<csv> --opt build-arg:K=V... --opt target=... --opt no-cache=true
//     --output type=image,name=<reg>/<repo>:<tag1>,name=...,push=true|false.
//  4. dockerruntime 启动: rootless 模式加 SecurityOpt (seccomp/apparmor/systempaths
//     unconfined), :latest 模式给 Privileged. DOCKER_CONFIG 指向 mount 进容器
//     的 .devsys-buildkit/<step>/ 目录.
func (s *Service) runBuildStep(ctx context.Context, step pipelineTaskStep, stepEnv map[string]string, workspace string, stepSecrets map[string]resolvedSecretBinding, ensureDockerfile func(bool, func(string) error) error, logFn func(string) error) (int, error) {
	build := step.Build
	if build == nil {
		return -1, fmt.Errorf("build configuration missing")
	}
	if strings.TrimSpace(workspace) == "" {
		return -1, fmt.Errorf("workspace not prepared")
	}
	if ensureDockerfile != nil {
		if err := ensureDockerfile(true, logFn); err != nil {
			return -1, err
		}
		defer ensureDockerfile(false, logFn)
	}

	registry, username, password, err := resolveBuildRegistryCredentials(step.Name, build, stepSecrets)
	if err != nil {
		return -1, err
	}

	// 写临时 docker config.json. 路径放在 workspace 子目录里, 结束时清理.
	configHostDir := filepath.Join(workspace, ".devsys-buildkit", sanitizeContainerName(step.Name))
	if err := os.MkdirAll(configHostDir, 0o700); err != nil {
		return -1, fmt.Errorf("准备 buildkit 工作目录失败: %w", err)
	}
	defer os.RemoveAll(filepath.Join(workspace, ".devsys-buildkit"))

	authJSON, err := buildDockerAuthConfigJSON(registry, username, password)
	if err != nil {
		return -1, err
	}
	configHostPath := filepath.Join(configHostDir, "config.json")
	if err := os.WriteFile(configHostPath, authJSON, 0o600); err != nil {
		return -1, fmt.Errorf("写入 docker config 失败: %w", err)
	}

	containerWorkspace := "/workspace"
	relConfigDir := strings.TrimPrefix(configHostDir, workspace)
	relConfigDir = strings.TrimPrefix(relConfigDir, string(filepath.Separator))
	containerConfigDir := path.Join(containerWorkspace, filepath.ToSlash(relConfigDir))

	args := buildBuildctlArgs(build, registry, containerWorkspace)

	// 运行模式由镜像名 + 显式 build.Privileged 共同决定:
	//   - 镜像名含 "rootless" 且 build.Privileged 未强制 true → rootless 模式
	//     (Docker 不加 --privileged, 加少量必要的 SecurityOpt).
	//   - 否则 → privileged 模式 (Docker --privileged, 不动 SecurityOpt).
	// 默认镜像 moby/buildkit:latest 自动落到 privileged 分支, 兼容所有 Docker daemon.
	imageLower := strings.ToLower(step.Image)
	rootlessImage := strings.Contains(imageLower, "rootless")
	usePrivileged := build.Privileged || !rootlessImage

	if logFn != nil {
		_ = logFn(fmt.Sprintf("BuildKit registry=%s repo=%s tags=%v platforms=%v push=%v mode=%s",
			registry, build.Repo, build.Tags, build.Platforms, buildPushEnabled(build),
			privilegedModeLabel(usePrivileged)))
	}

	// 先剥宿主机专属 env (TMPDIR/HOME/PATH 等), 再回填 BuildKit / docker login
	// 需要的几个 key. 否则宿主 macOS 的 TMPDIR=/var/folders/... 会被 BuildKit
	// os.MkdirTemp(os.TempDir(), ...) 当作 Linux 容器里的路径去 stat, 直接挂掉.
	// applyContainerEnvDefaults 同时会注入 git safe.directory=*, 不影响 BuildKit.
	containerEnv := applyContainerEnvDefaults(pluginContainerEnv(stepEnv))
	containerEnv["DOCKER_CONFIG"] = containerConfigDir
	if _, ok := containerEnv["TMPDIR"]; !ok {
		containerEnv["TMPDIR"] = "/tmp"
	}
	if _, ok := containerEnv["HOME"]; !ok {
		containerEnv["HOME"] = "/tmp"
	}
	// rootless 模式默认禁用 process sandbox, 节省启动时间且与常见 CI 容器内核兼容.
	if !usePrivileged {
		if _, ok := containerEnv["BUILDKITD_FLAGS"]; !ok {
			containerEnv["BUILDKITD_FLAGS"] = "--oci-worker-no-process-sandbox"
		}
	}

	cfg := dockerruntime.ContainerConfig{
		Name:       pluginContainerName(step, stepEnv),
		Image:      step.Image,
		Entrypoint: []string{"buildctl-daemonless.sh"},
		Cmd:        args,
		Env:        envMapToSlice(containerEnv),
		WorkingDir: containerWorkspace,
		Volumes:    map[string]struct{}{containerWorkspace: {}},
		Binds:      []string{fmt.Sprintf("%s:%s", workspace, containerWorkspace)},
		Privileged: usePrivileged,
	}
	if !usePrivileged {
		// 故意不带 systempaths=unconfined: Docker Engine 20.10+ Linux 才接受,
		// Colima / Docker Desktop / 旧 Docker 都会以 "invalid --security-opt"
		// 拒收, 让容器根本起不来. 缺它在某些环境 rootless buildkitd 启动会受限,
		// 那时再 fallback 到 privileged (默认行为).
		cfg.SecurityOpt = []string{
			"seccomp=unconfined",
			"apparmor=unconfined",
		}
	}
	runner, err := s.dockerRunner()
	if err != nil {
		return -1, err
	}
	return runner.Run(ctx, cfg, logFn)
}

func privilegedModeLabel(privileged bool) string {
	if privileged {
		return "privileged"
	}
	return "rootless"
}

func buildPushEnabled(b *pipelineBuildConfig) bool {
	if b == nil || b.Push == nil {
		return true
	}
	return *b.Push
}

// resolveBuildRegistryCredentials 拼出 docker registry 推送所需的 (registry,
// username, password). 显式 build.* 字段优先, 否则从 stepSecrets 找首个 type=docker
// 凭证回填. 三件套缺一不可, 否则 fail-fast.
func resolveBuildRegistryCredentials(stepName string, build *pipelineBuildConfig, stepSecrets map[string]resolvedSecretBinding) (string, string, string, error) {
	registry := normalizeDockerRepo(build.Registry)
	username := strings.TrimSpace(build.Username)
	password := build.Password

	if registry == "" || username == "" || password == "" {
		// 找首个 docker 凭证 (按 alias key 字典序确保稳定)
		keys := make([]string, 0, len(stepSecrets))
		for k, b := range stepSecrets {
			if strings.ToLower(strings.TrimSpace(b.Type)) == "docker" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			b := stepSecrets[keys[0]]
			if registry == "" {
				registry = normalizeDockerRepo(b.Values["docker.registry"])
				if registry == "" {
					registry = normalizeDockerRepo(b.Values["docker.repo"])
				}
			}
			if username == "" {
				username = b.Values["docker.username"]
			}
			if password == "" {
				password = b.Values["docker.password"]
			}
		}
	}

	missing := make([]string, 0, 3)
	if registry == "" {
		missing = append(missing, "registry")
	}
	if username == "" {
		missing = append(missing, "username")
	}
	if password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return "", "", "", fmt.Errorf("kind=build 步骤 %q 缺少 %v: 请在 build.* 显式设置, 或在 step 上声明 certificate: <docker_cert>", stepName, missing)
	}
	return registry, username, password, nil
}

// buildDockerAuthConfigJSON 渲染 ~/.docker/config.json. buildctl 会用它鉴权
// registry 推送; 字段格式与 docker CLI 写出来的一致.
func buildDockerAuthConfigJSON(registry, username, password string) ([]byte, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	cfg := map[string]any{
		"auths": map[string]any{
			registry: map[string]any{
				"auth":     auth,
				"username": username,
				"password": password,
			},
		},
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("序列化 docker config 失败: %w", err)
	}
	return out, nil
}

// buildBuildctlArgs 把 pipelineBuildConfig 转成 buildctl 命令行参数.
// containerWorkspace 是容器内 workspace 挂载点, 用于拼 --local 路径.
func buildBuildctlArgs(b *pipelineBuildConfig, registry, containerWorkspace string) []string {
	contextRel := strings.TrimSpace(b.Context)
	if contextRel == "" {
		contextRel = "."
	}
	contextPath := path.Join(containerWorkspace, filepath.ToSlash(contextRel))

	dockerfileRel := strings.TrimSpace(b.Dockerfile)
	if dockerfileRel == "" {
		dockerfileRel = "Dockerfile"
	}
	dockerfileBase := path.Base(filepath.ToSlash(dockerfileRel))
	dockerfileDirRel := path.Dir(filepath.ToSlash(dockerfileRel))
	if dockerfileDirRel == "" || dockerfileDirRel == "." {
		dockerfileDirRel = contextRel
	}
	dockerfilePath := path.Join(containerWorkspace, dockerfileDirRel)

	args := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + contextPath,
		"--local", "dockerfile=" + dockerfilePath,
		"--opt", "filename=" + dockerfileBase,
	}

	if len(b.Platforms) > 0 {
		args = append(args, "--opt", "platform="+strings.Join(b.Platforms, ","))
	}

	// 稳定顺序, 便于复现/对比日志.
	if len(b.BuildArgs) > 0 {
		keys := make([]string, 0, len(b.BuildArgs))
		for k := range b.BuildArgs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--opt", "build-arg:"+k+"="+b.BuildArgs[k])
		}
	}
	if strings.TrimSpace(b.Target) != "" {
		args = append(args, "--opt", "target="+strings.TrimSpace(b.Target))
	}
	if b.NoCache {
		args = append(args, "--opt", "no-cache=true")
	}

	tags := b.Tags
	if len(tags) == 0 {
		tags = []string{"latest"}
	}
	output := strings.Builder{}
	output.WriteString("type=image")
	for _, tag := range tags {
		output.WriteString(",name=")
		output.WriteString(registry + "/" + b.Repo + ":" + tag)
	}
	if buildPushEnabled(b) {
		output.WriteString(",push=true")
	} else {
		output.WriteString(",push=false")
	}
	args = append(args, "--output", output.String())
	return args
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
		Env:        envMapToSlice(applyContainerEnvDefaults(pluginContainerEnv(stepEnv))),
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
		Source:           model.PipelineConfigSourceInline,
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
	// Source 字段在旧数据上可能为空, 强制回填为 inline 以便业务逻辑统一判断.
	if cfg.Source == "" {
		cfg.Source = model.PipelineConfigSourceInline
	}
	switch cfg.Source {
	case model.PipelineConfigSourceTemplate:
		// 模板模式: 清空 compose 残留, 保证 TemplateVariables 至少是空 map
		// 让前端表单不需要做 nil 判断.
		cfg.ComposeSteps = nil
		if cfg.TemplateVariables == nil {
			cfg.TemplateVariables = map[string]string{}
		}
	case model.PipelineConfigSourceCompose:
		cfg.TemplateID = nil
		cfg.TemplateVariables = nil
		if cfg.ComposeSteps == nil {
			cfg.ComposeSteps = []model.ComposeStepRef{}
		}
	default: // inline (含 "")
		cfg.TemplateID = nil
		cfg.TemplateVariables = nil
		cfg.ComposeSteps = nil
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

	return s.reloadJobCronSchedules(ctx)
}

// reloadJobCronSchedules 加载所有 PipelineJob.cron_schedules 到 scheduler.
// 与 reloadCronSchedules 同语义: 已注册但 DB 已删除的 jobID 也会被清理.
func (s *Service) reloadJobCronSchedules(ctx context.Context) error {
	type jobCronRecord struct {
		JobID         int64    `gorm:"column:id"`
		CronSchedules []string `gorm:"column:cron_schedules;serializer:json"`
	}
	var records []jobCronRecord
	if err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.PipelineJob{}).
			Select("id", "cron_schedules").
			Find(&records).Error
	}); err != nil {
		return err
	}

	seen := make(map[int64]struct{}, len(records))
	for _, record := range records {
		schedules := sanitizeCronSchedules(record.CronSchedules)
		s.RefreshJobCronEntries(record.JobID, schedules)
		seen[record.JobID] = struct{}{}
	}

	s.cronMu.Lock()
	existing := make([]int64, 0, len(s.jobCronEntries))
	for jobID := range s.jobCronEntries {
		existing = append(existing, jobID)
	}
	s.cronMu.Unlock()
	for _, jobID := range existing {
		if _, ok := seen[jobID]; !ok {
			s.RefreshJobCronEntries(jobID, nil)
		}
	}
	return nil
}

// SetJobScheduler 注入 Job 调度器实现. 在 services.go 装配 jobSvc 之后调用,
// 让 cron 触发 Job 时能回调到 job 服务. 之前未注入时, 任何 Job cron entry
// 触发都会被 runScheduledJobPipeline 直接忽略并打 warning, 不会 panic.
func (s *Service) SetJobScheduler(scheduler JobScheduler) {
	s.cronMu.Lock()
	s.jobScheduler = scheduler
	s.cronMu.Unlock()
}

// RefreshJobCronEntries 与 refreshCronEntries 同结构, 只是按 jobID 维护 entries
// 并在 cron 触发时调 runScheduledJobPipeline. 由 Job 服务在 Create / Update /
// Delete 后显式调用; 启动时也由 reloadCronSchedules 一次性装载.
func (s *Service) RefreshJobCronEntries(jobID int64, schedules []string) {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	if s.scheduler == nil {
		return
	}

	if ids, ok := s.jobCronEntries[jobID]; ok {
		for _, id := range ids {
			s.scheduler.Remove(id)
		}
		delete(s.jobCronEntries, jobID)
	}

	sanitized := sanitizeCronSchedules(schedules)
	if len(sanitized) == 0 {
		return
	}

	for _, spec := range sanitized {
		specCopy := spec
		entryID, err := s.scheduler.Add(specCopy, func() {
			s.runScheduledJobPipeline(jobID, specCopy)
		})
		if err != nil {
			log.Warn().Err(err).Int64("job_id", jobID).Str("cron_expression", specCopy).Msg("skipping invalid cron expression")
			continue
		}
		s.jobCronEntries[jobID] = append(s.jobCronEntries[jobID], entryID)
		log.Debug().Int64("job_id", jobID).Str("cron_expression", specCopy).Msg("registered cron job schedule")
	}
}

// runScheduledJobPipeline 将 Job cron 触发转发给 jobScheduler 实现. 与
// runScheduledPipeline 对仗, 但 Job 的实际构造逻辑放在 job 包以避免
// pipeline → job 直接耦合.
func (s *Service) runScheduledJobPipeline(jobID int64, expression string) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Int64("job_id", jobID).Str("cron_expression", expression).Msg("cron job panicked")
		}
	}()

	s.cronMu.Lock()
	scheduler := s.jobScheduler
	s.cronMu.Unlock()
	if scheduler == nil {
		log.Warn().Int64("job_id", jobID).Str("cron_expression", expression).Msg("job scheduler not configured, skipping cron tick")
		return
	}

	ctx := context.Background()
	log.Info().
		Int64("job_id", jobID).
		Str("cron_expression", expression).
		Msg("triggering scheduled job")
	if err := scheduler.TriggerCron(ctx, jobID, expression); err != nil {
		log.Error().Err(err).Int64("job_id", jobID).Str("cron_expression", expression).Msg("failed to trigger cron job")
	}
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
				dockerRepo := normalizeDockerRepo(dockerCert.Repo)
				env[fmt.Sprintf("%s_USERNAME", sanitized)] = dockerCert.Username
				env[fmt.Sprintf("%s_PASSWORD", sanitized)] = dockerCert.Password
				env[fmt.Sprintf("%s_REPO", sanitized)] = dockerRepo

				resolved.Values["docker.username"] = dockerCert.Username
				resolved.Values["docker.password"] = dockerCert.Password
				resolved.Values["docker.repo"] = dockerRepo
				resolved.Values["docker.registry"] = dockerRepo
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
				dockerRepo := normalizeDockerRepo(dockerCert.Repo)
				env[fmt.Sprintf("%s_USERNAME", sanitized)] = dockerCert.Username
				env[fmt.Sprintf("%s_PASSWORD", sanitized)] = dockerCert.Password
				env[fmt.Sprintf("%s_REPO", sanitized)] = dockerRepo

				resolved.Values["docker.username"] = dockerCert.Username
				resolved.Values["docker.password"] = dockerCert.Password
				resolved.Values["docker.repo"] = dockerRepo
				resolved.Values["docker.registry"] = dockerRepo
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
	return s.cancelPipelineRun(ctx, model.PipelineOwnerRepo, repoID, pipelineID, reason)
}

// CancelJobPipelineRun 是 CancelPipelineRun 的 Job 变体, 按 job_id 鉴权.
func (s *Service) CancelJobPipelineRun(ctx context.Context, jobID, pipelineID int64, reason string) error {
	return s.cancelPipelineRun(ctx, model.PipelineOwnerJob, jobID, pipelineID, reason)
}

func (s *Service) cancelPipelineRun(ctx context.Context, ownerKind string, ownerID int64, pipelineID int64, reason string) error {
	var pipeline model.Pipeline
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("id = ?", pipelineID).
			Take(&pipeline).Error
	})
	if err != nil {
		return err
	}
	if !pipelineOwnerMatches(&pipeline, ownerKind, ownerID) {
		return gorm.ErrRecordNotFound
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
