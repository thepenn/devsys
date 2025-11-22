package model

import (
	"fmt"
	"slices"
	"strings"
)

type Task struct {
	ID           string                 `json:"id"           gorm:"column:id;primaryKey"`
	PID          int                    `json:"pid"          gorm:"column:pid"`
	Name         string                 `json:"name"         gorm:"column:name"`
	Data         []byte                 `json:"-"            gorm:"column:data;type:longblob"`
	Labels       map[string]string      `json:"labels"       gorm:"column:labels;serializer:json"`
	Dependencies []string               `json:"dependencies" gorm:"column:dependencies;serializer:json"`
	RunOn        []string               `json:"run_on"       gorm:"column:run_on;serializer:json"`
	DepStatus    map[string]StatusValue `json:"dep_status"   gorm:"column:dependencies_status;serializer:json"`
	AgentID      int64                  `json:"agent_id"     gorm:"column:agent_id"`
	PipelineID   int64                  `json:"pipeline_id"  gorm:"column:pipeline_id"`
	RepoID       int64                  `json:"repo_id"      gorm:"column:repo_id"`
}

func (Task) TableName() string {
	return "tasks"
}

const (
	taskLabelRepo = "repo"
	taskLabelOrg  = "org-id"
)

func (t *Task) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s (%s) - %s", t.ID, t.Dependencies, t.DepStatus))
	return sb.String()
}

func (t *Task) ApplyLabelsFromRepo(r *Repo) error {
	if r == nil {
		return fmt.Errorf("repo is nil but needed to get task labels")
	}
	if t.Labels == nil {
		t.Labels = make(map[string]string)
	}
	t.Labels[taskLabelRepo] = r.FullName
	t.Labels[taskLabelOrg] = fmt.Sprintf("%d", r.OrgID)
	return nil
}

func (t *Task) ShouldRun() bool {
	if t.runsOnFailure() && t.runsOnSuccess() {
		return true
	}

	if !t.runsOnFailure() && t.runsOnSuccess() {
		for _, status := range t.DepStatus {
			if status != StatusSuccess {
				return false
			}
		}
		return true
	}

	if t.runsOnFailure() && !t.runsOnSuccess() {
		for _, status := range t.DepStatus {
			if status == StatusSuccess {
				return false
			}
		}
		return true
	}

	return false
}

func (t *Task) runsOnFailure() bool {
	return slices.Contains(t.RunOn, string(StatusFailure))
}

func (t *Task) runsOnSuccess() bool {
	if len(t.RunOn) == 0 {
		return true
	}

	return slices.Contains(t.RunOn, string(StatusSuccess))
}
