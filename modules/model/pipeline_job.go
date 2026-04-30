package model

// PipelineJob 是独立于项目 (Repo) 的 pipeline 定义, 直接存完整 YAML 并支持
// 手动触发. 与 PipelineTemplate 的草稿/发布两态不同, Job 编辑即生效, 因为
// 没有引用方需要 staging.
//
// 当 GitEnabled=true 且 GitCloneURL 非空时, 触发器会把 git 信息塞进 task
// payload 的 RepoURL/RepoClone 字段, 让现有 handleTask 的 clone 逻辑接管;
// GitEnabled=false 时执行器仅准备空 workspace, commands 自行处理 (例如
// 直接拉镜像跑命令, 或在 commands 里手动 git clone).
//
// GitCredentialID 选填关联 certificates.id, 提供给执行器在 clone 时设置
// HTTP 基础认证. credentialdef 解析逻辑复用 system service 的 cert 解密.
type PipelineJob struct {
	ID              int64             `json:"id"                gorm:"column:id;primaryKey;autoIncrement"`
	Name            string            `json:"name"              gorm:"column:name;size:128;uniqueIndex:uq_pipeline_job_name;not null"`
	DisplayName     string            `json:"display_name"      gorm:"column:display_name;size:255"`
	Description     string            `json:"description"       gorm:"column:description;type:text"`
	Content         string            `json:"content"           gorm:"column:content;type:longtext"`
	GitEnabled      bool              `json:"git_enabled"       gorm:"column:git_enabled;default:false"`
	GitCloneURL     string            `json:"git_clone_url"     gorm:"column:git_clone_url;size:1000"`
	GitBranch       string            `json:"git_branch"        gorm:"column:git_branch;size:200"`
	GitCredentialID *int64            `json:"git_credential_id" gorm:"column:git_credential_id;index"`
	Variables       map[string]string `json:"variables"         gorm:"column:variables;serializer:json"`
	// CronSchedules 是标准 5 字段 cron 表达式列表 (minute hour day month weekday).
	// 多条并存; 删空即取消所有调度. 写库后由 service 层调
	// pipelineSvc.RefreshJobCronEntries 同步到内存 scheduler.
	CronSchedules []string `json:"cron_schedules" gorm:"column:cron_schedules;serializer:json"`
	CreatedBy     string   `json:"created_by"     gorm:"column:created_by;size:128"`
	UpdatedBy     string   `json:"updated_by"     gorm:"column:updated_by;size:128"`
	Created       int64    `json:"created"        gorm:"column:created"`
	Updated       int64    `json:"updated"        gorm:"column:updated"`
}

func (PipelineJob) TableName() string {
	return "pipeline_jobs"
}
