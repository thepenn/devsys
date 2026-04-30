package model

// PipelineConfigSource 标识 RepoPipelineConfig 当前使用的 YAML 来源.
//   - PipelineConfigSourceInline: 项目自定义 YAML, 存在 Content 字段.
//   - PipelineConfigSourceTemplate: 引用通用 PipelineTemplate (kind=pipeline),
//     触发时按 TemplateID 拉取 PublishedContent 并以 TemplateVariables 做
//     ${VAR} 替换. 该模式下 Content 字段保留历史快照, 不再用于执行.
//   - PipelineConfigSourceCompose: 按 ComposeSteps 顺序组装多个 step 模板
//     (kind=step) 拼成最终 pipeline. Content / TemplateID / TemplateVariables
//     在该模式下不使用 (TemplateID 清空, Content 保留切换前快照).
const (
	PipelineConfigSourceInline   = "inline"
	PipelineConfigSourceTemplate = "template"
	PipelineConfigSourceCompose  = "compose"
)

type PipelineCertificateBinding struct {
	CertificateID int64  `json:"certificate_id"`
	Alias         string `json:"alias"`
}

// ComposeStepRef 是项目装配模式 (Source=compose) 中对一个 step 模板的引用.
//
//   - StepTemplateID 必填, 指向 PipelineTemplate.ID 且对应模板 Kind=step.
//   - Alias 可选, 非空时把该片段里所有 step.Name 替换为 alias (多 step 时加
//     "-N" 后缀防冲突). 用于在多次引用同一模板时区分.
//   - Variables 可选, 仅作用于本片段的 ${VAR} 渲染, 优先级高于全局 vars.
type ComposeStepRef struct {
	StepTemplateID int64             `json:"step_template_id"`
	Alias          string            `json:"alias,omitempty"`
	Variables      map[string]string `json:"variables,omitempty"`
}

type RepoPipelineConfig struct {
	ID                int64             `json:"id"                 gorm:"column:id;primaryKey;autoIncrement"`
	RepoID            int64             `json:"repo_id"            gorm:"column:repo_id;uniqueIndex"`
	Content           string            `json:"content"            gorm:"column:content;type:longtext"`
	Source            string            `json:"source"             gorm:"column:source;size:16;default:inline;not null"`
	TemplateID        *int64            `json:"template_id"        gorm:"column:template_id;index"`
	TemplateVariables map[string]string `json:"template_variables" gorm:"column:template_variables;serializer:json"`
	ComposeSteps      []ComposeStepRef  `json:"compose_steps"      gorm:"column:compose_steps;serializer:json"`
	Dockerfile        string            `json:"dockerfile"         gorm:"column:dockerfile;type:longtext"`
	CleanupEnabled    bool              `json:"cleanup_enabled"    gorm:"column:cleanup_enabled"`
	RetentionDays     int               `json:"retention_days"     gorm:"column:retention_days"`
	MaxRecords        int               `json:"max_records"        gorm:"column:max_records"`
	DisallowParallel  bool              `json:"disallow_parallel"  gorm:"column:disallow_parallel"`
	CronSchedules     []string          `json:"cron_schedules"     gorm:"column:cron_schedules;serializer:json"`
	Created           int64             `json:"created"            gorm:"column:created"`
	Updated           int64             `json:"updated"            gorm:"column:updated"`

	// legacy columns retained for backward-compatibility with existing databases.
	LegacyVariables    map[string]string            `json:"-" gorm:"column:variables;serializer:json"`
	LegacyCertificates []PipelineCertificateBinding `json:"-" gorm:"column:certificates;serializer:json"`
	LegacyCronEnabled  bool                         `json:"-" gorm:"column:cron_enabled"`
	LegacyCronSpec     string                       `json:"-" gorm:"column:cron_spec;size:255"`
}

// EffectiveSource 在 Source 为空时回落到 inline, 防止旧数据被误判.
func (c *RepoPipelineConfig) EffectiveSource() string {
	if c == nil {
		return PipelineConfigSourceInline
	}
	switch c.Source {
	case PipelineConfigSourceTemplate:
		return PipelineConfigSourceTemplate
	case PipelineConfigSourceCompose:
		return PipelineConfigSourceCompose
	default:
		return PipelineConfigSourceInline
	}
}

func (RepoPipelineConfig) TableName() string {
	return "repo_pipeline_configs"
}
