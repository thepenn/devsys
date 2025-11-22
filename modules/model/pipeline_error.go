package model

import "fmt"

type PipelineErrorType string

const (
	PipelineErrorTypeLinter      PipelineErrorType = "linter"
	PipelineErrorTypeDeprecation PipelineErrorType = "deprecation"
	PipelineErrorTypeCompiler    PipelineErrorType = "compiler"
	PipelineErrorTypeGeneric     PipelineErrorType = "generic"
	PipelineErrorTypeBadHabit    PipelineErrorType = "bad_habit"
)

type PipelineError struct {
	Type      PipelineErrorType `json:"type"`
	Message   string            `json:"message"`
	IsWarning bool              `json:"is_warning"`
	Data      any               `json:"data"`
}

func (e *PipelineError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}
