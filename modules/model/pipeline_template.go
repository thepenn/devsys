package model

import "time"

// PipelineTemplate.Kind 区分模板用途:
//   - PipelineTemplateKindPipeline (默认): 完整 pipeline YAML, 项目通过
//     RepoPipelineConfig.Source = template + TemplateID 整体引用.
//   - PipelineTemplateKindStep: step 片段 (内容必须以 steps: 开头), 项目
//     通过 RepoPipelineConfig.Source = compose + ComposeSteps 按顺序组装多
//     个片段.
const (
	PipelineTemplateKindPipeline = "pipeline"
	PipelineTemplateKindStep     = "step"
)

// PipelineTemplate 通用流水线模板. 草稿/发布双态:
//   - DraftContent 是编辑中的最新 YAML, 始终允许覆盖.
//   - PublishedContent 是给项目引用方真正使用的 YAML; 仅在
//     管理员显式 Publish 后从 DraftContent 拷贝过来.
//
// Name 为引用方唯一可读标识 (项目下拉选模板时显示).
// 关联到 RepoPipelineConfig.TemplateID 或 ComposeSteps[].StepTemplateID;
// 删除模板前 service 层会同时校验 template / compose 两处引用.
type PipelineTemplate struct {
	ID               int64      `json:"id"                gorm:"column:id;primaryKey;autoIncrement"`
	Name             string     `json:"name"              gorm:"column:name;size:128;not null;uniqueIndex:uq_pipeline_template_name"`
	DisplayName      string     `json:"display_name"      gorm:"column:display_name;size:255;not null"`
	Description      string     `json:"description"       gorm:"column:description;type:text"`
	Kind             string     `json:"kind"              gorm:"column:kind;size:16;default:pipeline;not null"`
	DraftContent     string     `json:"draft_content"     gorm:"column:draft_content;type:longtext"`
	PublishedContent string     `json:"published_content" gorm:"column:published_content;type:longtext"`
	PublishedAt      *time.Time `json:"published_at"      gorm:"column:published_at"`
	PublishedBy      string     `json:"published_by"      gorm:"column:published_by;size:128"`
	CreatedBy        string     `json:"created_by"        gorm:"column:created_by;size:128"`
	UpdatedBy        string     `json:"updated_by"        gorm:"column:updated_by;size:128"`
	Created          int64      `json:"created"           gorm:"column:created"`
	Updated          int64      `json:"updated"           gorm:"column:updated"`
}

// EffectiveKind 在 Kind 为空时回落到 pipeline (兼容老数据).
func (t *PipelineTemplate) EffectiveKind() string {
	if t == nil || t.Kind == "" {
		return PipelineTemplateKindPipeline
	}
	return t.Kind
}

func (PipelineTemplate) TableName() string {
	return "pipeline_templates"
}

// IsPublished 表示模板是否曾经发布过, 项目引用方仅在该字段为 true 时
// 才能正常触发流水线.
func (t *PipelineTemplate) IsPublished() bool {
	if t == nil {
		return false
	}
	return t.PublishedAt != nil && t.PublishedContent != ""
}
