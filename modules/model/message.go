package model

// Message types and sources are stable string constants so DB rows produced
// by old code keep rendering correctly after future enum extensions.
const (
	MessageTypeInfo  = "info"
	MessageTypeWarn  = "warn"
	MessageTypeError = "error"

	MessageSourceSystem   = "system"
	MessageSourcePipeline = "pipeline"
	MessageSourceAlert    = "alert"
	MessageSourceRBAC     = "rbac"
)

// Message is an in-app notification delivered to a single user. Other services
// emit messages via service/message.Service#Create. The user reads them at
// GET /api/v1/messages and marks them as read individually or in bulk.
//
// Created/ReadAt 都是 unix 秒, 与库内其它表 (Pipeline/Step/...) 保持风格一致.
type Message struct {
	ID      int64  `json:"id"       gorm:"column:id;primaryKey;autoIncrement"`
	UserID  int64  `json:"user_id"  gorm:"column:user_id;index:idx_messages_user_created,priority:1"`
	Type    string `json:"type"     gorm:"column:type;size:16"`
	Source  string `json:"source"   gorm:"column:source;size:32"`
	Title   string `json:"title"    gorm:"column:title;size:255"`
	Content string `json:"content"  gorm:"column:content;type:text"`
	// ReadAt = 0 表示未读; > 0 是 unix 秒时间戳.
	ReadAt  int64 `json:"read_at"  gorm:"column:read_at;index:idx_messages_read"`
	Created int64 `json:"created"  gorm:"column:created;index:idx_messages_user_created,priority:2"`
}

func (Message) TableName() string {
	return "messages"
}
