package model

// AuditLog records a single mutating HTTP request that passed through the
// audit middleware. Read-only requests (GET) are intentionally excluded to
// keep the table size manageable.
//
// 写入路径: routers/middleware/audit -> service/audit (channel + worker, 批量).
// 读取路径: GET /api/v1/audit/logs (受 system:audit label 保护).
type AuditLog struct {
	ID       int64  `json:"id"        gorm:"column:id;primaryKey;autoIncrement"`
	UserID   int64  `json:"user_id"   gorm:"column:user_id;index:idx_audit_user_created,priority:1"`
	Login    string `json:"login"     gorm:"column:login;size:191"`
	Method   string `json:"method"    gorm:"column:method;size:10"`
	Path     string `json:"path"      gorm:"column:path;size:255;index:idx_audit_path"`
	Status   int    `json:"status"    gorm:"column:status"`
	Duration int64  `json:"duration"  gorm:"column:duration"` // milliseconds
	IP       string `json:"ip"        gorm:"column:ip;size:64"`
	// Summary 是 path params + 关键 query params 的 JSON 摘要 (不含 body, 不含
	// password 等敏感字段). 仅用于事后审计排查, 非业务记录.
	Summary string `json:"summary"   gorm:"column:summary;type:text"`
	Created int64  `json:"created"   gorm:"column:created;index:idx_audit_user_created,priority:2;index:idx_audit_created"`
}

// TableName 显式指定表名, 避免 gorm 复数推断.
func (AuditLog) TableName() string {
	return "audit_logs"
}
