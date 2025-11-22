package model

type Redirection struct {
	ID       int64  `gorm:"column:id;primaryKey;autoIncrement"`
	RepoID   int64  `gorm:"column:repo_id"`
	FullName string `gorm:"column:repo_full_name;size:191;uniqueIndex"`
}

func (r Redirection) TableName() string {
	return "redirections"
}
