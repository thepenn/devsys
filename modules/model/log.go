package model

type LogEntryType int

const (
	LogEntryStdout LogEntryType = iota
	LogEntryStderr
	LogEntryExitCode
	LogEntryMetadata
	LogEntryProgress
)

type LogEntry struct {
	ID      int64        `json:"id"      gorm:"column:id;primaryKey;autoIncrement"`
	StepID  int64        `json:"step_id" gorm:"column:step_id;index"`
	Time    int64        `json:"time"    gorm:"column:time"`
	Line    int          `json:"line"    gorm:"column:line"`
	Data    []byte       `json:"data"    gorm:"column:data;type:longblob"`
	Created int64        `json:"-"       gorm:"column:created"`
	Type    LogEntryType `json:"type"    gorm:"column:type"`
}

func (LogEntry) TableName() string {
	return "log_entries"
}
