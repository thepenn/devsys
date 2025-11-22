//go:build wireinject
// +build wireinject

package wire

import (
	"time"

	"github.com/google/wire"

	"github.com/thepenn/devsys/internal/cache"
	"github.com/thepenn/devsys/internal/config"
	"github.com/thepenn/devsys/internal/handler"
	"github.com/thepenn/devsys/internal/server"
	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/routers"
	adminmw "github.com/thepenn/devsys/routers/middleware/admin"
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
	InjectedAdminMiddleware,
	InjectedAuthMiddleware,
	NewApp,
)

func InjectedRouters(cfg *config.Config, services *service.Services, authMiddleware *authmw.Middleware) *routers.Routers {
	return routers.NewRouters(cfg, services, authMiddleware)
}

func InjectedHandler(cfg *config.Config, routers *routers.Routers, authMiddleware *authmw.Middleware, adminMiddleware *adminmw.Middleware, metric *metrics.Middleware) *handler.Handler {
	return handler.NewHandler(
		handler.WithConfig(cfg.Server.Host, cfg.Server.RootPath),
		handler.WithRegisterControllers(routers),
		handler.WithRegisterMiddlewares(authMiddleware),
		handler.WithRegisterMiddlewares(adminMiddleware),
		handler.WithRegisterMiddlewares(metric),
	)
}

func InjectedHttpServer(cfg *config.Config, corsMiddleware *corsmw.Middleware, h *handler.Handler) *server.HttpServer {
	return server.NewHttpServer(cfg.Server.Host, corsMiddleware.WrapHTTP(h.Handler()))
}

func InjectedDatabase(cfg *config.Config) (*store.DB, error) {
	db, err := store.Connect(cfg.Database.Datasource, cfg.Database.MaxConnections, cfg.Database.ShowSql)
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

func InjectedAdminMiddleware(services *service.Services) *adminmw.Middleware {
	return adminmw.New(services.User)
}

func InjectedAuthMiddleware(services *service.Services) *authmw.Middleware {
	return authmw.New(services.Auth)
}

func WireApp(cfg *config.Config) (*App, error) {
	wire.Build(appSet)
	return nil, nil
}
