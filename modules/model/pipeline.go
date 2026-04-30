package model

// PipelineOwnerKind 区分 Pipeline 行属于 repo 触发还是独立 Job 触发.
//   - PipelineOwnerRepo (默认): RepoID > 0, JobID = 0, 走原有 git clone 流程.
//   - PipelineOwnerJob: RepoID = 0, JobID > 0, 由 PipelineJob.Trigger 创建,
//     执行器在 RepoClone 为空时跳过 clone 仅准备 workspace.
const (
	PipelineOwnerRepo = "repo"
	PipelineOwnerJob  = "job"
)

type Pipeline struct {
	ID                   int64             `json:"id"                      gorm:"column:id;primaryKey;autoIncrement"`
	// 旧 uniqueIndex (repo_id, number) 改为 (repo_id, job_id, number) 以容纳
	// repo_id=0 的 Job 触发场景; CreatePipeline 的 MAX(number) 也会按 owner
	// 范围分别统计.
	RepoID               int64             `json:"-"                       gorm:"column:repo_id;index;uniqueIndex:uq_pipeline_owner_number"`
	JobID                int64             `json:"job_id,omitempty"        gorm:"column:job_id;index;uniqueIndex:uq_pipeline_owner_number;not null;default:0"`
	OwnerKind            string            `json:"owner_kind,omitempty"    gorm:"column:owner_kind;size:16;not null;default:repo"`
	Number               int64             `json:"number"                  gorm:"column:number;uniqueIndex:uq_pipeline_owner_number"`
	Author               string            `json:"author"                  gorm:"column:author;index"`
	Parent               int64             `json:"parent"                  gorm:"column:parent"`
	Event                WebhookEvent      `json:"event"                   gorm:"column:event"`
	EventReason          []string          `json:"event_reason"            gorm:"column:event_reason;serializer:json"`
	Status               StatusValue       `json:"status"                  gorm:"column:status;index"`
	Errors               []*PipelineError  `json:"errors"                  gorm:"column:errors;serializer:json"`
	Created              int64             `json:"created"                 gorm:"column:created;not null;default:0"`
	Updated              int64             `json:"updated"                 gorm:"column:updated;not null;default:0"`
	Started              int64             `json:"started"                 gorm:"column:started"`
	Finished             int64             `json:"finished"                gorm:"column:finished"`
	DeployTo             string            `json:"deploy_to"               gorm:"column:deploy"`
	DeployTask           string            `json:"deploy_task"             gorm:"column:deploy_task"`
	Commit               string            `json:"commit"                  gorm:"column:commit"`
	Branch               string            `json:"branch"                  gorm:"column:branch"`
	Ref                  string            `json:"ref"                     gorm:"column:ref"`
	Refspec              string            `json:"refspec"                 gorm:"column:refspec"`
	Title                string            `json:"title"                   gorm:"column:title"`
	Message              string            `json:"message"                 gorm:"column:message;type:text"`
	Timestamp            int64             `json:"timestamp"               gorm:"column:timestamp"`
	Sender               string            `json:"sender"                  gorm:"column:sender"`
	Avatar               string            `json:"author_avatar"           gorm:"column:avatar;size:500"`
	Email                string            `json:"author_email"            gorm:"column:email;size:500"`
	ForgeURL             string            `json:"forge_url"               gorm:"column:forge_url"`
	Reviewer             string            `json:"reviewed_by"             gorm:"column:reviewer"`
	Reviewed             int64             `json:"reviewed"                gorm:"column:reviewed"`
	Workflows            []*Workflow       `json:"workflows,omitempty"     gorm:"-"`
	ChangedFiles         []string          `json:"changed_files,omitempty" gorm:"column:changed_files;serializer:json"`
	AdditionalVariables  map[string]string `json:"variables,omitempty"     gorm:"column:additional_variables;serializer:json"`
	PullRequestLabels    []string          `json:"pr_labels,omitempty"     gorm:"column:pr_labels;serializer:json"`
	PullRequestMilestone string            `json:"pr_milestone,omitempty"  gorm:"column:pr_milestone"`
	IsPrerelease         bool              `json:"is_prerelease,omitempty" gorm:"column:is_prerelease"`
	FromFork             bool              `json:"from_fork,omitempty"     gorm:"column:from_fork"`
}

func (Pipeline) TableName() string {
	return "pipelines"
}

type PipelineFilter struct {
	Before      int64
	After       int64
	Branch      string
	Events      []WebhookEvent
	RefContains string
	Status      StatusValue
}

func (p Pipeline) IsMultiPipeline() bool {
	return len(p.Workflows) > 1
}

func (p Pipeline) IsPullRequest() bool {
	return p.Event == EventPull || p.Event == EventPullClosed || p.Event == EventPullMetadata
}

type PipelineOptions struct {
	Branch    string            `json:"branch"`
	Variables map[string]string `json:"variables"`
	Commit    string            `json:"commit"`
}

// EffectiveOwnerKind 在 OwnerKind 为空时回落到 repo, 兼容历史数据.
func (p *Pipeline) EffectiveOwnerKind() string {
	if p == nil || p.OwnerKind == "" {
		return PipelineOwnerRepo
	}
	return p.OwnerKind
}
