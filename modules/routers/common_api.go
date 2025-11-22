package routers

import (
	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/internal/config"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
)

type Routers struct {
	health   *health
	web      *webHandler
	auth     *authRouter
	repos    *repoRouter
	system   *systemRouter
	k8s      *k8sRouter
	services *service.Services
	cfg      *config.Config
}

func NewRouters(cfg *config.Config, services *service.Services, authMW *authmw.Middleware) *Routers {
	return &Routers{
		health:   &health{},
		web:      &webHandler{},
		auth:     newAuthRouter(services, authMW),
		repos:    newRepoRouter(services, authMW),
		k8s:      newK8sRouter(services, authMW),
		system:   newSystemRouter(services, authMW),
		services: services,
		cfg:      cfg,
	}
}

func (r *Routers) Router(register func(string) *restful.WebService) []*restful.WebService {
	var ws []*restful.WebService

	{
		sysTags := []string{"系统"}
		ws = append(ws, r.health.router(register, sysTags)...)
		ws = append(ws, r.web.router(register, sysTags)...)
		ws = append(ws, r.system.router(register, sysTags)...)
	}

	{
		authTags := []string{"认证"}
		ws = append(ws, r.auth.router(register, authTags)...)
	}

	{
		repoTags := []string{"仓库"}
		ws = append(ws, r.repos.router(register, repoTags)...)
	}

	{
		adminTags := []string{"Kubernetes"}
		ws = append(ws, r.k8s.router(register, adminTags)...)
	}

	return ws
}
