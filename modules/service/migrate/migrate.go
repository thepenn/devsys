package migrate

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

// AutoMigrate synchronises the database schema with the model definitions.
func AutoMigrate(db *store.DB) error {
	gormDB := db.GetDB()

	// Pipeline 表的 (repo_id, number) 唯一索引在加 job_id 列前必须先 drop,
	// 否则 AutoMigrate 不会主动迁移. 加列后下面会让 AutoMigrate 重建为
	// (repo_id, job_id, number) 的 uq_pipeline_owner_number.
	if gormDB.Migrator().HasIndex(&model.Pipeline{}, "uq_pipeline_repo_number") {
		if err := gormDB.Migrator().DropIndex(&model.Pipeline{}, "uq_pipeline_repo_number"); err != nil {
			return err
		}
	}

	if err := gormDB.AutoMigrate(
		&model.User{},
		&model.Forge{},
		&model.Repo{},
		&model.ServerConfig{},
		&model.PipelineTemplate{},
		&model.PipelineJob{},
		&model.RepoPipelineConfig{},
		&model.Pipeline{},
		&model.Workflow{},
		&model.Step{},
		&model.Task{},
		&model.LogEntry{},
		&model.Certificate{},
		&model.Role{},
		&model.Label{},
		&model.Endpoint{},
		&model.UserRole{},
		&model.AuditLog{},
		&model.Message{},
	); err != nil {
		return err
	}

	if err := seedRBAC(gormDB); err != nil {
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
	if !gormDB.Migrator().HasColumn(&model.RepoPipelineConfig{}, "source") {
		if err := gormDB.Migrator().AddColumn(&model.RepoPipelineConfig{}, "Source"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.RepoPipelineConfig{}, "template_id") {
		if err := gormDB.Migrator().AddColumn(&model.RepoPipelineConfig{}, "TemplateID"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.RepoPipelineConfig{}, "template_variables") {
		if err := gormDB.Migrator().AddColumn(&model.RepoPipelineConfig{}, "TemplateVariables"); err != nil {
			return err
		}
	}
	// 旧行 source 列可能为空, 显式回填默认值, 让查询中 Source==""
	// 也能等价为 inline.
	if err := gormDB.Model(&model.RepoPipelineConfig{}).
		Where("source IS NULL OR source = ''").
		Update("source", model.PipelineConfigSourceInline).Error; err != nil {
		return err
	}
	if !gormDB.Migrator().HasColumn(&model.Step{}, "approval") {
		if err := gormDB.Migrator().AddColumn(&model.Step{}, "Approval"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.Pipeline{}, "job_id") {
		if err := gormDB.Migrator().AddColumn(&model.Pipeline{}, "JobID"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.Pipeline{}, "owner_kind") {
		if err := gormDB.Migrator().AddColumn(&model.Pipeline{}, "OwnerKind"); err != nil {
			return err
		}
	}
	if !gormDB.Migrator().HasColumn(&model.PipelineJob{}, "cron_schedules") {
		if err := gormDB.Migrator().AddColumn(&model.PipelineJob{}, "CronSchedules"); err != nil {
			return err
		}
	}
	// PipelineTemplate.kind: 旧表无此列, 加列后回填默认值 pipeline.
	if !gormDB.Migrator().HasColumn(&model.PipelineTemplate{}, "kind") {
		if err := gormDB.Migrator().AddColumn(&model.PipelineTemplate{}, "Kind"); err != nil {
			return err
		}
	}
	if err := gormDB.Model(&model.PipelineTemplate{}).
		Where("kind IS NULL OR kind = ''").
		Update("kind", model.PipelineTemplateKindPipeline).Error; err != nil {
		return err
	}
	// RepoPipelineConfig.compose_steps: 新增 source=compose 模式所需 JSON 列.
	if !gormDB.Migrator().HasColumn(&model.RepoPipelineConfig{}, "compose_steps") {
		if err := gormDB.Migrator().AddColumn(&model.RepoPipelineConfig{}, "ComposeSteps"); err != nil {
			return err
		}
	}
	// 旧 pipelines 行 owner_kind 可能为空, 显式回填为 repo, 让查询统计时
	// 不会把它们误算进 Job 范围.
	if err := gormDB.Model(&model.Pipeline{}).
		Where("owner_kind IS NULL OR owner_kind = ''").
		Update("owner_kind", model.PipelineOwnerRepo).Error; err != nil {
		return err
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

	if err := seedBuiltinStepTemplates(gormDB); err != nil {
		return err
	}

	return nil
}

// seedBuiltinStepTemplates 在 PipelineTemplate 表里幂等写入官方预制的 step 模板,
// 让用户开箱即可在 compose 模式里直接引用. 仅当同名记录不存在时插入, 永远不覆盖
// 用户手动改过的内容.
func seedBuiltinStepTemplates(db *gorm.DB) error {
	type seed struct {
		Name        string
		DisplayName string
		Description string
		Content     string
	}
	now := time.Now().Unix()
	seeds := []seed{
		{
			Name:        "buildkit-image",
			DisplayName: "BuildKit 构建镜像 (无需 dockerd)",
			Description: "使用 moby/buildkit 在容器内 daemonless 模式构建并推送镜像. 用户只需提供 docker 凭证 + repo 即可. 详见 docs/pipeline.md kind: build 章节.",
			Content: `- name: build-and-push-image
  kind: build
  certificate: ${DOCKER_CERT_NAME:-aliyun_docker_registry}
  build:
    repo: ${IMAGE_REPO}
    tags:
      - latest
      - build-${CI_PIPELINE_NUMBER}
    dockerfile: ${DOCKERFILE:-Dockerfile}
    context: ${BUILD_CONTEXT:-.}
    platforms:
      - linux/amd64
`,
		},
	}
	for _, s := range seeds {
		var existing model.PipelineTemplate
		err := db.Where("name = ?", s.Name).Take(&existing).Error
		if err == nil {
			continue // 已存在, 不动
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		published := time.Unix(now, 0)
		row := model.PipelineTemplate{
			Name:             s.Name,
			DisplayName:      s.DisplayName,
			Description:      s.Description,
			Kind:             model.PipelineTemplateKindStep,
			DraftContent:     s.Content,
			PublishedContent: s.Content,
			PublishedAt:      &published,
			PublishedBy:      "system",
			CreatedBy:        "system",
			UpdatedBy:        "system",
			Created:          now,
			Updated:          now,
		}
		if err := db.Create(&row).Error; err != nil {
			return err
		}
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

// seedRBAC ensures the built-in label catalog and roles exist.
//
// Label 行始终用 OnConflict UPSERT 同步 title/module (label 字典是代码常量,
// 由代码权威定义). 角色行同步 title/parents (角色元数据).
//
// 但角色 ↔ label 的多对多绑定**只在 role_labels 为空时才 seed**: 这是为了
// 避免 admin 在 UI 调整内置角色 (例如给 ops 加 db:write) 后, 下次启动被
// 默认值覆盖. 想"恢复内置默认"时把目标角色 label 全删 → 重启即可自动回填.
func seedRBAC(db *gorm.DB) error {
	now := time.Now().Unix()

	labelDefs := label.AllLabels()
	for i := range labelDefs {
		def := labelDefs[i]
		row := model.Label{
			Name:    def.Name,
			Title:   def.Title,
			Module:  def.Module,
			Builtin: true,
		}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "module", "builtin"}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}

	var allLabels []model.Label
	if err := db.Find(&allLabels).Error; err != nil {
		return err
	}
	labelByName := make(map[string]model.Label, len(allLabels))
	for _, l := range allLabels {
		labelByName[l.Name] = l
	}

	for _, rd := range label.AllRoles() {
		role := model.Role{
			Name:    rd.Name,
			Title:   rd.Title,
			Parents: strings.Join(rd.Parents, ","),
			Builtin: true,
			Created: now,
			Updated: now,
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"title", "parents", "builtin", "updated",
			}),
		}).Create(&role).Error; err != nil {
			return err
		}

		var existing model.Role
		if err := db.Where("name = ?", rd.Name).Take(&existing).Error; err != nil {
			return err
		}

		// 仅当当前没有任何 role_labels 关联时 seed; 否则保留 admin 在 UI
		// 上的自定义. 想恢复内置默认: 在 UI 把 label 删空后重启即可.
		var existingLabelCount int64
		if err := db.Table("role_labels").
			Where("role_id = ?", existing.ID).
			Count(&existingLabelCount).Error; err != nil {
			return err
		}
		if existingLabelCount > 0 {
			continue
		}

		seen := make(map[int64]struct{}, len(rd.Labels))
		for _, name := range rd.Labels {
			lab, ok := labelByName[name]
			if !ok {
				continue
			}
			if _, dup := seen[lab.ID]; dup {
				continue
			}
			seen[lab.ID] = struct{}{}
			if err := db.Exec(
				"INSERT INTO role_labels (role_id, label_id) VALUES (?, ?)",
				existing.ID, lab.ID,
			).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
