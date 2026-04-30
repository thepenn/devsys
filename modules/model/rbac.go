package model

// Role 表示一个 RBAC 角色, 通过 role_labels 多对多关联到 Label.
// 角色继承通过 Parents (逗号分隔, 引用其它 role.Name) 在内存图中重建.
type Role struct {
	ID      int64   `json:"id"      gorm:"column:id;primaryKey;autoIncrement"`
	Name    string  `json:"name"    gorm:"column:name;size:64;uniqueIndex"`
	Title   string  `json:"title"   gorm:"column:title;size:128"`
	Parents string  `json:"parents" gorm:"column:parents;size:255"`
	Builtin bool    `json:"builtin" gorm:"column:builtin"`
	Created int64   `json:"created" gorm:"column:created"`
	Updated int64   `json:"updated" gorm:"column:updated"`
	Labels  []Label `json:"labels,omitempty" gorm:"many2many:role_labels;joinForeignKey:role_id;joinReferences:label_id"`
}

// TableName 显式指定表名, 避免 gorm 默认复数推断.
func (Role) TableName() string {
	return "roles"
}

// Label 是权限标签, 同时充当 gorbac.Permission 的 ID.
// Endpoint 携带的 labels 与 role 拥有的 labels 取交集即可放行.
type Label struct {
	ID      int64  `json:"id"      gorm:"column:id;primaryKey;autoIncrement"`
	Name    string `json:"name"    gorm:"column:name;size:64;uniqueIndex"`
	Title   string `json:"title"   gorm:"column:title;size:128"`
	Module  string `json:"module"  gorm:"column:module;size:64;index"`
	Builtin bool   `json:"builtin" gorm:"column:builtin"`
}

func (Label) TableName() string {
	return "labels"
}

// Endpoint 是被 ACL 中间件保护的 API 路由元数据, 由 router.Sync 在服务启动时
// 从 go-restful 已注册路由中自动 upsert. 仅供管理 UI 展示, 实际鉴权数据从
// 路由 Metadata 中读取.
type Endpoint struct {
	ID      int64   `json:"id"      gorm:"column:id;primaryKey;autoIncrement"`
	Path    string  `json:"path"    gorm:"column:path;size:255;uniqueIndex:uq_endpoints_method_path,priority:2"`
	Method  string  `json:"method"  gorm:"column:method;size:10;uniqueIndex:uq_endpoints_method_path,priority:1"`
	Module  string  `json:"module"  gorm:"column:module;size:64"`
	Remark  string  `json:"remark"  gorm:"column:remark;size:255"`
	Updated int64   `json:"updated" gorm:"column:updated"`
	Labels  []Label `json:"labels,omitempty" gorm:"many2many:endpoint_labels;joinForeignKey:endpoint_id;joinReferences:label_id"`
}

func (Endpoint) TableName() string {
	return "endpoints"
}

// UserRole 是用户与角色的多对多关联.
type UserRole struct {
	UserID  int64 `json:"user_id" gorm:"column:user_id;primaryKey"`
	RoleID  int64 `json:"role_id" gorm:"column:role_id;primaryKey"`
	Created int64 `json:"created" gorm:"column:created"`
}

func (UserRole) TableName() string {
	return "user_roles"
}
