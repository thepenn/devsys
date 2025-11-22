package handler

import (
	"net/http"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/go-openapi/spec"
	"github.com/rs/zerolog/log"
)

type RegisterRouter interface {
	Router(func(string) *restful.WebService) []*restful.WebService
}

type RegisterMiddleware interface {
	Middleware() []restful.FilterFunction
}

type StorageRouter interface {
	StoreRouter(container *restful.Container)
}

type Handler struct {
	Host          string
	APIPath       string
	controllers   []RegisterRouter
	middlewares   []RegisterMiddleware
	storageRouter StorageRouter
}

type Option func(*Handler)

func WithRegisterControllers(opts RegisterRouter) Option {
	return func(h *Handler) {
		h.controllers = append(h.controllers, opts)
	}
}

func WithRegisterMiddlewares(opts RegisterMiddleware) Option {
	return func(h *Handler) {
		h.middlewares = append(h.middlewares, opts)
	}
}

func WithConfig(host, apiPath string) Option {
	return func(handler *Handler) {
		handler.Host = host
		handler.APIPath = apiPath
	}
}

func WithStorageRouter(opts StorageRouter) Option {
	return func(handler *Handler) {
		handler.storageRouter = opts
	}
}

func NewHandler(opts ...Option) *Handler {
	h := &Handler{}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

func (h *Handler) Handler() http.Handler {
	var defaultContainer = restful.DefaultContainer

	// 注册全局中间件
	for _, middleware := range h.middlewares {
		for _, filter := range middleware.Middleware() {
			defaultContainer.Filter(filter)
		}
	}

	for _, controller := range h.controllers {
		webServices := controller.Router(func(modulePath string) *restful.WebService {
			return h.initWebService(h.APIPath + modulePath)
		})

		for _, ws := range webServices {
			defaultContainer.Add(ws)
		}
	}

	if h.storageRouter != nil {
		h.storageRouter.StoreRouter(defaultContainer)
	}

	c := restfulspec.Config{
		WebServices: defaultContainer.RegisteredWebServices(),
		APIPath:     "/api.json",
		PostBuildSwaggerObjectHandler: func(s *spec.Swagger) {
			s.Host = h.Host
			s.Schemes = []string{"http"}
		},
	}
	defaultContainer.Add(restfulspec.NewOpenAPIService(c))
	log.Info().Msgf("Api docs: http://%s%s", h.Host, c.APIPath)

	return defaultContainer
}

func (h *Handler) initWebService(fullPath string) *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(fullPath).Consumes(restful.MIME_JSON, "text/plain", "*/*").Produces(restful.MIME_JSON, "text/plain", "*/*")

	return ws
}
