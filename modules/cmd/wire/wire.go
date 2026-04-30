//go:build wireinject
// +build wireinject

package wire

import (
	"time"

	"github.com/google/wire"

	"github.com/thepenn/devsys/internal/cache"
	"github.com/thepenn/devsys/internal/config"
	"github.com/thepenn/devsys/internal/handler"
	routersync "github.com/thepenn/devsys/internal/router"
	"github.com/thepenn/devsys/internal/server"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/routers"
	aclmw "github.com/thepenn/devsys/routers/middleware/acl"
	auditmw "github.com/thepenn/devsys/routers/middleware/audit"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	corsmw "github.com/thepenn/devsys/routers/middleware/cors"
	"github.com/thepenn/devsys/routers/middleware/metrics"
	"github.com/thepenn/devsys/service"
	"github.com/thepenn/devsys/service/migrate"
	"github.com/thepenn/devsys/service/pipeline/queue"
)

type App struct {
	HttpServer *server.HttpServer
	Services   *service.Services
	DB         *store.DB
	Cache      *cache.Cache
}

// NewApp 创建应用实例
func NewApp(httpServer *server.HttpServer, services *service.Services, db *store.DB, cache *cache.Cache) *App {
	return &App{
		HttpServer: httpServer,
		Services:   services,
		DB:         db,
		Cache:      cache,
	}
}

var appSet = wire.NewSet(
	InjectedRouters,
	InjectedHandler,
	InjectedHttpServer,
	InjectedDatabase,
	InjectedCache,
	InjectedQueue,
	InjectedServices,
	InjectedMetricsMiddleware,
	InjectedCorsMiddleware,
	InjectedAuthMiddleware,
	InjectedACLMiddleware,
	InjectedAuditMiddleware,
	InjectedRouterSync,
	NewApp,
)

func InjectedRouters(cfg *config.Config, services *service.Services, authMiddleware *authmw.Middleware) *routers.Routers {
	return routers.NewRouters(cfg, services, authMiddleware)
}

func InjectedHandler(
	cfg *config.Config,
	routers *routers.Routers,
	authMiddleware *authmw.Middleware,
	aclMiddleware *aclmw.Middleware,
	auditMiddleware *auditmw.Middleware,
	metric *metrics.Middleware,
	routerSync *routersync.Sync,
) *handler.Handler {
	return handler.NewHandler(
		handler.WithConfig(cfg.Server.Host, cfg.Server.RootPath),
		handler.WithRegisterControllers(routers),
		// 中间件链 auth -> acl -> audit -> metrics:
		//   1. auth: 解析 JWT 注入 SessionClaims (audit 也读它写 user 字段)
		//   2. acl:  按 Metadata.labels 做 RBAC 校验, 拒绝未授权请求
		//   3. audit: 把放行的非 GET 请求落 audit_logs (异步)
		//   4. metrics: Prometheus 计时
		handler.WithRegisterMiddlewares(authMiddleware),
		handler.WithRegisterMiddlewares(aclMiddleware),
		handler.WithRegisterMiddlewares(auditMiddleware),
		handler.WithRegisterMiddlewares(metric),
		handler.WithStorageRouter(routerSync),
	)
}

func InjectedHttpServer(cfg *config.Config, corsMiddleware *corsmw.Middleware, h *handler.Handler) *server.HttpServer {
	return server.NewHttpServer(cfg.Server.Host, corsMiddleware.WrapHTTP(h.Handler()))
}

func InjectedDatabase(cfg *config.Config) (*store.DB, error) {
	db, err := store.ConnectWithDriver(cfg.Database.Driver, cfg.Database.Datasource, cfg.Database.MaxConnections, cfg.Database.ShowSql)
	if err != nil {
		return nil, err
	}
	if err := migrate.AutoMigrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func InjectedCache() *cache.Cache {
	return cache.New(5 * time.Minute)
}

func InjectedQueue(cfg *config.Config) *queue.PipelineQueue {
	return queue.New(cfg.Pipeline.QueueCapacity)
}

func InjectedServices(db *store.DB, q *queue.PipelineQueue, cache *cache.Cache, cfg *config.Config) (*service.Services, error) {
	return service.NewServices(db, q, cache, cfg)
}

func InjectedMetricsMiddleware() *metrics.Middleware {
	return metrics.New()
}

func InjectedCorsMiddleware() *corsmw.Middleware {
	return corsmw.New()
}

func InjectedAuthMiddleware(services *service.Services) *authmw.Middleware {
	return authmw.New(services.Auth)
}

func InjectedACLMiddleware(services *service.Services) *aclmw.Middleware {
	return aclmw.New(services.User, services.RBACEng)
}

func InjectedAuditMiddleware(services *service.Services) *auditmw.Middleware {
	return auditmw.New(services.Audit)
}

func InjectedRouterSync(db *store.DB) *routersync.Sync {
	return routersync.New(db)
}

func WireApp(cfg *config.Config) (*App, error) {
	wire.Build(appSet)
	return nil, nil
}
