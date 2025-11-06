package model

type ForgeType string

const (
	ForgeTypeGithub              ForgeType = "github"
	ForgeTypeGitlab              ForgeType = "gitlab"
	ForgeTypeGitee               ForgeType = "gitee"
	ForgeTypeGitea               ForgeType = "gitea"
	ForgeTypeForgejo             ForgeType = "forgejo"
	ForgeTypeBitbucket           ForgeType = "bitbucket"
	ForgeTypeBitbucketDatacenter ForgeType = "bitbucket-dc"
	ForgeTypeAddon               ForgeType = "addon"
)

type Forge struct {
	ID                int64          `json:"id"                           gorm:"column:id;primaryKey;autoIncrement"`
	Type              ForgeType      `json:"type"                         gorm:"column:type;size:100"`
	URL               string         `json:"url"                          gorm:"column:url;size:500"`
	OAuthClientID     string         `json:"client,omitempty"             gorm:"column:oauth_client_id;size:250"`
	OAuthClientSecret string         `json:"-"                            gorm:"column:oauth_client_secret;size:250"`
	SkipVerify        bool           `json:"skip_verify,omitempty"        gorm:"column:skip_verify"`
	OAuthHost         string         `json:"oauth_host,omitempty"         gorm:"column:oauth_host;size:250"`
	AdditionalOptions map[string]any `json:"additional_options,omitempty" gorm:"column:additional_options;serializer:json"`
}

func (Forge) TableName() string {
	return "forges"
}

func (f *Forge) PublicCopy() *Forge {
	return &Forge{
		ID:   f.ID,
		Type: f.Type,
		URL:  f.URL,
	}
}
