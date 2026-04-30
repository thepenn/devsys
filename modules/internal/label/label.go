// Package label centralises ACL related metadata keys and the well-known
// label / role identifiers used across the codebase.
//
// 权限链路: OAuth -> User -> Roles -> Labels -> Endpoint.Labels
// 路由通过 go-restful Metadata 声明所属 labels (标签集), role 拥有一组 labels,
// 命中 (任一相同 label) 即放行. superadmin 拥有 Wildcard `*` 直接通过所有 ACL.
package label

// Metadata keys used on go-restful routes. Routes that opt-in to ACL must
// set MetaACL = true and MetaLabels = []string{...}.
const (
	MetaACL    = "acl"
	MetaLabels = "labels"
	MetaModule = "module"
	MetaRemark = "remark"
)

// Wildcard label, granted to superadmin. ACL middleware short-circuits when
// the user's effective label set contains Wildcard.
const Wildcard = "*"

// Built-in role identifiers. These are seeded on first migration and the
// gorbac engine refers to roles by these names.
const (
	RoleSuperadmin = "superadmin"
	RoleAdmin      = "admin"
	RoleOps        = "ops"
	RoleDeveloper  = "developer"
	RoleGuest      = "guest"
)

// Built-in label identifiers. Naming convention: "<module>:<action>".
const (
	// Kubernetes 管理
	K8sRead  = "k8s:read"
	K8sWrite = "k8s:write"

	// 项目 / 仓库管理
	ProjectRead     = "project:read"
	ProjectWrite    = "project:write"
	PipelineTrigger = "pipeline:trigger"

	// 通用 pipeline 模板
	PipelineTemplateRead  = "pipeline_template:read"
	PipelineTemplateWrite = "pipeline_template:write"

	// 独立 pipeline Job (与项目解耦)
	PipelineJobRead    = "pipeline_job:read"
	PipelineJobWrite   = "pipeline_job:write"
	PipelineJobTrigger = "pipeline_job:trigger"

	// 消息通知 / 告警
	MessageRead = "message:read"
	AlertRead   = "alert:read"
	AlertWrite  = "alert:write"

	// 数据库管理
	DBRead  = "db:read"
	DBWrite = "db:write"

	// 系统管理
	SystemRead        = "system:read"
	SystemWrite       = "system:write"
	SystemRoleWrite   = "system:role_write"
	SystemAudit       = "system:audit"
	SystemCertificate = "system:certificate"
)

// Module names used by the endpoint catalog (purely informational, surfaced
// in the role-management UI).
const (
	ModuleK8s     = "K8s 管理"
	ModuleProject = "项目管理"
	ModuleMessage = "消息通知"
	ModuleDB      = "数据库管理"
	ModuleSystem  = "系统管理"
	ModuleAuth    = "认证"
)

// AllLabels returns the canonical list of built-in labels in registration
// order. Used by the migration to seed the labels table.
func AllLabels() []LabelDef {
	return []LabelDef{
		{Name: Wildcard, Title: "全部权限", Module: ModuleSystem},

		{Name: K8sRead, Title: "Kubernetes 只读", Module: ModuleK8s},
		{Name: K8sWrite, Title: "Kubernetes 写入", Module: ModuleK8s},

		{Name: ProjectRead, Title: "项目只读", Module: ModuleProject},
		{Name: ProjectWrite, Title: "项目写入", Module: ModuleProject},
		{Name: PipelineTrigger, Title: "流水线触发", Module: ModuleProject},
		{Name: PipelineTemplateRead, Title: "通用 Pipeline 只读", Module: ModuleProject},
		{Name: PipelineTemplateWrite, Title: "通用 Pipeline 写入", Module: ModuleProject},
		{Name: PipelineJobRead, Title: "独立 Job 只读", Module: ModuleProject},
		{Name: PipelineJobWrite, Title: "独立 Job 写入", Module: ModuleProject},
		{Name: PipelineJobTrigger, Title: "独立 Job 触发", Module: ModuleProject},

		{Name: MessageRead, Title: "消息通知只读", Module: ModuleMessage},
		{Name: AlertRead, Title: "告警只读", Module: ModuleMessage},
		{Name: AlertWrite, Title: "告警写入", Module: ModuleMessage},

		{Name: DBRead, Title: "数据库只读", Module: ModuleDB},
		{Name: DBWrite, Title: "数据库写入", Module: ModuleDB},

		{Name: SystemRead, Title: "系统只读", Module: ModuleSystem},
		{Name: SystemWrite, Title: "系统写入", Module: ModuleSystem},
		{Name: SystemRoleWrite, Title: "角色权限管理", Module: ModuleSystem},
		{Name: SystemAudit, Title: "操作审计", Module: ModuleSystem},
		{Name: SystemCertificate, Title: "凭证管理", Module: ModuleSystem},
	}
}

// LabelDef is a transport struct used by the migration seed; it intentionally
// does NOT depend on the gorm model package to avoid an import cycle.
type LabelDef struct {
	Name   string
	Title  string
	Module string
}

// RoleDef captures a built-in role's seed values. The Labels field references
// label Name strings; the migration resolves them to label IDs.
type RoleDef struct {
	Name    string
	Title   string
	Parents []string
	Labels  []string
}

// AllRoles returns the canonical list of built-in roles. Inheritance
// (Parents) is encoded so the gorbac engine can wire up SetParents.
func AllRoles() []RoleDef {
	return []RoleDef{
		{
			Name:   RoleGuest,
			Title:  "访客 (默认角色)",
			Labels: []string{},
		},
		{
			Name:    RoleDeveloper,
			Title:   "开发者",
			Parents: []string{RoleGuest},
			Labels: []string{
				ProjectRead, ProjectWrite, PipelineTrigger, PipelineTemplateRead,
				PipelineJobRead, PipelineJobTrigger,
				K8sRead, MessageRead, AlertRead,
			},
		},
		{
			Name:    RoleOps,
			Title:   "运维",
			Parents: []string{RoleGuest},
			Labels: []string{
				K8sRead, K8sWrite,
				ProjectRead, PipelineTrigger, PipelineTemplateRead,
				PipelineJobRead, PipelineJobWrite, PipelineJobTrigger,
				MessageRead, AlertRead, AlertWrite,
				DBRead, DBWrite,
				SystemRead, SystemCertificate,
			},
		},
		{
			Name:    RoleAdmin,
			Title:   "管理员",
			Parents: []string{RoleDeveloper, RoleOps},
			Labels: []string{
				SystemWrite, SystemAudit, SystemRoleWrite, PipelineTemplateWrite,
				PipelineJobWrite,
			},
		},
		{
			Name:    RoleSuperadmin,
			Title:   "超级管理员",
			Parents: []string{RoleAdmin},
			Labels:  []string{Wildcard},
		},
	}
}

// IsBuiltinRole reports whether a role name refers to a built-in role.
func IsBuiltinRole(name string) bool {
	switch name {
	case RoleSuperadmin, RoleAdmin, RoleOps, RoleDeveloper, RoleGuest:
		return true
	default:
		return false
	}
}
