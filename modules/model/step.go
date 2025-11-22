package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

const (
	FailureIgnore = "ignore"
	FailureFail   = "fail"
)

type Step struct {
	ID         int64         `json:"id"                 gorm:"column:id;primaryKey;autoIncrement"`
	UUID       string        `json:"uuid"               gorm:"column:uuid;index"`
	PipelineID int64         `json:"pipeline_id"        gorm:"column:pipeline_id;index;uniqueIndex:uq_step_pipeline_pid"`
	PID        int           `json:"pid"                gorm:"column:pid;uniqueIndex:uq_step_pipeline_pid"`
	PPID       int           `json:"ppid"               gorm:"column:ppid"`
	Name       string        `json:"name"               gorm:"column:name"`
	State      StatusValue   `json:"state"              gorm:"column:state"`
	Error      string        `json:"error,omitempty"    gorm:"column:error;type:text"`
	Failure    string        `json:"-"                  gorm:"column:failure"`
	ExitCode   int           `json:"exit_code"          gorm:"column:exit_code"`
	Started    int64         `json:"started,omitempty"  gorm:"column:started"`
	Finished   int64         `json:"finished,omitempty" gorm:"column:finished"`
	Type       StepType      `json:"type,omitempty"     gorm:"column:type"`
	Approval   *StepApproval `json:"approval,omitempty" gorm:"column:approval;serializer:json"`
}

func (Step) TableName() string {
	return "steps"
}

func (p *Step) Running() bool {
	return p.State == StatusPending || p.State == StatusRunning
}

func (p *Step) Failing() bool {
	return p.Failure == FailureFail && (p.State == StatusError || p.State == StatusKilled || p.State == StatusFailure)
}

type StepType string

const (
	StepTypeClone    StepType = "clone"
	StepTypeService  StepType = "service"
	StepTypePlugin   StepType = "plugin"
	StepTypeCommands StepType = "commands"
	StepTypeCache    StepType = "cache"
	StepTypeApproval StepType = "approval"
)

type StepApprovalStrategy string

const (
	StepApprovalStrategyAny StepApprovalStrategy = "any"
	StepApprovalStrategyAll StepApprovalStrategy = "all"
)

type StepApprovalState string

const (
	StepApprovalStatePending  StepApprovalState = "pending"
	StepApprovalStateApproved StepApprovalState = "approved"
	StepApprovalStateRejected StepApprovalState = "rejected"
	StepApprovalStateExpired  StepApprovalState = "expired"
)

type StepApprovalDecision struct {
	User      string `json:"user"`
	Action    string `json:"action"`
	Comment   string `json:"comment"`
	Timestamp int64  `json:"timestamp"`
}

type StepApproval struct {
	Message          string                 `json:"message"`
	Approvers        []string               `json:"approvers"`
	Strategy         StepApprovalStrategy   `json:"strategy"`
	Timeout          int64                  `json:"timeout"`
	RequestedBy      string                 `json:"requested_by"`
	RequestedAt      int64                  `json:"requested_at"`
	ExpiresAt        int64                  `json:"expires_at"`
	State            StepApprovalState      `json:"state"`
	Decisions        []StepApprovalDecision `json:"decisions"`
	FinalizedBy      string                 `json:"finalized_by"`
	FinalizedAt      int64                  `json:"finalized_at"`
	CanApprove       bool                   `json:"can_approve" gorm:"-"`
	CanReject        bool                   `json:"can_reject" gorm:"-"`
	PendingApprovers []string               `json:"pending_approvers,omitempty" gorm:"-"`
}

// Value implements driver.Valuer to persist the approval definition as JSON.
func (s *StepApproval) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// Scan implements sql.Scanner to restore the approval definition from JSON.
func (s *StepApproval) Scan(value any) error {
	if s == nil {
		return fmt.Errorf("StepApproval: Scan on nil pointer")
	}
	if value == nil {
		*s = StepApproval{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("StepApproval: unsupported source %T", value)
	}

	return json.Unmarshal(bytes, s)
}
