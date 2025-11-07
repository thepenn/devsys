package migrate

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

// AutoMigrate synchronises the database schema with the model definitions.
func AutoMigrate(db *store.DB) error {
	gormDB := db.GetDB()

	if err := gormDB.AutoMigrate(
		&model.User{},
		&model.Forge{},
		&model.Repo{},
		&model.ServerConfig{},
		&model.RepoPipelineConfig{},
		&model.Pipeline{},
		&model.Workflow{},
		&model.Step{},
		&model.Task{},
		&model.LogEntry{},
		&model.Redirection{},
		&model.Certificate{},
	); err != nil {
		return err
	}

	if !gormDB.Migrator().HasColumn(&model.RepoPipelineConfig{}, "dockerfile") {
		if err := gormDB.Migrator().AddColumn(&model.RepoPipelineConfig{}, "Dockerfile"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.RepoPipelineConfig{}, "cron_schedules") {
		if err := gormDB.Migrator().AddColumn(&model.RepoPipelineConfig{}, "CronSchedules"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.Step{}, "approval") {
		if err := gormDB.Migrator().AddColumn(&model.Step{}, "Approval"); err != nil {
			return err
		}
	}

	deprecatedIndexes := []string{
		"uq_repos_forge_login",
		"uq_repos_name",
	}

	for _, idx := range deprecatedIndexes {
		if gormDB.Migrator().HasIndex(&model.Repo{}, idx) {
			if err := gormDB.Migrator().DropIndex(&model.Repo{}, idx); err != nil {
				return err
			}
		}
	}

	if err := migratePipelineSettingsIntoConfig(gormDB); err != nil {
		return err
	}

	return nil
}

type legacyPipelineSettings struct {
	ID               int64                              `gorm:"column:id"`
	RepoID           int64                              `gorm:"column:repo_id"`
	CleanupEnabled   bool                               `gorm:"column:cleanup_enabled"`
	RetentionDays    int                                `gorm:"column:retention_days"`
	MaxRecords       int                                `gorm:"column:max_records"`
	DisallowParallel bool                               `gorm:"column:disallow_parallel"`
	Variables        map[string]string                  `gorm:"column:variables;serializer:json"`
	Certificates     []model.PipelineCertificateBinding `gorm:"column:certificates;serializer:json"`
	CronEnabled      bool                               `gorm:"column:cron_enabled"`
	CronSpec         string                             `gorm:"column:cron_spec"`
	Created          int64                              `gorm:"column:created"`
	Updated          int64                              `gorm:"column:updated"`
}

func migratePipelineSettingsIntoConfig(gormDB *gorm.DB) error {
	if !gormDB.Migrator().HasTable("repo_pipeline_settings") {
		return nil
	}

	var records []legacyPipelineSettings
	if err := gormDB.Table("repo_pipeline_settings").Find(&records).Error; err != nil {
		return err
	}

	now := time.Now().Unix()

	for _, record := range records {
		if record.RepoID == 0 {
			continue
		}

		var cfg model.RepoPipelineConfig
		err := gormDB.Where("repo_id = ?", record.RepoID).Take(&cfg).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			newCfg := model.RepoPipelineConfig{
				RepoID:            record.RepoID,
				Content:           "",
				CleanupEnabled:    record.CleanupEnabled,
				RetentionDays:     record.RetentionDays,
				MaxRecords:        record.MaxRecords,
				DisallowParallel:  record.DisallowParallel,
				CronSchedules:     migrateCronSchedules(record.CronEnabled, record.CronSpec),
				Created:           record.Created,
				Updated:           record.Updated,
				LegacyCronEnabled: record.CronEnabled,
				LegacyCronSpec:    record.CronSpec,
			}
			if newCfg.MaxRecords <= 0 {
				newCfg.MaxRecords = 10
			}
			if newCfg.Created == 0 {
				if record.Updated > 0 {
					newCfg.Created = record.Updated
				} else {
					newCfg.Created = now
				}
			}
			if newCfg.Updated == 0 {
				newCfg.Updated = now
			}
			if err := gormDB.Create(&newCfg).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			cfg.CleanupEnabled = record.CleanupEnabled
			cfg.RetentionDays = record.RetentionDays
			if record.MaxRecords > 0 {
				cfg.MaxRecords = record.MaxRecords
			} else if cfg.MaxRecords <= 0 {
				cfg.MaxRecords = 10
			}
			cfg.DisallowParallel = record.DisallowParallel
			if len(cfg.CronSchedules) == 0 {
				cfg.CronSchedules = migrateCronSchedules(record.CronEnabled, record.CronSpec)
			}
			// retain legacy values for backward compatibility
			cfg.LegacyCronEnabled = record.CronEnabled
			cfg.LegacyCronSpec = record.CronSpec
			if record.Created > 0 && cfg.Created == 0 {
				cfg.Created = record.Created
			}
			if record.Updated > 0 {
				if cfg.Updated == 0 || record.Updated > cfg.Updated {
					cfg.Updated = record.Updated
				}
			} else if cfg.Updated == 0 {
				cfg.Updated = now
			}
			if cfg.Updated == 0 {
				cfg.Updated = now
			}
			if cfg.Created == 0 {
				cfg.Created = cfg.Updated
			}
			if err := gormDB.Save(&cfg).Error; err != nil {
				return err
			}
		}
	}

	if err := gormDB.Migrator().DropTable("repo_pipeline_settings"); err != nil {
		return err
	}

	return nil
}

func migrateCronSchedules(enabled bool, spec string) []string {
	if !enabled {
		return []string{}
	}
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return []string{}
	}
	return []string{trimmed}
}
