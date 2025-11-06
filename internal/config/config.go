package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

func Environ() (Config, error) {
	cfg := Config{}
	err := envconfig.Process("", &cfg)

	return cfg, err
}

type Config struct {
	Database Database
	Logging  Logging
	Server   Server
	Pipeline Pipeline
	Git      Git
	Auth     Auth
}

type Database struct {
	Driver         string `envconfig:"DATABASE_DRIVER"          default:"mysql"`
	Datasource     string `envconfig:"DATABASE_DATASOURCE"      default:"root:password@tcp(localhost:3306)/devops?charset=utf8mb4&parseTime=True&loc=Local"`
	MaxConnections int    `envconfig:"DATABASE_MAX_CONNECTIONS" default:"10"`
	ShowSql        bool   `envconfig:"DATABASE_SHOW_SQL"        default:"false"`
}
type Logging struct {
	Level  string `envconfig:"LOG_LEVEL"  default:"info"`
	Pretty bool   `envconfig:"LOG_PRETTY" default:"false"`
}

type Server struct {
	Host     string `envconfig:"SERVER_HOST" default:"localhost:8080"`
	RootPath string `envconfig:"SERVER_ROOT_PATH" default:"/api/v1"`
}

type Pipeline struct {
	WorkerCount   int `envconfig:"PIPELINE_WORKER_COUNT"   default:"2"`
	QueueCapacity int `envconfig:"PIPELINE_QUEUE_CAPACITY" default:"128"`
}

type Git struct {
	GitHub GitHub
	GitLab GitLab
	Gitee  Gitee
	Gitea  Gitea
}

type GitHub struct {
	Enabled       bool   `envconfig:"SERVER_GITHUB" default:"false"`
	URL           string `envconfig:"SERVER_GITHUB_URL" default:"https://github.com"`
	APIURL        string `envconfig:"SERVER_GITHUB_API_URL" default:"https://api.github.com"`
	ClientID      string `envconfig:"SERVER_GITHUB_CLIENT"`
	ClientSecret  string `envconfig:"SERVER_GITHUB_SECRET"`
	RedirectURL   string `envconfig:"SERVER_GITHUB_REDIRECT"`
	Scopes        string `envconfig:"SERVER_GITHUB_SCOPES" default:"read:user repo read:org"`
	Organizations string `envconfig:"SERVER_GITHUB_ORGS"`
	IncludeForks  bool   `envconfig:"SERVER_GITHUB_INCLUDE_FORKS" default:"false"`
	SkipVerify    bool   `envconfig:"SERVER_GITHUB_SKIP_VERIFY" default:"false"`
}

type GitLab struct {
	Enabled      bool   `envconfig:"SERVER_GITLAB" default:"true"`
	URL          string `envconfig:"SERVER_GITLAB_URL" default:"https://gitlab.com"`
	ClientID     string `envconfig:"SERVER_GITLAB_CLIENT"`
	ClientSecret string `envconfig:"SERVER_GITLAB_SECRET"`
	RedirectURL  string `envconfig:"SERVER_GITLAB_REDIRECT"`
	Scopes       string `envconfig:"SERVER_GITLAB_SCOPES" default:"read_user api"`
	SkipVerify   bool   `envconfig:"SERVER_GITLAB_SKIP_VERIFY" default:"false"`
	Organizations string `envconfig:"SERVER_GITLAB_ORGS"`
}

type Gitee struct {
	Enabled      bool   `envconfig:"SERVER_GITEE" default:"false"`
	URL          string `envconfig:"SERVER_GITEE_URL" default:"https://gitee.com"`
	ClientID     string `envconfig:"SERVER_GITEE_CLIENT"`
	ClientSecret string `envconfig:"SERVER_GITEE_SECRET"`
	RedirectURL  string `envconfig:"SERVER_GITEE_REDIRECT"`
	Scopes       string `envconfig:"SERVER_GITEE_SCOPES" default:"user_info projects"`
	SkipVerify   bool   `envconfig:"SERVER_GITEE_SKIP_VERIFY" default:"false"`
	Organizations string `envconfig:"SERVER_GITEE_ORGS"`
}

type Gitea struct {
	Enabled      bool   `envconfig:"SERVER_GITEA" default:"false"`
	URL          string `envconfig:"SERVER_GITEA_URL" default:""`
	ClientID     string `envconfig:"SERVER_GITEA_CLIENT"`
	ClientSecret string `envconfig:"SERVER_GITEA_SECRET"`
	RedirectURL  string `envconfig:"SERVER_GITEA_REDIRECT"`
	Scopes       string `envconfig:"SERVER_GITEA_SCOPES" default:"read:user user:email repo"`
	SkipVerify   bool   `envconfig:"SERVER_GITEA_SKIP_VERIFY" default:"false"`
	Organizations string `envconfig:"SERVER_GITEA_ORGS"`
}

type Auth struct {
	Provider      string        `envconfig:"SERVER_AUTH_PROVIDER" default:"gitlab"`
	SessionSecret string        `envconfig:"SERVER_AUTH_SESSION_SECRET" default:""`
	TokenTTL      time.Duration `envconfig:"SERVER_AUTH_TOKEN_TTL"      default:"24h"`
	StateTTL      time.Duration `envconfig:"SERVER_AUTH_STATE_TTL"      default:"10m"`
}
