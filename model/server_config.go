package model

type ServerConfig struct {
	Key   string `json:"key"   gorm:"column:key;size:191;primaryKey"`
	Value string `json:"value" gorm:"column:value"`
}

func (ServerConfig) TableName() string {
	return "server_configs"
}
