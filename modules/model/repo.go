package model

import (
	"fmt"
	"strings"
)

type ApprovalMode string

const (
	RequireApprovalNone         ApprovalMode = "none"
	RequireApprovalForks        ApprovalMode = "forks"
	RequireApprovalPullRequests ApprovalMode = "pull_requests"
	RequireApprovalAllEvents    ApprovalMode = "all_events"
)

func (mode ApprovalMode) Valid() bool {
	switch mode {
	case RequireApprovalNone,
		RequireApprovalForks,
		RequireApprovalPullRequests,
		RequireApprovalAllEvents:
		return true
	default:
		return false
	}
}

type Repo struct {
	ID      int64 `json:"id,omitempty"                    gorm:"column:id;primaryKey;autoIncrement"`
	UserID  int64 `json:"-"                               gorm:"column:user_id;index"`
	ForgeID int64 `json:"forge_id,omitempty"              gorm:"column:forge_id;index;uniqueIndex:uq_repos_forge_remote_id;uniqueIndex:uq_repos_forge_owner_name,priority:1"`

	ForgeRemoteID                ForgeRemoteID        `json:"forge_remote_id"                 gorm:"column:forge_remote_id;size:191;uniqueIndex:uq_repos_forge_remote_id"`
	OrgID                        int64                `json:"org_id"                          gorm:"column:org_id;index"`
	Owner                        string               `json:"owner"                           gorm:"column:owner;size:191;index;uniqueIndex:uq_repos_forge_owner_name,priority:2"`
	Name                         string               `json:"name"                            gorm:"column:name;size:191;uniqueIndex:uq_repos_forge_owner_name,priority:3"`
	FullName                     string               `json:"full_name"                       gorm:"column:full_name;size:191;uniqueIndex"`
	Avatar                       string               `json:"avatar_url,omitempty"            gorm:"column:avatar;size:500"`
	ForgeURL                     string               `json:"forge_url,omitempty"             gorm:"column:forge_url;size:1000"`
	Clone                        string               `json:"clone_url,omitempty"             gorm:"column:clone;size:1000"`
	CloneSSH                     string               `json:"clone_url_ssh"                   gorm:"column:clone_ssh;size:1000"`
	Branch                       string               `json:"default_branch,omitempty"        gorm:"column:branch;size:500"`
	PREnabled                    bool                 `json:"pr_enabled"                      gorm:"column:pr_enabled;default:true"`
	Timeout                      int64                `json:"timeout,omitempty"               gorm:"column:timeout"`
	Visibility                   RepoVisibility       `json:"visibility"                      gorm:"column:visibility;size:10"`
	IsSCMPrivate                 bool                 `json:"private"                         gorm:"column:private"`
	Trusted                      TrustedConfiguration `json:"trusted"                         gorm:"column:trusted;serializer:json"`
	RequireApproval              ApprovalMode         `json:"require_approval"                gorm:"column:require_approval;size:50"`
	ApprovalAllowedUsers         []string             `json:"approval_allowed_users"          gorm:"column:approval_allowed_users;serializer:json"`
	IsActive                     bool                 `json:"active"                          gorm:"column:active"`
	AllowPull                    bool                 `json:"allow_pr"                        gorm:"column:allow_pr"`
	AllowDeploy                  bool                 `json:"allow_deploy"                    gorm:"column:allow_deploy"`
	Config                       string               `json:"config_file"                     gorm:"column:config_path;size:500"`
	Hash                         string               `json:"-"                               gorm:"column:hash;size:500"`
	CancelPreviousPipelineEvents []WebhookEvent       `json:"cancel_previous_pipeline_events" gorm:"column:cancel_previous_pipeline_events;serializer:json"`
	NetrcTrustedPlugins          []string             `json:"netrc_trusted"                   gorm:"column:netrc_trusted;serializer:json"`
	ConfigExtensionEndpoint      string               `json:"config_extension_endpoint"       gorm:"column:config_extension_endpoint;size:500"`
}

func (Repo) TableName() string {
	return "repos"
}

type RepoFilter struct {
	Name string
}

func (r *Repo) ResetVisibility() {
	r.Visibility = VisibilityPublic
	if r.IsSCMPrivate {
		r.Visibility = VisibilityPrivate
	}
}

func ParseRepo(str string) (user, repo string, err error) {
	before, after, _ := strings.Cut(str, "/")
	if before == "" || after == "" {
		err = fmt.Errorf("invalid or missing repository (e.g. octocat/hello-world)")
		return user, repo, err
	}
	user = before
	repo = after
	return user, repo, err
}

func (r *Repo) Update(from *Repo) {
	if from.ForgeRemoteID.IsValid() {
		r.ForgeRemoteID = from.ForgeRemoteID
	}
	r.Owner = from.Owner
	r.Name = from.Name
	r.FullName = from.FullName
	r.Avatar = from.Avatar
	r.ForgeURL = from.ForgeURL
	r.PREnabled = from.PREnabled
	if len(from.Clone) > 0 {
		r.Clone = from.Clone
	}
	if len(from.CloneSSH) > 0 {
		r.CloneSSH = from.CloneSSH
	}
	r.Branch = from.Branch
	if from.IsSCMPrivate != r.IsSCMPrivate {
		if from.IsSCMPrivate {
			r.Visibility = VisibilityPrivate
		} else {
			r.Visibility = VisibilityPublic
		}
	}
	r.IsSCMPrivate = from.IsSCMPrivate
}

type RepoPatch struct {
	Config                       *string                    `json:"config_file,omitempty"`
	RequireApproval              *string                    `json:"require_approval,omitempty"`
	ApprovalAllowedUsers         *[]string                  `json:"approval_allowed_users,omitempty"`
	Timeout                      *int64                     `json:"timeout,omitempty"`
	Visibility                   *string                    `json:"visibility,omitempty"`
	AllowPull                    *bool                      `json:"allow_pr,omitempty"`
	AllowDeploy                  *bool                      `json:"allow_deploy,omitempty"`
	CancelPreviousPipelineEvents *[]WebhookEvent            `json:"cancel_previous_pipeline_events"`
	NetrcTrusted                 *[]string                  `json:"netrc_trusted"`
	Trusted                      *TrustedConfigurationPatch `json:"trusted"`
	ConfigExtensionEndpoint      *string                    `json:"config_extension_endpoint,omitempty"`
}

type ForgeRemoteID string

func (r ForgeRemoteID) IsValid() bool {
	return r != "" && r != "0"
}

type TrustedConfiguration struct {
	Network  bool `json:"network"`
	Volumes  bool `json:"volumes"`
	Security bool `json:"security"`
}

type TrustedConfigurationPatch struct {
	Network  *bool `json:"network"`
	Volumes  *bool `json:"volumes"`
	Security *bool `json:"security"`
}

type RepoLastPipeline struct {
	*Repo
	LastPipeline *Pipeline `json:"last_pipeline,omitempty"`
}
