// Package job 管理独立于项目的 pipeline 任务 (PipelineJob).
//
// 与基于 Repo 的流水线相比, Job 没有 git webhook / 项目级配置, 编辑即生效;
// Trigger 时直接渲染 ${VAR} 占位符并复用 pipelineService.BuildAndEnqueueRun
// 走同一份 step / task / queue 链路.
//
// 启用 Git 时, payload.RepoClone / RepoBranch 由 Job.GitCloneURL/GitBranch
// 提供, 凭证作为环境变量注入 (commands 自行 git clone). 不启用时 RepoClone
// 留空, handleTask 仅创建空 workspace.
package job

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
	pipelinesvc "github.com/thepenn/devsys/service/pipeline"
	"github.com/thepenn/devsys/service/pipeline/spec"
	templatesvc "github.com/thepenn/devsys/service/pipeline/template"
	systemsvc "github.com/thepenn/devsys/service/system"
)

var (
	ErrJobNotFound      = errors.New("pipeline job not found")
	ErrJobNameConflict  = errors.New("pipeline job name already exists")
	ErrJobInvalidConfig = errors.New("pipeline job configuration invalid")
)

type Service struct {
	db          *store.DB
	pipelineSvc *pipelinesvc.Service
	systemSvc   *systemsvc.Service
}

// New 构造 Job service. systemSvc 可空, 仅用于在触发时把 git 凭证解密成 env.
func New(db *store.DB, pipelineSvc *pipelinesvc.Service, systemSvc *systemsvc.Service) *Service {
	return &Service{db: db, pipelineSvc: pipelineSvc, systemSvc: systemSvc}
}

// CreateInput 创建/更新通用参数.
type CreateInput struct {
	Name            string
	DisplayName     string
	Description     string
	Content         string
	GitEnabled      bool
	GitCloneURL     string
	GitBranch       string
	GitCredentialID *int64
	Variables       map[string]string
	CronSchedules   []string
	Actor           string
}

// UpdateInput 局部更新; nil 字段表示不修改.
type UpdateInput struct {
	DisplayName     *string
	Description     *string
	Content         *string
	GitEnabled      *bool
	GitCloneURL     *string
	GitBranch       *string
	GitCredentialID *int64
	ClearCredential bool
	Variables       *map[string]string
	// CronSchedules nil 表示不动; 指向 nil 切片 / 空切片表示清空所有调度.
	CronSchedules *[]string
	Actor         string
}

// ListOptions 列表查询.
type ListOptions struct {
	Page    int
	PerPage int
	Keyword string
}

// ListResult 分页结果, items 包含最近一次运行摘要.
type ListResult struct {
	Items   []JobSummary `json:"items"`
	Total   int64        `json:"total"`
	Page    int          `json:"page"`
	PerPage int          `json:"per_page"`
}

// JobSummary 列表行 (不含 YAML 内容).
type JobSummary struct {
	ID            int64             `json:"id"`
	Name          string            `json:"name"`
	DisplayName   string            `json:"display_name"`
	Description   string            `json:"description"`
	GitEnabled    bool              `json:"git_enabled"`
	CreatedBy     string            `json:"created_by"`
	UpdatedBy     string            `json:"updated_by"`
	Created       int64             `json:"created"`
	Updated       int64             `json:"updated"`
	LastRunStatus model.StatusValue `json:"last_run_status,omitempty"`
	LastRunTime   int64             `json:"last_run_time,omitempty"`
	LastRunNumber int64             `json:"last_run_number,omitempty"`
	TotalRuns     int64             `json:"total_runs"`
}

// Create 写入新 Job. Content 非空时做 spec.Parse 校验; Name 全局唯一.
func (s *Service) Create(ctx context.Context, in CreateInput) (*model.PipelineJob, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("job name is required")
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = name
	}
	if strings.TrimSpace(in.Content) != "" {
		if _, err := spec.Parse(in.Content); err != nil {
			return nil, fmt.Errorf("YAML 校验失败: %w", err)
		}
	}
	now := time.Now().Unix()
	job := &model.PipelineJob{
		Name:            name,
		DisplayName:     displayName,
		Description:     strings.TrimSpace(in.Description),
		Content:         in.Content,
		GitEnabled:      in.GitEnabled,
		GitCloneURL:     strings.TrimSpace(in.GitCloneURL),
		GitBranch:       strings.TrimSpace(in.GitBranch),
		GitCredentialID: in.GitCredentialID,
		Variables:       cloneStringMap(in.Variables),
		CronSchedules:   cleanCronSchedules(in.CronSchedules),
		CreatedBy:       in.Actor,
		UpdatedBy:       in.Actor,
		Created:         now,
		Updated:         now,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.PipelineJob
		err := tx.WithContext(ctx).Where("name = ?", name).Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.WithContext(ctx).Create(job).Error
		case err != nil:
			return err
		default:
			return ErrJobNameConflict
		}
	})
	if err != nil {
		return nil, err
	}
	if s.pipelineSvc != nil {
		s.pipelineSvc.RefreshJobCronEntries(job.ID, job.CronSchedules)
	}
	return job, nil
}

// Get 按 id 拉取.
func (s *Service) Get(ctx context.Context, id int64) (*model.PipelineJob, error) {
	if id <= 0 {
		return nil, ErrJobNotFound
	}
	var job model.PipelineJob
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&job, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Update 局部修改.
func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (*model.PipelineJob, error) {
	if in.Content != nil && strings.TrimSpace(*in.Content) != "" {
		if _, err := spec.Parse(*in.Content); err != nil {
			return nil, fmt.Errorf("YAML 校验失败: %w", err)
		}
	}
	var updated *model.PipelineJob
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var job model.PipelineJob
		if err := tx.WithContext(ctx).First(&job, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrJobNotFound
			}
			return err
		}
		if in.DisplayName != nil {
			job.DisplayName = strings.TrimSpace(*in.DisplayName)
			if job.DisplayName == "" {
				job.DisplayName = job.Name
			}
		}
		if in.Description != nil {
			job.Description = strings.TrimSpace(*in.Description)
		}
		if in.Content != nil {
			job.Content = *in.Content
		}
		if in.GitEnabled != nil {
			job.GitEnabled = *in.GitEnabled
		}
		if in.GitCloneURL != nil {
			job.GitCloneURL = strings.TrimSpace(*in.GitCloneURL)
		}
		if in.GitBranch != nil {
			job.GitBranch = strings.TrimSpace(*in.GitBranch)
		}
		if in.ClearCredential {
			job.GitCredentialID = nil
		} else if in.GitCredentialID != nil {
			job.GitCredentialID = in.GitCredentialID
		}
		if in.Variables != nil {
			job.Variables = cloneStringMap(*in.Variables)
		}
		if in.CronSchedules != nil {
			job.CronSchedules = cleanCronSchedules(*in.CronSchedules)
		}
		job.UpdatedBy = in.Actor
		job.Updated = time.Now().Unix()
		if err := tx.WithContext(ctx).Save(&job).Error; err != nil {
			return err
		}
		updated = &job
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 调度器同步在事务外做, 仅在 cron_schedules 真发生变更时刷新.
	// 这里偷懒: 任何更新都重新注册一遍, 与 RefreshJobCronEntries 的幂等
	// 行为一致, 不会泄漏 entries.
	if s.pipelineSvc != nil && updated != nil {
		s.pipelineSvc.RefreshJobCronEntries(updated.ID, updated.CronSchedules)
	}
	return updated, nil
}

// Delete 删除 Job. 当前不阻止有运行历史的 Job 被删除 (历史 pipelines 行
// 通过 owner_kind/job_id 仍可查询, 只是失去 Job 元数据).
func (s *Service) Delete(ctx context.Context, id int64) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var job model.PipelineJob
		if err := tx.WithContext(ctx).First(&job, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrJobNotFound
			}
			return err
		}
		return tx.WithContext(ctx).Delete(&model.PipelineJob{}, id).Error
	})
	if err != nil {
		return err
	}
	if s.pipelineSvc != nil {
		s.pipelineSvc.RefreshJobCronEntries(id, nil)
	}
	return nil
}

// List 按 keyword 模糊查询, 同时附带最近一次运行的 status / time.
func (s *Service) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 20
	}
	if opts.PerPage > 200 {
		opts.PerPage = 200
	}
	var (
		items []model.PipelineJob
		total int64
	)
	err := s.db.View(func(tx *gorm.DB) error {
		base := tx.WithContext(ctx).Model(&model.PipelineJob{})
		if kw := strings.TrimSpace(opts.Keyword); kw != "" {
			like := "%" + kw + "%"
			base = base.Where("name LIKE ? OR display_name LIKE ?", like, like)
		}
		if err := base.Count(&total).Error; err != nil {
			return err
		}
		return base.Order("id DESC").
			Offset((opts.Page - 1) * opts.PerPage).
			Limit(opts.PerPage).
			Find(&items).Error
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &ListResult{Items: []JobSummary{}, Total: total, Page: opts.Page, PerPage: opts.PerPage}, nil
	}
	ids := make([]int64, 0, len(items))
	for _, j := range items {
		ids = append(ids, j.ID)
	}
	lastByJob, totalsByJob, err := s.aggregateRuns(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]JobSummary, 0, len(items))
	for _, j := range items {
		summary := JobSummary{
			ID:          j.ID,
			Name:        j.Name,
			DisplayName: j.DisplayName,
			Description: j.Description,
			GitEnabled:  j.GitEnabled,
			CreatedBy:   j.CreatedBy,
			UpdatedBy:   j.UpdatedBy,
			Created:     j.Created,
			Updated:     j.Updated,
			TotalRuns:   totalsByJob[j.ID],
		}
		if last, ok := lastByJob[j.ID]; ok {
			summary.LastRunStatus = last.Status
			summary.LastRunTime = last.Updated
			summary.LastRunNumber = last.Number
		}
		out = append(out, summary)
	}
	return &ListResult{Items: out, Total: total, Page: opts.Page, PerPage: opts.PerPage}, nil
}

type lastRunRow struct {
	JobID   int64
	Number  int64
	Status  model.StatusValue
	Updated int64
}

// aggregateRuns 一次拉所有 jobIDs 的最近运行 + 总数, 减少 N+1.
func (s *Service) aggregateRuns(ctx context.Context, jobIDs []int64) (map[int64]lastRunRow, map[int64]int64, error) {
	lastMap := make(map[int64]lastRunRow, len(jobIDs))
	totals := make(map[int64]int64, len(jobIDs))
	if len(jobIDs) == 0 {
		return lastMap, totals, nil
	}
	type totalRow struct {
		JobID int64
		Total int64
	}
	var totalRows []totalRow
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.Pipeline{}).
			Select("job_id, COUNT(*) AS total").
			Where("job_id IN ? AND owner_kind = ?", jobIDs, model.PipelineOwnerJob).
			Group("job_id").
			Scan(&totalRows).Error
	})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range totalRows {
		totals[r.JobID] = r.Total
	}
	// 最近一次运行: 用窗口函数兼容性差, 简单用子查询取每个 job_id 的最大 id.
	var rows []lastRunRow
	err = s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Raw(`SELECT p.job_id, p.number, p.status, p.updated FROM pipelines p
			INNER JOIN (
				SELECT job_id, MAX(id) AS max_id FROM pipelines
				WHERE owner_kind = ? AND job_id IN ? GROUP BY job_id
			) m ON m.max_id = p.id`, model.PipelineOwnerJob, jobIDs).
			Scan(&rows).Error
	})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		lastMap[r.JobID] = r
	}
	return lastMap, totals, nil
}

// Trigger 手动触发一次 Job 运行 (event=manual).
func (s *Service) Trigger(ctx context.Context, jobID int64, opts model.PipelineOptions, author string) (*model.Pipeline, error) {
	authorName := strings.TrimSpace(author)
	if authorName == "" {
		authorName = "system"
	}
	return s.triggerJobWithEvent(ctx, jobID, opts, authorName, model.EventManual,
		fmt.Sprintf("手动触发 Job（%s）", authorName), "")
}

// TriggerCron 是 cron 调度器的回调入口. 实现了 pipelinesvc.JobScheduler 接口.
// 注入了 CRON_EXPRESSION / CRON_TRIGGERED_AT / CRON_TRIGGERED_BY 三个变量,
// 让 commands 能识别出当前由 cron 触发.
func (s *Service) TriggerCron(ctx context.Context, jobID int64, expression string) error {
	opts := model.PipelineOptions{
		Variables: map[string]string{
			"CRON_EXPRESSION":   expression,
			"CRON_TRIGGERED_AT": time.Now().UTC().Format(time.RFC3339),
			"CRON_TRIGGERED_BY": "cron",
		},
	}
	message := fmt.Sprintf("定时触发（%s）", expression)
	title := fmt.Sprintf("定时任务 - %s", expression)
	_, err := s.triggerJobWithEvent(ctx, jobID, opts, "cron", model.EventCron, message, title)
	return err
}

// triggerJobWithEvent 是 Trigger / TriggerCron 共用的内部实现, 仅 event /
// 默认变量 / 默认 message / title 上有差异.
func (s *Service) triggerJobWithEvent(ctx context.Context, jobID int64, opts model.PipelineOptions, author string, event model.WebhookEvent, message, title string) (*model.Pipeline, error) {
	job, err := s.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(job.Content) == "" {
		return nil, fmt.Errorf("job %s 未配置 YAML 内容", job.Name)
	}
	mergedVars := cloneStringMap(job.Variables)
	if mergedVars == nil {
		mergedVars = map[string]string{}
	}
	for k, v := range opts.Variables {
		mergedVars[k] = v
	}
	// 渲染优先级: opts.Variables ⊕ job.Variables (上面已合并)
	//             > Certificate 仓库按变量名 fallback
	//             > ${VAR:-default}
	//             > 空串
	rendered := templatesvc.RenderWithFallback(job.Content, mergedVars, templatesvc.NewCertificateFallback(ctx, s.systemSvc))

	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		branch = strings.TrimSpace(job.GitBranch)
	}
	if branch == "" {
		branch = "main"
	}

	repoClone := ""
	repoURL := ""
	repoBranch := strings.TrimSpace(job.GitBranch)
	if job.GitEnabled {
		repoClone = strings.TrimSpace(job.GitCloneURL)
		repoURL = repoClone
		if job.GitCredentialID != nil && s.systemSvc != nil {
			if cert, err := s.systemSvc.GetCertificateWithSecrets(ctx, *job.GitCredentialID); err == nil && cert != nil {
				if gitCert, err := cert.AsGitCertificate(); err == nil {
					if mergedVars["JOB_GIT_USERNAME"] == "" {
						mergedVars["JOB_GIT_USERNAME"] = gitCert.Username
					}
					if mergedVars["JOB_GIT_PASSWORD"] == "" {
						mergedVars["JOB_GIT_PASSWORD"] = gitCert.Password
					}
				}
			}
		}
	}

	finalTitle := strings.TrimSpace(title)
	if finalTitle == "" {
		finalTitle = fmt.Sprintf("job %s - %s", job.Name, author)
	}
	finalMessage := strings.TrimSpace(message)
	if finalMessage == "" {
		finalMessage = fmt.Sprintf("Job（%s）触发", author)
	}

	return s.pipelineSvc.BuildAndEnqueueRun(ctx, pipelinesvc.BuildAndEnqueueInput{
		OwnerKind:  model.PipelineOwnerJob,
		JobID:      job.ID,
		Event:      event,
		Author:     author,
		Message:    finalMessage,
		Title:      finalTitle,
		Branch:     branch,
		Commit:     strings.TrimSpace(opts.Commit),
		Variables:  mergedVars,
		YAML:       rendered,
		RepoURL:    repoURL,
		RepoClone:  repoClone,
		RepoBranch: repoBranch,
		ExtraLabels: map[string]string{
			"job":    job.Name,
			"job_id": fmt.Sprintf("%d", job.ID),
			"owner":  string(model.PipelineOwnerJob),
		},
	})
}

// cleanCronSchedules 去掉空白与重复, 保持顺序; 返回 nil 时调度器会清空所有
// 已注册 entries.
func cleanCronSchedules(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
