package model

type PipelineCertificateBinding struct {
	CertificateID int64  `json:"certificate_id"`
	Alias         string `json:"alias"`
}

type RepoPipelineConfig struct {
	ID               int64    `json:"id"                gorm:"column:id;primaryKey;autoIncrement"`
	RepoID           int64    `json:"repo_id"           gorm:"column:repo_id;uniqueIndex"`
	Content          string   `json:"content"           gorm:"column:content;type:longtext"`
	Dockerfile       string   `json:"dockerfile"        gorm:"column:dockerfile;type:longtext"`
	CleanupEnabled   bool     `json:"cleanup_enabled"   gorm:"column:cleanup_enabled"`
	RetentionDays    int      `json:"retention_days"    gorm:"column:retention_days"`
	MaxRecords       int      `json:"max_records"       gorm:"column:max_records"`
	DisallowParallel bool     `json:"disallow_parallel" gorm:"column:disallow_parallel"`
	CronSchedules    []string `json:"cron_schedules"    gorm:"column:cron_schedules;serializer:json"`
	Created          int64    `json:"created"           gorm:"column:created"`
	Updated          int64    `json:"updated"           gorm:"column:updated"`

	// legacy columns retained for backward-compatibility with existing databases.
	LegacyVariables    map[string]string            `json:"-" gorm:"column:variables;serializer:json"`
	LegacyCertificates []PipelineCertificateBinding `json:"-" gorm:"column:certificates;serializer:json"`
	LegacyCronEnabled  bool                         `json:"-" gorm:"column:cron_enabled"`
	LegacyCronSpec     string                       `json:"-" gorm:"column:cron_spec;size:255"`
}

func (RepoPipelineConfig) TableName() string {
	return "repo_pipeline_configs"
}
