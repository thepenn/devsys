package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"github.com/xanzy/go-gitlab"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"

	"github.com/thepenn/devsys/internal/config"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
	"github.com/thepenn/devsys/service/repo"
	"github.com/thepenn/devsys/service/user"
	"gorm.io/gorm"
)

const (
	providerGitHub = "github"
	providerGitLab = "gitlab"
	providerGitee  = "gitee"
	providerGitea  = "gitea"
)

type Service struct {
	cfg   *config.Config
	db    *store.DB
	users *user.Service
	repos *repo.Service

	provider   string
	sessionKey []byte
	tokenTTL   time.Duration
	scopes     []string
	httpClient *http.Client

	githubWebBase      string
	githubAPIBase      string
	githubOrgs         []string
	githubIncludeForks bool

	gitlabOrgs []string
	giteaOrgs  []string
	giteeOrgs  []string
}

type giteeUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type giteeRepo struct {
	ID            int64      `json:"id"`
	FullName      string     `json:"full_name"`
	Name          string     `json:"name"`
	HTMLURL       string     `json:"html_url"`
	SSHURL        string     `json:"ssh_url"`
	CloneURL      string     `json:"clone_url"`
	DefaultBranch string     `json:"default_branch"`
	Private       bool       `json:"private"`
	Owner         giteeOwner `json:"owner"`
}

type giteeOwner struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	SiteAdmin bool   `json:"site_admin"`
}

type githubEmail struct {
	Email      string `json:"email"`
	Primary    bool   `json:"primary"`
	Verified   bool   `json:"verified"`
	Visibility string `json:"visibility"`
}

type githubRepoOwner struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type githubOrg struct {
	Login string `json:"login"`
}

type githubOrgMembership struct {
	State        string    `json:"state"`
	Role         string    `json:"role"`
	Organization githubOrg `json:"organization"`
}

type githubRepo struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	FullName      string          `json:"full_name"`
	Owner         githubRepoOwner `json:"owner"`
	HTMLURL       string          `json:"html_url"`
	CloneURL      string          `json:"clone_url"`
	SSHURL        string          `json:"ssh_url"`
	DefaultBranch string          `json:"default_branch"`
	Private       bool            `json:"private"`
	Visibility    string          `json:"visibility"`
	Fork          bool            `json:"fork"`
	Archived      bool            `json:"archived"`
}

func New(cfg *config.Config, db *store.DB, users *user.Service, repos *repo.Service) (*Service, error) {
	secret := strings.TrimSpace(cfg.Auth.SessionSecret)
	if secret == "" {
		generated, err := randomState()
		if err != nil {
			return nil, fmt.Errorf("generate session secret: %w", err)
		}
		log.Warn().Msg("auth session secret not configured; using in-memory secret")
		secret = generated
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.Auth.Provider))
	if provider == "" {
		provider = providerGitLab
	}

	var scopes []string
	var httpClient *http.Client
	var githubWebBase string
	var githubAPIBase string
	var githubOrgs []string
	var githubIncludeForks bool
	var gitlabOrgs []string
	var giteaOrgs []string
	var giteeOrgs []string
	switch provider {
	case providerGitHub:
		if !cfg.Git.GitHub.Enabled {
			return nil, errors.New("github authentication disabled")
		}
		scopes = strings.Fields(cfg.Git.GitHub.Scopes)
		if len(scopes) == 0 {
			scopes = []string{"read:user", "repo"}
		}
		httpClient = newHTTPClient(cfg.Git.GitHub.SkipVerify)
		githubWebBase = normalizeBaseURL(cfg.Git.GitHub.URL, "https://github.com")
		githubAPIBase = normalizeBaseURL(cfg.Git.GitHub.APIURL, "https://api.github.com")
		githubOrgs = splitAndTrim(cfg.Git.GitHub.Organizations, ",")
		githubIncludeForks = cfg.Git.GitHub.IncludeForks
	case providerGitLab:
		if !cfg.Git.GitLab.Enabled {
			return nil, errors.New("gitlab authentication disabled")
		}
		scopes = strings.Fields(cfg.Git.GitLab.Scopes)
		if len(scopes) == 0 {
			scopes = []string{"read_user", "api"}
		}
		httpClient = newHTTPClient(cfg.Git.GitLab.SkipVerify)
		gitlabOrgs = splitAndTrim(cfg.Git.GitLab.Organizations, ",")
	case providerGitee:
		if !cfg.Git.Gitee.Enabled {
			return nil, errors.New("gitee authentication disabled")
		}
		scopes = strings.Fields(cfg.Git.Gitee.Scopes)
		if len(scopes) == 0 {
			scopes = []string{"user_info", "projects"}
		}
		httpClient = newHTTPClient(cfg.Git.Gitee.SkipVerify)
		giteeOrgs = splitAndTrim(cfg.Git.Gitee.Organizations, ",")
	case providerGitea:
		if !cfg.Git.Gitea.Enabled {
			return nil, errors.New("gitea authentication disabled")
		}
		scopes = strings.Fields(cfg.Git.Gitea.Scopes)
		if len(scopes) == 0 {
			scopes = []string{"read:user", "user:email", "repo"}
		}
		httpClient = newHTTPClient(cfg.Git.Gitea.SkipVerify)
		giteaOrgs = splitAndTrim(cfg.Git.Gitea.Organizations, ",")
	default:
		return nil, fmt.Errorf("unsupported auth provider: %s", provider)
	}

	return &Service{
		cfg:        cfg,
		db:         db,
		users:      users,
		repos:      repos,
		provider:   provider,
		sessionKey: []byte(secret),
		tokenTTL:   cfg.Auth.TokenTTL,
		scopes:     scopes,
		httpClient: httpClient,

		githubWebBase:      githubWebBase,
		githubAPIBase:      githubAPIBase,
		githubOrgs:         githubOrgs,
		githubIncludeForks: githubIncludeForks,
		gitlabOrgs:         gitlabOrgs,
		giteaOrgs:          giteaOrgs,
		giteeOrgs:          giteeOrgs,
	}, nil
}

func (s *Service) BeginGitLabAuth(ctx context.Context, redirect string) (string, string, error) {
	switch s.provider {
	case providerGitHub:
		return s.beginGitHubAuth(ctx, redirect)
	case providerGitee:
		return s.beginGiteeAuth(ctx, redirect)
	case providerGitea:
		return s.beginGiteaAuth(ctx, redirect)
	default:
		return s.beginGitLabAuth(ctx, redirect)
	}
}

func (s *Service) CompleteGitLabAuth(ctx context.Context, code, state string) (*AuthResponse, error) {
	switch s.provider {
	case providerGitHub:
		return s.completeGitHubAuth(ctx, code, state)
	case providerGitee:
		return s.completeGiteeAuth(ctx, code, state)
	case providerGitea:
		return s.completeGiteaAuth(ctx, code, state)
	default:
		return s.completeGitLabAuth(ctx, code, state)
	}
}

func (s *Service) SyncGitLabRepositories(ctx context.Context, userID int64) error {
	switch s.provider {
	case providerGitHub:
		return s.syncGitHubRepositories(ctx, userID)
	case providerGitee:
		return s.syncGiteeRepositories(ctx, userID)
	case providerGitea:
		return s.syncGiteaRepositories(ctx, userID)
	default:
		return s.syncGitLabRepositories(ctx, userID)
	}
}

func (s *Service) SyncRepositories(ctx context.Context, userID int64) error {
	return s.SyncGitLabRepositories(ctx, userID)
}

func (s *Service) SyncRepository(ctx context.Context, userID int64, remoteID string) error {
	switch s.provider {
	case providerGitHub:
		return s.syncGitHubRepository(ctx, userID, remoteID)
	case providerGitee:
		return s.syncGiteeRepository(ctx, userID, remoteID)
	case providerGitea:
		return s.syncGiteaRepository(ctx, userID, remoteID)
	default:
		return s.syncGitLabRepository(ctx, userID, remoteID)
	}
}

func (s *Service) ParseToken(tokenString string) (*SessionClaims, error) {
	claims := &SessionClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.sessionKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *Service) CurrentUser(ctx context.Context, userID int64) (*UserInfo, error) {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userModel == nil {
		return nil, nil
	}
	info := toUserInfo(userModel, s.provider)
	return &info, nil
}

func (s *Service) beginGitLabAuth(ctx context.Context, redirect string) (string, string, error) {
	if s.cfg.Git.GitLab.ClientID == "" || s.cfg.Git.GitLab.ClientSecret == "" || s.cfg.Git.GitLab.RedirectURL == "" {
		return "", "", errors.New("gitlab oauth configuration incomplete")
	}

	oauthCfg := s.gitLabOAuthConfig()
	state, err := randomState()
	if err != nil {
		return "", "", err
	}

	encodedState, err := s.encodeState(state, redirect)
	if err != nil {
		return "", "", err
	}

	log.Debug().Str("state", state).Str("redirect", redirect).Msg("gitlab oauth begin")

	authURL := oauthCfg.AuthCodeURL(encodedState, oauth2.SetAuthURLParam("scope", strings.Join(s.scopes, " ")))
	return encodedState, authURL, nil
}

func (s *Service) completeGitLabAuth(ctx context.Context, code, state string) (*AuthResponse, error) {
	if code == "" || state == "" {
		return nil, errors.New("missing code or state")
	}
	rawState, redirect, err := s.decodeState(state)
	if err != nil {
		return nil, err
	}
	log.Debug().Str("state", rawState).Msg("gitlab oauth callback")

	oauthCfg := s.gitLabOAuthConfig()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange oauth token: %w", err)
	}

	client, err := s.gitLabClient(token.AccessToken)
	if err != nil {
		return nil, err
	}

	gitUser, _, err := client.Users.CurrentUser()
	if err != nil {
		return nil, fmt.Errorf("fetch gitlab user: %w", err)
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitlab, s.cfg.Git.GitLab.URL)
	if err != nil {
		return nil, err
	}

	appUser, err := s.users.UpsertGitUser(ctx, forge.ID, user.GitUser{
		RemoteID: strconv.FormatInt(int64(gitUser.ID), 10),
		Login:    firstNonEmpty(gitUser.Username, gitUser.Name),
		Email:    firstNonEmpty(gitUser.Email, gitUser.PublicEmail),
		Avatar:   gitUser.AvatarURL,
		IsAdmin:  gitUser.IsAdmin,
	}, token)
	if err != nil {
		return nil, err
	}

	repos, err := s.listGitLabProjects(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := s.repos.SyncGitRepositories(ctx, forge.ID, appUser.ID, repos, false); err != nil {
		return nil, err
	}

	jwtToken, err := s.generateToken(appUser)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token:    jwtToken,
		User:     toUserInfo(appUser, providerGitLab),
		Redirect: redirect,
	}, nil
}

func (s *Service) syncGitLabRepositories(ctx context.Context, userID int64) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if userModel.AccessToken == "" {
		return errors.New("user has no stored gitlab token")
	}

	client, err := s.gitLabClient(userModel.AccessToken)
	if err != nil {
		return err
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitlab, s.cfg.Git.GitLab.URL)
	if err != nil {
		return err
	}

	repos, err := s.listGitLabProjects(ctx, client)
	if err != nil {
		return err
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, repos, true)
}

func (s *Service) syncGitLabRepository(ctx context.Context, userID int64, remoteID string) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if userModel.AccessToken == "" {
		return errors.New("user has no stored gitlab token")
	}

	client, err := s.gitLabClient(userModel.AccessToken)
	if err != nil {
		return err
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitlab, s.cfg.Git.GitLab.URL)
	if err != nil {
		return err
	}

	projectID, err := strconv.ParseInt(remoteID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid repository id: %w", err)
	}

	project, _, err := client.Projects.GetProject(projectID, nil)
	if err != nil {
		return fmt.Errorf("fetch gitlab project: %w", err)
	}

	owner := gitLabProjectNamespace(project)
	if !s.gitlabOrgAllowed(owner) {
		if strings.TrimSpace(owner) == "" {
			owner = "(unknown)"
		}
		return fmt.Errorf("gitlab project owner %s not permitted by configuration", owner)
	}

	repoData := convertGitLabProject(project)
	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, []repo.GitRepository{repoData}, true)
}

func (s *Service) beginGitHubAuth(ctx context.Context, redirect string) (string, string, error) {
	oauthCfg, err := s.githubOAuthConfig()
	if err != nil {
		return "", "", err
	}

	state, err := randomState()
	if err != nil {
		return "", "", err
	}

	encodedState, err := s.encodeState(state, redirect)
	if err != nil {
		return "", "", err
	}

	log.Debug().Str("state", state).Str("redirect", redirect).Msg("github oauth begin")

	authURL := oauthCfg.AuthCodeURL(encodedState, oauth2.SetAuthURLParam("scope", strings.Join(s.scopes, " ")))
	return encodedState, authURL, nil
}

func (s *Service) completeGitHubAuth(ctx context.Context, code, state string) (*AuthResponse, error) {
	if code == "" || state == "" {
		return nil, errors.New("missing code or state")
	}

	rawState, redirect, err := s.decodeState(state)
	if err != nil {
		return nil, err
	}
	log.Debug().Str("state", rawState).Msg("github oauth callback")

	oauthCfg, err := s.githubOAuthConfig()
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange github oauth token: %w", err)
	}

	apiClient := oauthCfg.Client(ctx, token)

	userInfo, err := s.githubFetchCurrentUser(ctx, apiClient)
	if err != nil {
		return nil, err
	}
	if userInfo == nil || strings.TrimSpace(userInfo.Login) == "" {
		return nil, errors.New("github user login empty")
	}
	if strings.TrimSpace(userInfo.Email) == "" {
		if email, err := s.githubFetchPrimaryEmail(ctx, apiClient); err == nil && email != "" {
			userInfo.Email = email
		}
	}
	isAdmin := userInfo.SiteAdmin
	if !isAdmin && len(s.githubOrgs) > 0 {
		admin, err := s.githubIsOrganizationAdmin(ctx, apiClient)
		if err != nil {
			log.Warn().Err(err).Msg("failed to determine github organization admin status")
		}
		if admin {
			isAdmin = true
		}
	}

	forgeURL := s.githubWebBase
	if forgeURL == "" {
		forgeURL = normalizeBaseURL(s.cfg.Git.GitHub.URL, "https://github.com")
	}
	forge, err := s.ensureForge(ctx, model.ForgeTypeGithub, forgeURL)
	if err != nil {
		return nil, err
	}

	appUser, err := s.users.UpsertGitUser(ctx, forge.ID, user.GitUser{
		RemoteID: strconv.FormatInt(userInfo.ID, 10),
		Login:    firstNonEmpty(userInfo.Login, userInfo.Name),
		Email:    userInfo.Email,
		Avatar:   userInfo.AvatarURL,
		IsAdmin:  isAdmin,
	}, token)
	if err != nil {
		return nil, err
	}

	repositories, err := s.listGitHubRepositories(ctx, apiClient)
	if err != nil {
		return nil, err
	}
	if err := s.repos.SyncGitRepositories(ctx, forge.ID, appUser.ID, repositories, false); err != nil {
		return nil, err
	}

	jwtToken, err := s.generateToken(appUser)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token:    jwtToken,
		User:     toUserInfo(appUser, providerGitHub),
		Redirect: redirect,
	}, nil
}

func (s *Service) syncGitHubRepositories(ctx context.Context, userID int64) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if strings.TrimSpace(userModel.AccessToken) == "" {
		return errors.New("user has no stored github token")
	}

	token := &oauth2.Token{
		AccessToken:  userModel.AccessToken,
		RefreshToken: userModel.RefreshToken,
	}
	if userModel.Expiry > 0 {
		token.Expiry = time.Unix(userModel.Expiry, 0)
	}

	oauthCfg, err := s.githubOAuthConfig()
	if err != nil {
		return err
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	apiClient := oauthCfg.Client(ctx, token)

	forgeURL := s.githubWebBase
	if forgeURL == "" {
		forgeURL = normalizeBaseURL(s.cfg.Git.GitHub.URL, "https://github.com")
	}
	forge, err := s.ensureForge(ctx, model.ForgeTypeGithub, forgeURL)
	if err != nil {
		return err
	}

	repositories, err := s.listGitHubRepositories(ctx, apiClient)
	if err != nil {
		return err
	}

	if !userModel.Admin {
		if admin, err := s.githubIsOrganizationAdmin(ctx, apiClient); err == nil {
			if admin {
				userModel.Admin = true
				if err := s.users.Update(ctx, userModel); err != nil {
					log.Warn().Err(err).Msg("failed to update user admin flag for github organization admin")
				}
			}
		} else {
			log.Warn().Err(err).Msg("failed to determine github organization admin status during sync")
		}
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, repositories, true)
}

func (s *Service) syncGitHubRepository(ctx context.Context, userID int64, remoteID string) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if strings.TrimSpace(userModel.AccessToken) == "" {
		return errors.New("user has no stored github token")
	}

	repoID, err := strconv.ParseInt(remoteID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid repository id: %w", err)
	}

	token := &oauth2.Token{
		AccessToken:  userModel.AccessToken,
		RefreshToken: userModel.RefreshToken,
	}
	if userModel.Expiry > 0 {
		token.Expiry = time.Unix(userModel.Expiry, 0)
	}

	oauthCfg, err := s.githubOAuthConfig()
	if err != nil {
		return err
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	apiClient := oauthCfg.Client(ctx, token)

	repository, err := s.fetchGitHubRepositoryByID(ctx, apiClient, repoID)
	if err != nil {
		return err
	}
	if repository == nil {
		return fmt.Errorf("github repository %d not found", repoID)
	}
	if !s.githubOrgAllowed(repository.Owner.Login) {
		return fmt.Errorf("repository owner %s not permitted by configuration", repository.Owner.Login)
	}

	converted, _, ok := s.convertGitHubRepository(*repository, true)
	if !ok {
		return fmt.Errorf("github repository %d is filtered by configuration", repoID)
	}

	forgeURL := s.githubWebBase
	if forgeURL == "" {
		forgeURL = normalizeBaseURL(s.cfg.Git.GitHub.URL, "https://github.com")
	}
	forge, err := s.ensureForge(ctx, model.ForgeTypeGithub, forgeURL)
	if err != nil {
		return err
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, []repo.GitRepository{converted}, true)
}

func (s *Service) beginGiteeAuth(ctx context.Context, redirect string) (string, string, error) {
	if s.cfg.Git.Gitee.ClientID == "" || s.cfg.Git.Gitee.ClientSecret == "" || s.cfg.Git.Gitee.RedirectURL == "" {
		return "", "", errors.New("gitee oauth configuration incomplete")
	}

	oauthCfg := s.giteeOAuthConfig()
	state, err := randomState()
	if err != nil {
		return "", "", err
	}

	encodedState, err := s.encodeState(state, redirect)
	if err != nil {
		return "", "", err
	}

	log.Debug().Str("state", state).Str("redirect", redirect).Msg("gitee oauth begin")

	authURL := oauthCfg.AuthCodeURL(encodedState, oauth2.SetAuthURLParam("scope", strings.Join(s.scopes, " ")))
	return encodedState, authURL, nil
}

func (s *Service) completeGiteeAuth(ctx context.Context, code, state string) (*AuthResponse, error) {
	if code == "" || state == "" {
		return nil, errors.New("missing code or state")
	}
	rawState, redirect, err := s.decodeState(state)
	if err != nil {
		return nil, err
	}
	log.Debug().Str("state", rawState).Msg("gitee oauth callback")

	oauthCfg := s.giteeOAuthConfig()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange gitee oauth token: %w", err)
	}

	userInfo, err := s.fetchGiteeUser(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitee, s.cfg.Git.Gitee.URL)
	if err != nil {
		return nil, err
	}

	appUser, err := s.users.UpsertGitUser(ctx, forge.ID, user.GitUser{
		RemoteID: strconv.FormatInt(userInfo.ID, 10),
		Login:    firstNonEmpty(userInfo.Login, userInfo.Name),
		Email:    userInfo.Email,
		Avatar:   userInfo.AvatarURL,
	}, token)
	if err != nil {
		return nil, err
	}

	repos, err := s.fetchGiteeRepos(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	if err := s.repos.SyncGitRepositories(ctx, forge.ID, appUser.ID, repos, false); err != nil {
		return nil, err
	}

	jwtToken, err := s.generateToken(appUser)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token:    jwtToken,
		User:     toUserInfo(appUser, providerGitee),
		Redirect: redirect,
	}, nil
}

func (s *Service) syncGiteeRepositories(ctx context.Context, userID int64) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if userModel.AccessToken == "" {
		return errors.New("user has no stored gitee token")
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitee, s.cfg.Git.Gitee.URL)
	if err != nil {
		return err
	}

	repos, err := s.fetchGiteeRepos(ctx, userModel.AccessToken)
	if err != nil {
		return err
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, repos, true)
}

func (s *Service) syncGiteeRepository(ctx context.Context, userID int64, remoteID string) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if userModel.AccessToken == "" {
		return errors.New("user has no stored gitee token")
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitee, s.cfg.Git.Gitee.URL)
	if err != nil {
		return err
	}

	repoData, err := s.fetchGiteeRepoByID(ctx, userModel.AccessToken, remoteID)
	if err != nil {
		return err
	}

	if !s.giteeOrgAllowed(repoData.Owner) {
		owner := repoData.Owner
		if strings.TrimSpace(owner) == "" {
			owner = "(unknown)"
		}
		return fmt.Errorf("gitee repository owner %s not permitted by configuration", owner)
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, []repo.GitRepository{repoData}, true)
}

func (s *Service) beginGiteaAuth(ctx context.Context, redirect string) (string, string, error) {
	if s.cfg.Git.Gitea.ClientID == "" || s.cfg.Git.Gitea.ClientSecret == "" || s.cfg.Git.Gitea.RedirectURL == "" {
		return "", "", errors.New("gitea oauth configuration incomplete")
	}

	oauthCfg := s.giteaOAuthConfig()
	state, err := randomState()
	if err != nil {
		return "", "", err
	}

	encodedState, err := s.encodeState(state, redirect)
	if err != nil {
		return "", "", err
	}

	log.Debug().Str("state", state).Str("redirect", redirect).Msg("gitea oauth begin")

	authURL := oauthCfg.AuthCodeURL(encodedState, oauth2.SetAuthURLParam("scope", strings.Join(s.scopes, " ")))
	return encodedState, authURL, nil
}

func (s *Service) completeGiteaAuth(ctx context.Context, code, state string) (*AuthResponse, error) {
	if code == "" || state == "" {
		return nil, errors.New("missing code or state")
	}
	rawState, redirect, err := s.decodeState(state)
	if err != nil {
		return nil, err
	}
	log.Debug().Str("state", rawState).Msg("gitea oauth callback")

	oauthCfg := s.giteaOAuthConfig()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange gitea oauth token: %w", err)
	}

	client, err := s.giteaClient(token.AccessToken)
	if err != nil {
		return nil, err
	}
	client.SetContext(ctx)

	gitUser, _, err := client.GetMyUserInfo()
	if err != nil {
		return nil, fmt.Errorf("fetch gitea user: %w", err)
	}

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitea, s.cfg.Git.Gitea.URL)
	if err != nil {
		return nil, err
	}

	appUser, err := s.users.UpsertGitUser(ctx, forge.ID, user.GitUser{
		RemoteID: strconv.FormatInt(gitUser.ID, 10),
		Login:    firstNonEmpty(gitUser.UserName, gitUser.FullName, gitUser.Email),
		Email:    gitUser.Email,
		Avatar:   gitUser.AvatarURL,
		IsAdmin:  gitUser.IsAdmin,
	}, token)
	if err != nil {
		return nil, err
	}

	repos, err := s.listGiteaRepositories(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := s.repos.SyncGitRepositories(ctx, forge.ID, appUser.ID, repos, false); err != nil {
		return nil, err
	}

	jwtToken, err := s.generateToken(appUser)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token:    jwtToken,
		User:     toUserInfo(appUser, providerGitea),
		Redirect: redirect,
	}, nil
}

func (s *Service) syncGiteaRepositories(ctx context.Context, userID int64) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if userModel.AccessToken == "" {
		return errors.New("user has no stored gitea token")
	}

	client, err := s.giteaClient(userModel.AccessToken)
	if err != nil {
		return err
	}
	client.SetContext(ctx)

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitea, s.cfg.Git.Gitea.URL)
	if err != nil {
		return err
	}

	repos, err := s.listGiteaRepositories(ctx, client)
	if err != nil {
		return err
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, repos, true)
}

func (s *Service) syncGiteaRepository(ctx context.Context, userID int64, remoteID string) error {
	userModel, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if userModel == nil {
		return fmt.Errorf("user %d not found", userID)
	}
	if userModel.AccessToken == "" {
		return errors.New("user has no stored gitea token")
	}

	client, err := s.giteaClient(userModel.AccessToken)
	if err != nil {
		return err
	}
	client.SetContext(ctx)

	forge, err := s.ensureForge(ctx, model.ForgeTypeGitea, s.cfg.Git.Gitea.URL)
	if err != nil {
		return err
	}

	repoID, err := strconv.ParseInt(remoteID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid repository id: %w", err)
	}

	repository, _, err := client.GetRepoByID(repoID)
	if err != nil {
		return fmt.Errorf("fetch gitea repository: %w", err)
	}

	repoData := convertGiteaRepo(repository)
	if !s.giteaOrgAllowed(repoData.Owner) {
		owner := repoData.Owner
		if strings.TrimSpace(owner) == "" {
			owner = "(unknown)"
		}
		return fmt.Errorf("gitea repository owner %s not permitted by configuration", owner)
	}

	return s.repos.SyncGitRepositories(ctx, forge.ID, userModel.ID, []repo.GitRepository{repoData}, true)
}

func (s *Service) giteaOAuthConfig() *oauth2.Config {
	base := strings.TrimSuffix(s.cfg.Git.Gitea.URL, "/")
	return &oauth2.Config{
		ClientID:     s.cfg.Git.Gitea.ClientID,
		ClientSecret: s.cfg.Git.Gitea.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		},
		RedirectURL: s.cfg.Git.Gitea.RedirectURL,
		Scopes:      s.scopes,
	}
}

func (s *Service) giteaClient(accessToken string) (*gitea.Client, error) {
	base := strings.TrimSuffix(s.cfg.Git.Gitea.URL, "/")
	client, err := gitea.NewClient(base,
		gitea.SetToken(accessToken),
		gitea.SetHTTPClient(s.httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("create gitea client: %w", err)
	}
	return client, nil
}

func (s *Service) listGiteaRepositories(ctx context.Context, client *gitea.Client) ([]repo.GitRepository, error) {
	opts := gitea.ListReposOptions{
		ListOptions: gitea.ListOptions{
			Page:     1,
			PageSize: 50,
		},
	}

	var repositories []repo.GitRepository
	for {
		items, resp, err := client.ListMyRepos(opts)
		if err != nil {
			return nil, fmt.Errorf("list gitea repositories: %w", err)
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			owner := ""
			if item.Owner != nil {
				owner = item.Owner.UserName
			}
			if !s.giteaOrgAllowed(owner) {
				continue
			}
			repositories = append(repositories, convertGiteaRepo(item))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return repositories, nil
}

func (s *Service) githubOAuthConfig() (*oauth2.Config, error) {
	if strings.TrimSpace(s.cfg.Git.GitHub.ClientID) == "" ||
		strings.TrimSpace(s.cfg.Git.GitHub.ClientSecret) == "" ||
		strings.TrimSpace(s.cfg.Git.GitHub.RedirectURL) == "" {
		return nil, errors.New("github oauth configuration incomplete")
	}

	base := normalizeBaseURL(s.cfg.Git.GitHub.URL, "https://github.com")
	endpoint := githuboauth.Endpoint
	if !strings.EqualFold(base, "https://github.com") {
		endpoint = oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		}
	}

	return &oauth2.Config{
		ClientID:     s.cfg.Git.GitHub.ClientID,
		ClientSecret: s.cfg.Git.GitHub.ClientSecret,
		Endpoint:     endpoint,
		RedirectURL:  s.cfg.Git.GitHub.RedirectURL,
		Scopes:       s.scopes,
	}, nil
}

func (s *Service) gitLabOAuthConfig() *oauth2.Config {
	base := strings.TrimSuffix(s.cfg.Git.GitLab.URL, "/")
	return &oauth2.Config{
		ClientID:     s.cfg.Git.GitLab.ClientID,
		ClientSecret: s.cfg.Git.GitLab.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  base + "/oauth/authorize",
			TokenURL: base + "/oauth/token",
		},
		RedirectURL: s.cfg.Git.GitLab.RedirectURL,
		Scopes:      s.scopes,
	}
}

func (s *Service) giteeOAuthConfig() *oauth2.Config {
	base := strings.TrimSuffix(s.cfg.Git.Gitee.URL, "/")
	return &oauth2.Config{
		ClientID:     s.cfg.Git.Gitee.ClientID,
		ClientSecret: s.cfg.Git.Gitee.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  base + "/oauth/authorize",
			TokenURL: base + "/oauth/token",
		},
		RedirectURL: s.cfg.Git.Gitee.RedirectURL,
		Scopes:      s.scopes,
	}
}

func (s *Service) gitLabClient(accessToken string) (*gitlab.Client, error) {
	base := strings.TrimSuffix(s.cfg.Git.GitLab.URL, "/")
	client, err := gitlab.NewOAuthClient(accessToken, gitlab.WithBaseURL(base), gitlab.WithHTTPClient(s.httpClient))
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}
	return client, nil
}

func (s *Service) listGitLabProjects(ctx context.Context, client *gitlab.Client) ([]repo.GitRepository, error) {
	opts := &gitlab.ListProjectsOptions{
		Membership: gitlab.Bool(true),
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var repositories []repo.GitRepository
	for {
		projects, resp, err := client.Projects.ListProjects(opts)
		if err != nil {
			return nil, fmt.Errorf("list gitlab projects: %w", err)
		}
		for _, project := range projects {
			if project == nil {
				continue
			}
			if !s.gitlabOrgAllowed(gitLabProjectNamespace(project)) {
				continue
			}

			repositories = append(repositories, convertGitLabProject(project))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return repositories, nil
}

func (s *Service) fetchGiteeUser(ctx context.Context, accessToken string) (*giteeUser, error) {
	var user giteeUser
	if err := s.giteeAPIGet(ctx, "/user", accessToken, &user); err != nil {
		return nil, err
	}
	if user.Login == "" {
		return nil, errors.New("gitee user login empty")
	}
	return &user, nil
}

func (s *Service) fetchGiteeRepos(ctx context.Context, accessToken string) ([]repo.GitRepository, error) {
	perPage := 100
	page := 1
	var repositories []repo.GitRepository

	for {
		path := fmt.Sprintf("/user/repos?page=%d&per_page=%d", page, perPage)
		var items []giteeRepo
		if err := s.giteeAPIGet(ctx, path, accessToken, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}

		for _, item := range items {
			if !s.giteeOrgAllowed(item.Owner.Login) {
				continue
			}
			repositories = append(repositories, convertGiteeRepo(item))
		}

		if len(items) < perPage {
			break
		}
		page++
	}

	return repositories, nil
}

func (s *Service) giteeAPIGet(ctx context.Context, path, accessToken string, v interface{}) error {
	base := strings.TrimSuffix(s.cfg.Git.Gitee.URL, "/")
	baseURL, err := url.Parse(base)
	if err != nil {
		return err
	}

	rel, err := url.Parse("/api/v5" + path)
	if err != nil {
		return err
	}

	apiURL := baseURL.ResolveReference(rel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitee api %s failed: %s", path, resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

func (s *Service) fetchGiteeRepoByID(ctx context.Context, accessToken, remoteID string) (repo.GitRepository, error) {
	path := fmt.Sprintf("/repositories/%s", remoteID)
	var item giteeRepo
	if err := s.giteeAPIGet(ctx, path, accessToken, &item); err != nil {
		return repo.GitRepository{}, err
	}
	return convertGiteeRepo(item), nil
}

func (s *Service) listGitHubRepositories(ctx context.Context, client *http.Client) ([]repo.GitRepository, error) {
	includeForks := s.githubIncludeForks
	seen := make(map[int64]struct{})
	repositories := make([]repo.GitRepository, 0)

	if len(s.githubOrgs) == 0 {
		params := url.Values{}
		params.Set("affiliation", "owner,organization_member")
		items, err := s.fetchGitHubReposForPath(ctx, client, "/user/repos", params)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			repo, id, ok := s.convertGitHubRepository(item, includeForks)
			if !ok || !s.githubOrgAllowed(repo.Owner) {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			repositories = append(repositories, repo)
		}
		return repositories, nil
	}

	for _, org := range s.githubOrgs {
		orgName := strings.TrimSpace(org)
		if orgName == "" {
			continue
		}
		path := fmt.Sprintf("/orgs/%s/repos", url.PathEscape(orgName))
		params := url.Values{}
		params.Set("type", "all")
		items, err := s.fetchGitHubReposForPath(ctx, client, path, params)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			repo, id, ok := s.convertGitHubRepository(item, includeForks)
			if !ok {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			repositories = append(repositories, repo)
		}
	}

	return repositories, nil
}

func (s *Service) fetchGitHubReposForPath(ctx context.Context, client *http.Client, path string, baseParams url.Values) ([]githubRepo, error) {
	const perPage = 100

	results := make([]githubRepo, 0, perPage)
	for page := 1; ; page++ {
		params := cloneValues(baseParams)
		params.Set("per_page", strconv.Itoa(perPage))
		params.Set("page", strconv.Itoa(page))

		var batch []githubRepo
		header, err := s.githubAPI(ctx, client, http.MethodGet, path, params, &batch)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		results = append(results, batch...)

		if !githubHasNextPage(header) {
			break
		}
	}

	return results, nil
}

func (s *Service) fetchGitHubRepositoryByID(ctx context.Context, client *http.Client, repoID int64) (*githubRepo, error) {
	path := fmt.Sprintf("/repositories/%d", repoID)
	var item githubRepo
	if _, err := s.githubAPI(ctx, client, http.MethodGet, path, nil, &item); err != nil {
		return nil, err
	}
	if item.ID == 0 {
		return nil, nil
	}
	return &item, nil
}

func (s *Service) githubFetchCurrentUser(ctx context.Context, client *http.Client) (*githubUser, error) {
	var user githubUser
	if _, err := s.githubAPI(ctx, client, http.MethodGet, "/user", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) githubFetchPrimaryEmail(ctx context.Context, client *http.Client) (string, error) {
	emails, err := s.githubFetchUserEmails(ctx, client)
	if err != nil {
		return "", err
	}
	var fallback string
	for _, email := range emails {
		if !email.Verified {
			continue
		}
		if email.Primary {
			return email.Email, nil
		}
		if fallback == "" {
			fallback = email.Email
		}
	}
	return fallback, nil
}

func (s *Service) githubFetchUserEmails(ctx context.Context, client *http.Client) ([]githubEmail, error) {
	var emails []githubEmail
	if _, err := s.githubAPI(ctx, client, http.MethodGet, "/user/emails", nil, &emails); err != nil {
		return nil, err
	}
	return emails, nil
}

func (s *Service) githubFetchOrgMembership(ctx context.Context, client *http.Client, org string) (*githubOrgMembership, error) {
	path := fmt.Sprintf("/user/memberships/orgs/%s", url.PathEscape(org))
	var membership githubOrgMembership
	_, err := s.githubAPI(ctx, client, http.MethodGet, path, nil, &membership)
	if err != nil {
		var apiErr *githubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &membership, nil
}

func (s *Service) githubIsOrganizationAdmin(ctx context.Context, client *http.Client) (bool, error) {
	if len(s.githubOrgs) == 0 {
		return false, nil
	}
	for _, org := range s.githubOrgs {
		name := strings.TrimSpace(org)
		if name == "" {
			continue
		}
		membership, err := s.githubFetchOrgMembership(ctx, client, name)
		if err != nil {
			return false, err
		}
		if membership == nil {
			continue
		}
		if !strings.EqualFold(membership.State, "active") {
			continue
		}
		if strings.EqualFold(membership.Role, "admin") {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) convertGitHubRepository(item githubRepo, includeForks bool) (repo.GitRepository, int64, bool) {
	if item.ID == 0 {
		return repo.GitRepository{}, 0, false
	}
	if !includeForks && item.Fork {
		return repo.GitRepository{}, 0, false
	}

	owner := strings.TrimSpace(item.Owner.Login)
	if owner == "" && strings.Contains(item.FullName, "/") {
		owner = strings.Split(item.FullName, "/")[0]
	}

	visibility := model.VisibilityPublic
	if item.Private {
		visibility = model.VisibilityPrivate
	} else if strings.EqualFold(item.Visibility, "internal") {
		visibility = model.VisibilityInternal
	}

	repository := repo.GitRepository{
		RemoteID:      strconv.FormatInt(item.ID, 10),
		Owner:         owner,
		Name:          item.Name,
		FullName:      item.FullName,
		AvatarURL:     item.Owner.AvatarURL,
		WebURL:        item.HTMLURL,
		HTTPCloneURL:  item.CloneURL,
		SSHCloneURL:   item.SSHURL,
		DefaultBranch: item.DefaultBranch,
		Visibility:    visibility,
		IsPrivate:     item.Private,
	}

	return repository, item.ID, true
}

type githubAPIError struct {
	StatusCode int
	Message    string
}

func (e *githubAPIError) Error() string {
	return e.Message
}

func (s *Service) githubAPI(ctx context.Context, client *http.Client, method, path string, params url.Values, out interface{}) (http.Header, error) {
	base := normalizeBaseURL(s.githubAPIBase, "https://api.github.com")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	endpoint := base + path
	if params != nil && len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, &githubAPIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("github api %s failed: %s: %s", path, resp.Status, strings.TrimSpace(string(body))),
		}
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return nil, err
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}

	return resp.Header, nil
}

func (s *Service) githubOrgAllowed(owner string) bool {
	return orgAllowed(owner, s.githubOrgs)
}

func (s *Service) gitlabOrgAllowed(owner string) bool {
	return orgAllowed(owner, s.gitlabOrgs)
}

func (s *Service) giteaOrgAllowed(owner string) bool {
	return orgAllowed(owner, s.giteaOrgs)
}

func (s *Service) giteeOrgAllowed(owner string) bool {
	return orgAllowed(owner, s.giteeOrgs)
}

func orgAllowed(owner string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	trimmedOwner := strings.TrimSpace(owner)
	if trimmedOwner == "" {
		return false
	}
	for _, candidate := range allowed {
		if strings.EqualFold(trimmedOwner, strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func normalizeBaseURL(raw, fallback string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = fallback
	}
	return strings.TrimSuffix(trimmed, "/")
}

func splitAndTrim(source, sep string) []string {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	parts := strings.Split(source, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func cloneValues(base url.Values) url.Values {
	if base == nil {
		return url.Values{}
	}
	cloned := make(url.Values, len(base))
	for key, values := range base {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func githubHasNextPage(header http.Header) bool {
	link := header.Get("Link")
	if link == "" {
		return false
	}
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
}

func (s *Service) generateToken(user *model.User) (string, error) {
	now := time.Now()
	claims := &SessionClaims{
		UserID: user.ID,
		Login:  user.Login,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.sessionKey)
}

func (s *Service) encodeState(state, redirect string) (string, error) {
	stateBytes := []byte(state)
	redirectBytes := []byte(redirect)

	mac := hmac.New(sha256.New, s.sessionKey)
	mac.Write(stateBytes)
	mac.Write(redirectBytes)
	sum := mac.Sum(nil)

	encoded := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString(stateBytes),
		base64.RawURLEncoding.EncodeToString(redirectBytes),
		base64.RawURLEncoding.EncodeToString(sum),
	}, ".")

	return encoded, nil
}

func (s *Service) decodeState(encoded string) (string, string, error) {
	parts := strings.Split(encoded, ".")
	if len(parts) != 3 {
		log.Warn().Str("state", encoded).Msg("oauth state malformed")
		return "", "", errors.New("invalid oauth state")
	}

	stateBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", errors.New("invalid oauth state")
	}
	redirectBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", errors.New("invalid oauth state")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", errors.New("invalid oauth state")
	}

	mac := hmac.New(sha256.New, s.sessionKey)
	mac.Write(stateBytes)
	mac.Write(redirectBytes)
	expected := mac.Sum(nil)

	if !hmac.Equal(signature, expected) {
		log.Warn().Str("state", encoded).Msg("oauth state signature mismatch")
		return "", "", errors.New("invalid oauth state")
	}

	return string(stateBytes), string(redirectBytes), nil
}

func (s *Service) ensureForge(ctx context.Context, forgeType model.ForgeType, rawURL string) (*model.Forge, error) {
	base := strings.TrimSuffix(rawURL, "/")

	var forge model.Forge
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).
			Where("type = ? AND url = ?", forgeType, base).
			Take(&forge).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		forge = model.Forge{
			Type: forgeType,
			URL:  base,
		}
		err = s.db.Transaction(func(tx *gorm.DB) error {
			return tx.WithContext(ctx).Create(&forge).Error
		})
	}
	if err != nil {
		return nil, err
	}
	return &forge, nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newHTTPClient(skipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402
	}
	return &http.Client{Transport: transport}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func convertGitLabProject(project *gitlab.Project) repo.GitRepository {
	owner := gitLabProjectNamespace(project)

	visibility := model.VisibilityPrivate
	isPrivate := true
	switch project.Visibility {
	case gitlab.PublicVisibility:
		visibility = model.VisibilityPublic
		isPrivate = false
	case gitlab.InternalVisibility:
		visibility = model.VisibilityInternal
	}

	return repo.GitRepository{
		RemoteID:      strconv.FormatInt(int64(project.ID), 10),
		Owner:         owner,
		Name:          project.Path,
		FullName:      project.PathWithNamespace,
		AvatarURL:     project.AvatarURL,
		WebURL:        project.WebURL,
		HTTPCloneURL:  project.HTTPURLToRepo,
		SSHCloneURL:   project.SSHURLToRepo,
		DefaultBranch: project.DefaultBranch,
		Visibility:    visibility,
		IsPrivate:     isPrivate,
		ConfigPath:    project.CIConfigPath,
	}
}

func gitLabProjectNamespace(project *gitlab.Project) string {
	if project == nil {
		return ""
	}
	if project.Namespace != nil {
		if path := strings.TrimSpace(project.Namespace.Path); path != "" {
			return path
		}
		if full := strings.TrimSpace(project.Namespace.FullPath); full != "" {
			parts := strings.Split(full, "/")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if name := strings.TrimSpace(project.Namespace.Name); name != "" {
			return name
		}
	}
	if combined := strings.TrimSpace(project.PathWithNamespace); combined != "" {
		parts := strings.Split(combined, "/")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	return ""
}

func convertGiteeRepo(item giteeRepo) repo.GitRepository {
	owner := item.Owner.Login
	visibility := model.VisibilityPublic
	if item.Private {
		visibility = model.VisibilityPrivate
	}

	return repo.GitRepository{
		RemoteID:      strconv.FormatInt(item.ID, 10),
		Owner:         owner,
		Name:          item.Name,
		FullName:      item.FullName,
		AvatarURL:     item.Owner.AvatarURL,
		WebURL:        item.HTMLURL,
		HTTPCloneURL:  item.CloneURL,
		SSHCloneURL:   item.SSHURL,
		DefaultBranch: item.DefaultBranch,
		Visibility:    visibility,
		IsPrivate:     item.Private,
		ConfigPath:    "",
	}
}

func convertGiteaRepo(item *gitea.Repository) repo.GitRepository {
	owner := ""
	if item.Owner != nil {
		owner = item.Owner.UserName
	}

	visibility := model.VisibilityPublic
	switch {
	case item.Private:
		visibility = model.VisibilityPrivate
	case item.Internal:
		visibility = model.VisibilityInternal
	}

	return repo.GitRepository{
		RemoteID:      strconv.FormatInt(item.ID, 10),
		Owner:         owner,
		Name:          item.Name,
		FullName:      item.FullName,
		AvatarURL:     item.AvatarURL,
		WebURL:        item.HTMLURL,
		HTTPCloneURL:  item.CloneURL,
		SSHCloneURL:   item.SSHURL,
		DefaultBranch: item.DefaultBranch,
		Visibility:    visibility,
		IsPrivate:     item.Private,
		ConfigPath:    "",
	}
}
