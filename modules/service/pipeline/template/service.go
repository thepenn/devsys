// Package template 管理通用 PipelineTemplate (草稿/发布两态).
//
// 数据流:
//
//	UI 编辑 -> UpdateDraft (yaml 解析校验) -> draft_content
//	UI 发布 -> Publish (要求 draft 不空且能解析) -> published_content
//	项目触发 -> Resolve (取 published_content + ${VAR} 替换) -> spec.Parse
//
// 删除模板前会拒绝仍被任何 RepoPipelineConfig.template_id 引用的模板,
// 调用方应先把这些项目切回 inline 或换其它模板.
package template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
	"github.com/thepenn/devsys/service/pipeline/spec"
	systemsvc "github.com/thepenn/devsys/service/system"
)

var (
	// ErrTemplateNotFound 模板不存在.
	ErrTemplateNotFound = errors.New("pipeline template not found")
	// ErrTemplateNameConflict 模板名重复.
	ErrTemplateNameConflict = errors.New("pipeline template name already exists")
	// ErrTemplateInUse 模板仍被项目引用, 拒绝删除.
	ErrTemplateInUse = errors.New("pipeline template is still in use")
	// ErrTemplateNotPublished 模板尚未发布, 项目触发时使用.
	ErrTemplateNotPublished = errors.New("pipeline template has no published version")
	// ErrTemplateDraftEmpty 草稿为空时不允许发布.
	ErrTemplateDraftEmpty = errors.New("pipeline template draft is empty")
	// ErrTemplateKindMismatch 模板 kind 与调用方期望不一致 (如 compose 引用了
	// kind=pipeline 的模板).
	ErrTemplateKindMismatch = errors.New("pipeline template kind mismatch")
	// ErrComposeEmpty compose 模式下 ComposeSteps 为空.
	ErrComposeEmpty = errors.New("compose steps list is empty")
)

// Service 暴露通用 pipeline 模板 CRUD + 发布 + 渲染.
//
// systemSvc 用于在 ${VAR} 渲染时按变量名 fallback 到 Certificate 仓库 (git
// → password, docker → repo URL); 可空, 此时退化为旧行为 (无凭证回填).
type Service struct {
	db        *store.DB
	systemSvc *systemsvc.Service
}

// New 构造一个 Service. systemSvc 启用凭证 fallback 渲染.
func New(db *store.DB, systemSvc *systemsvc.Service) *Service {
	return &Service{db: db, systemSvc: systemSvc}
}

// CreateInput 是 Create 的入参.
type CreateInput struct {
	Name         string
	DisplayName  string
	Description  string
	Kind         string // pipeline (默认) | step
	DraftContent string
	Actor        string
}

// UpdateInput 是 UpdateDraft 的入参.
type UpdateInput struct {
	DisplayName  *string
	Description  *string
	DraftContent *string
	Actor        string
}

// ListOptions 列表查询参数.
type ListOptions struct {
	Page          int
	PerPage       int
	OnlyPublished bool
	Kind          string // 空 = 全部; pipeline / step
	Keyword       string
}

// ListResult 分页结果.
type ListResult struct {
	Items   []TemplateSummary `json:"items"`
	Total   int64             `json:"total"`
	Page    int               `json:"page"`
	PerPage int               `json:"per_page"`
}

// TemplateSummary 列表里返回的精简结构 (不含 YAML 内容, 减少传输量).
type TemplateSummary struct {
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

// validateContent 按模板 kind 选择校验函数:
//   - kind=pipeline: spec.Parse (要求顶层 mapping + steps:)
//   - kind=step: spec.ParseStepFragment (放宽: 允许单 step 序列 / 单 step mapping)
//
// content 为空时跳过校验 (允许保存空草稿后再补内容).
func (s *Service) validateContent(kind, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	if kind == model.PipelineTemplateKindStep {
		_, err := spec.ParseStepFragment(content)
		return err
	}
	_, err := spec.Parse(content)
	return err
}

// Create 写入一条新模板. Name 必须唯一; DraftContent 非空时会做 YAML 校验,
// 按 kind 分发到 spec.Parse 或 spec.ParseStepFragment.
func (s *Service) Create(ctx context.Context, in CreateInput) (*model.PipelineTemplate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("template service not initialised")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("template name is required")
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = name
	}
	kind := strings.TrimSpace(in.Kind)
	switch kind {
	case "":
		kind = model.PipelineTemplateKindPipeline
	case model.PipelineTemplateKindPipeline, model.PipelineTemplateKindStep:
		// ok
	default:
		return nil, fmt.Errorf("invalid template kind: %s", kind)
	}
	if err := s.validateContent(kind, in.DraftContent); err != nil {
		return nil, fmt.Errorf("草稿 YAML 校验失败: %w", err)
	}
	now := time.Now().Unix()
	tpl := &model.PipelineTemplate{
		Name:         name,
		DisplayName:  displayName,
		Description:  strings.TrimSpace(in.Description),
		Kind:         kind,
		DraftContent: in.DraftContent,
		CreatedBy:    in.Actor,
		UpdatedBy:    in.Actor,
		Created:      now,
		Updated:      now,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.PipelineTemplate
		err := tx.WithContext(ctx).Where("name = ?", name).Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.WithContext(ctx).Create(tpl).Error
		case err != nil:
			return err
		default:
			return ErrTemplateNameConflict
		}
	})
	if err != nil {
		return nil, err
	}
	return tpl, nil
}

// Get 按主键拉取模板. 不存在返回 ErrTemplateNotFound.
func (s *Service) Get(ctx context.Context, id int64) (*model.PipelineTemplate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("template service not initialised")
	}
	if id <= 0 {
		return nil, ErrTemplateNotFound
	}
	var tpl model.PipelineTemplate
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).First(&tpl, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

// List 返回分页模板列表 + 每个模板的引用次数.
func (s *Service) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("template service not initialised")
	}
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
		items []model.PipelineTemplate
		total int64
	)
	err := s.db.View(func(tx *gorm.DB) error {
		ctxTx := tx.WithContext(ctx)
		base := ctxTx.Model(&model.PipelineTemplate{})
		if opts.OnlyPublished {
			base = base.Where("published_at IS NOT NULL AND published_content <> ''")
		}
		if kind := strings.TrimSpace(opts.Kind); kind != "" {
			// 兼容老数据 kind 为空时的默认 pipeline 语义.
			if kind == model.PipelineTemplateKindPipeline {
				base = base.Where("kind = ? OR kind IS NULL OR kind = ''", kind)
			} else {
				base = base.Where("kind = ?", kind)
			}
		}
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
		return &ListResult{Items: []TemplateSummary{}, Total: total, Page: opts.Page, PerPage: opts.PerPage}, nil
	}
	ids := make([]int64, 0, len(items))
	for _, t := range items {
		ids = append(ids, t.ID)
	}
	refs, err := s.countReferences(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateSummary, 0, len(items))
	for _, t := range items {
		out = append(out, summaryOf(t, refs[t.ID]))
	}
	return &ListResult{Items: out, Total: total, Page: opts.Page, PerPage: opts.PerPage}, nil
}

// UpdateDraft 修改草稿和元数据. 如果 DraftContent 提供且非空, 按模板 kind
// 做 YAML 校验 (kind=step 走 ParseStepFragment 放宽规则).
func (s *Service) UpdateDraft(ctx context.Context, id int64, in UpdateInput) (*model.PipelineTemplate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("template service not initialised")
	}
	var updated *model.PipelineTemplate
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var tpl model.PipelineTemplate
		if err := tx.WithContext(ctx).First(&tpl, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTemplateNotFound
			}
			return err
		}
		// 校验放在事务内, 用模板自己的 kind 决定走哪个 parser; 失败时
		// 整个事务回滚, draft 不会被部分写入.
		if in.DraftContent != nil {
			if err := s.validateContent(tpl.EffectiveKind(), *in.DraftContent); err != nil {
				return fmt.Errorf("草稿 YAML 校验失败: %w", err)
			}
		}
		if in.DisplayName != nil {
			tpl.DisplayName = strings.TrimSpace(*in.DisplayName)
			if tpl.DisplayName == "" {
				tpl.DisplayName = tpl.Name
			}
		}
		if in.Description != nil {
			tpl.Description = strings.TrimSpace(*in.Description)
		}
		if in.DraftContent != nil {
			tpl.DraftContent = *in.DraftContent
		}
		tpl.UpdatedBy = in.Actor
		tpl.Updated = time.Now().Unix()
		if err := tx.WithContext(ctx).Save(&tpl).Error; err != nil {
			return err
		}
		updated = &tpl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Publish 把当前 DraftContent 同步成 PublishedContent. 草稿空 / 解析失败时拒绝.
func (s *Service) Publish(ctx context.Context, id int64, actor string) (*model.PipelineTemplate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("template service not initialised")
	}
	var published *model.PipelineTemplate
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var tpl model.PipelineTemplate
		if err := tx.WithContext(ctx).First(&tpl, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTemplateNotFound
			}
			return err
		}
		draft := strings.TrimSpace(tpl.DraftContent)
		if draft == "" {
			return ErrTemplateDraftEmpty
		}
		if err := s.validateContent(tpl.EffectiveKind(), tpl.DraftContent); err != nil {
			return fmt.Errorf("草稿 YAML 校验失败: %w", err)
		}
		now := time.Now()
		tpl.PublishedContent = tpl.DraftContent
		tpl.PublishedAt = &now
		tpl.PublishedBy = actor
		tpl.UpdatedBy = actor
		tpl.Updated = now.Unix()
		if err := tx.WithContext(ctx).Save(&tpl).Error; err != nil {
			return err
		}
		published = &tpl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return published, nil
}

// Delete 删除模板; 仍被任何 RepoPipelineConfig 引用时返回 ErrTemplateInUse.
// 同时检查 source=template (TemplateID 直引) 与 source=compose (ComposeSteps 中含本 id).
func (s *Service) Delete(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return errors.New("template service not initialised")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var tpl model.PipelineTemplate
		if err := tx.WithContext(ctx).First(&tpl, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTemplateNotFound
			}
			return err
		}
		var inUse int64
		if err := tx.WithContext(ctx).
			Model(&model.RepoPipelineConfig{}).
			Where("template_id = ? AND source = ?", id, model.PipelineConfigSourceTemplate).
			Count(&inUse).Error; err != nil {
			return err
		}
		if inUse > 0 {
			return ErrTemplateInUse
		}
		// compose 引用: ComposeSteps 是 JSON 列, 无法直接 SQL 过滤, 全表扫.
		var composeCfgs []model.RepoPipelineConfig
		if err := tx.WithContext(ctx).
			Where("source = ?", model.PipelineConfigSourceCompose).
			Find(&composeCfgs).Error; err != nil {
			return err
		}
		for _, cfg := range composeCfgs {
			for _, ref := range cfg.ComposeSteps {
				if ref.StepTemplateID == id {
					return ErrTemplateInUse
				}
			}
		}
		return tx.WithContext(ctx).Delete(&model.PipelineTemplate{}, id).Error
	})
}

// ReferencingRepo 单条记录, 描述某个项目使用了模板 (用于删除前提示).
type ReferencingRepo struct {
	RepoID   int64  `json:"repo_id"`
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Owner    string `json:"owner"`
}

// ListReferencingRepos 返回引用某模板的项目列表 (template + compose 两种引用合并).
func (s *Service) ListReferencingRepos(ctx context.Context, id int64) ([]ReferencingRepo, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("template service not initialised")
	}
	var rows []ReferencingRepo
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Table("repo_pipeline_configs AS c").
			Select("r.id AS repo_id, r.full_name, r.name, r.owner").
			Joins("INNER JOIN repos r ON r.id = c.repo_id").
			Where("c.template_id = ? AND c.source = ?", id, model.PipelineConfigSourceTemplate).
			Order("r.full_name ASC").
			Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	// compose 引用: 拉所有 source=compose 的 cfg 在内存里过滤含本 id 的, 再拼接.
	type composeRow struct {
		RepoID   int64  `gorm:"column:repo_id"`
		FullName string `gorm:"column:full_name"`
		Name     string `gorm:"column:name"`
		Owner    string `gorm:"column:owner"`
		Steps    []model.ComposeStepRef `gorm:"column:compose_steps;serializer:json"`
	}
	var composeRows []composeRow
	err = s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Table("repo_pipeline_configs AS c").
			Select("r.id AS repo_id, r.full_name, r.name, r.owner, c.compose_steps").
			Joins("INNER JOIN repos r ON r.id = c.repo_id").
			Where("c.source = ?", model.PipelineConfigSourceCompose).
			Order("r.full_name ASC").
			Scan(&composeRows).Error
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{}, len(rows))
	for _, r := range rows {
		seen[r.RepoID] = struct{}{}
	}
	for _, cr := range composeRows {
		hit := false
		for _, ref := range cr.Steps {
			if ref.StepTemplateID == id {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if _, dup := seen[cr.RepoID]; dup {
			continue
		}
		seen[cr.RepoID] = struct{}{}
		rows = append(rows, ReferencingRepo{
			RepoID:   cr.RepoID,
			FullName: cr.FullName,
			Name:     cr.Name,
			Owner:    cr.Owner,
		})
	}
	if rows == nil {
		rows = []ReferencingRepo{}
	}
	return rows, nil
}

// Resolve 拉取已发布的 YAML 并按 vars 做 ${VAR} 替换. 返回最终待 spec.Parse 的字符串.
// 若 vars 缺失某变量, 会尝试按变量名匹配 Certificate 仓库做回填 (NewCertificateFallback).
func (s *Service) Resolve(ctx context.Context, id int64, vars map[string]string) (string, error) {
	tpl, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if !tpl.IsPublished() {
		return "", ErrTemplateNotPublished
	}
	rendered := RenderWithFallback(tpl.PublishedContent, vars, NewCertificateFallback(ctx, s.systemSvc))
	return rendered, nil
}

// RenderForPreview 一次返回 (rendered, missing). 共享同一个 fallback 实例,
// 让 Render 和 MissingPlaceholders 中的 cert 查询走同一份 cache, 避免双查 DB.
// 给 /pipeline-templates/{id}/render 路由用, 也方便其它需要"渲染+缺失检测"
// 双输出的场景.
func (s *Service) RenderForPreview(ctx context.Context, id int64, vars map[string]string) (string, []string, error) {
	tpl, err := s.Get(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if !tpl.IsPublished() {
		return "", nil, ErrTemplateNotPublished
	}
	fb := NewCertificateFallback(ctx, s.systemSvc)
	rendered := RenderWithFallback(tpl.PublishedContent, vars, fb)
	missing := MissingPlaceholdersWithFallback(tpl.PublishedContent, vars, fb)
	return rendered, missing, nil
}

// ResolveCompose 按 refs 顺序把多个 step 模板渲染并拼接成一个 PipelineSpec.
//
//   - 每个 ref 用 (globalVars ⊕ ref.Variables) 渲染对应模板的
//     PublishedContent (ref.Variables 优先).
//   - 解析后取片段里所有 StepSpec, push 到结果列表.
//   - 模板必须 kind=step + 已发布; kind 不匹配返回 ErrTemplateKindMismatch,
//     未发布返回 ErrTemplateNotPublished.
//   - Alias 非空时把片段内所有 step.Name 改为 alias (多 step 自动加 -N
//     后缀防冲突); 跨片段同名也会自动去重.
//
// 不再消费片段里的 name / workspace 等顶层字段; pipeline-level metadata 由
// 调用方在 BuildAndEnqueueRun 处用默认值组装.
func (s *Service) ResolveCompose(ctx context.Context, refs []model.ComposeStepRef, globalVars map[string]string) (*spec.PipelineSpec, error) {
	if len(refs) == 0 {
		return nil, ErrComposeEmpty
	}
	fb := NewCertificateFallback(ctx, s.systemSvc)
	out := &spec.PipelineSpec{
		Name:  "compose",
		Steps: make([]spec.StepSpec, 0, len(refs)),
	}
	usedNames := map[string]int{}
	for idx, ref := range refs {
		if ref.StepTemplateID <= 0 {
			return nil, fmt.Errorf("compose ref #%d: step_template_id missing", idx+1)
		}
		tpl, err := s.Get(ctx, ref.StepTemplateID)
		if err != nil {
			return nil, fmt.Errorf("compose ref #%d (template id=%d): %w", idx+1, ref.StepTemplateID, err)
		}
		if tpl.EffectiveKind() != model.PipelineTemplateKindStep {
			return nil, fmt.Errorf("compose ref #%d (%s): %w", idx+1, tpl.Name, ErrTemplateKindMismatch)
		}
		if !tpl.IsPublished() {
			return nil, fmt.Errorf("compose ref #%d (%s): %w", idx+1, tpl.Name, ErrTemplateNotPublished)
		}
		// 合并变量: globalVars 是基线, ref.Variables 覆盖之.
		merged := cloneStringMap(globalVars)
		for k, v := range ref.Variables {
			merged[k] = v
		}
		rendered := RenderWithFallback(tpl.PublishedContent, merged, fb)
		// 步骤模板 published_content 可能是单 step / 序列 / 完整 mapping,
		// 用 ParseStepFragment 统一接受所有形态.
		fragSteps, err := spec.ParseStepFragment(rendered)
		if err != nil {
			return nil, fmt.Errorf("compose ref #%d (%s): %w", idx+1, tpl.Name, err)
		}
		alias := strings.TrimSpace(ref.Alias)
		for stepIdx, step := range fragSteps {
			name := step.Name
			if alias != "" {
				if len(fragSteps) > 1 {
					name = fmt.Sprintf("%s-%d", alias, stepIdx+1)
				} else {
					name = alias
				}
			}
			step.Name = uniqueStepName(name, usedNames)
			out.Steps = append(out.Steps, step)
		}
	}
	if len(out.Steps) == 0 {
		return nil, fmt.Errorf("compose result has no steps")
	}
	return out, nil
}

// uniqueStepName 让所有 step 名称去重, 同名时追加 -2 / -3 后缀.
func uniqueStepName(base string, used map[string]int) string {
	if base == "" {
		base = "step"
	}
	if _, ok := used[base]; !ok {
		used[base] = 1
		return base
	}
	used[base]++
	return fmt.Sprintf("%s-%d", base, used[base])
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+4)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// countReferences 一次查出多个模板的引用数; 同时统计 source=template 与
// source=compose 两种引用 (compose 模式下需扫 JSON 列, 在内存里聚合).
func (s *Service) countReferences(ctx context.Context, ids []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		TemplateID int64
		Total      int64
	}
	var rows []row
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Model(&model.RepoPipelineConfig{}).
			Select("template_id, COUNT(*) AS total").
			Where("template_id IN ? AND source = ?", ids, model.PipelineConfigSourceTemplate).
			Group("template_id").
			Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.TemplateID] = r.Total
	}
	// 再统计 compose 引用: compose_steps 是 JSON 数组, 没法在 SQL 里按
	// step_template_id 直接 group, 拉出来在内存里聚合即可 (compose 配置
	// 数量级与 repo 数同级, 全表扫可接受).
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	var composeCfgs []model.RepoPipelineConfig
	err = s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("source = ?", model.PipelineConfigSourceCompose).
			Find(&composeCfgs).Error
	})
	if err != nil {
		return nil, err
	}
	for _, cfg := range composeCfgs {
		seen := map[int64]struct{}{}
		for _, ref := range cfg.ComposeSteps {
			if _, ok := idSet[ref.StepTemplateID]; !ok {
				continue
			}
			if _, dup := seen[ref.StepTemplateID]; dup {
				// 同一 cfg 多次引用同一 step 只算一次, 与 source=template
				// 的 1 行 = 1 引用语义对齐.
				continue
			}
			seen[ref.StepTemplateID] = struct{}{}
			out[ref.StepTemplateID]++
		}
	}
	return out, nil
}

func summaryOf(t model.PipelineTemplate, refs int64) TemplateSummary {
	return TemplateSummary{
		ID:           t.ID,
		Name:         t.Name,
		DisplayName:  t.DisplayName,
		Description:  t.Description,
		Kind:         t.EffectiveKind(),
		IsPublished:  t.IsPublished(),
		PublishedAt:  t.PublishedAt,
		PublishedBy:  t.PublishedBy,
		CreatedBy:    t.CreatedBy,
		UpdatedBy:    t.UpdatedBy,
		Created:      t.Created,
		Updated:      t.Updated,
		ReferencedBy: refs,
	}
}
