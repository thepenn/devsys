package model

type Workflow struct {
	ID         int64             `json:"id"                 gorm:"column:id;primaryKey;autoIncrement"`
	PipelineID int64             `json:"pipeline_id"        gorm:"column:pipeline_id;index;uniqueIndex:uq_workflow_pipeline_pid"`
	PID        int               `json:"pid"                gorm:"column:pid;uniqueIndex:uq_workflow_pipeline_pid"`
	Name       string            `json:"name"               gorm:"column:name"`
	State      StatusValue       `json:"state"              gorm:"column:state"`
	Error      string            `json:"error,omitempty"    gorm:"column:error;type:text"`
	Started    int64             `json:"started,omitempty"  gorm:"column:started"`
	Finished   int64             `json:"finished,omitempty" gorm:"column:finished"`
	AgentID    int64             `json:"agent_id,omitempty" gorm:"column:agent_id"`
	Platform   string            `json:"platform,omitempty" gorm:"column:platform"`
	Environ    map[string]string `json:"environ,omitempty"  gorm:"column:environ;serializer:json"`
	AxisID     int               `json:"-"                  gorm:"column:axis_id"`
	Children   []*Step           `json:"children,omitempty" gorm:"-"`
}

func (Workflow) TableName() string {
	return "workflows"
}

func (p *Workflow) Running() bool {
	return p.State == StatusPending || p.State == StatusRunning
}

func (p *Workflow) Failing() bool {
	return p.State == StatusError || p.State == StatusKilled || p.State == StatusFailure
}

func IsThereRunningStage(workflows []*Workflow) bool {
	for _, p := range workflows {
		if p.Running() {
			return true
		}
	}
	return false
}

func PipelineStatus(workflows []*Workflow) StatusValue {
	status := StatusSuccess

	for _, p := range workflows {
		if p.Failing() {
			status = p.State
		}
	}

	return status
}

func WorkflowStatus(steps []*Step) StatusValue {
	status := StatusSuccess

	for _, p := range steps {
		if p.Failing() {
			status = p.State
			break
		}
	}

	return status
}
