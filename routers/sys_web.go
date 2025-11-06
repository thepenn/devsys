package routers

import (
	"net/http"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"

	"github.com/kuzane/go-devops/web"
)

type webHandler struct{}

func (r *webHandler) router(register func(path string) *restful.WebService, tags []string) []*restful.WebService {
	// 注册静态文件服务
	staticWs := register("").Path("/static")
	staticWs.Route(staticWs.GET("/{filename:*}").
		To(r.static).Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Doc("静态文件").
		Returns(200, "OK", nil).
		Returns(404, "Not Found", nil))

	// 注册根路径服务
	rootWs := register("").Path("/")
	rootWs.Route(rootWs.GET("").To(r.defaultStatic).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Doc("web").
		Returns(200, "OK", nil).
		Returns(404, "Not Found", nil))

	return []*restful.WebService{staticWs, rootWs}
}

func (r *webHandler) static(req *restful.Request, resp *restful.Response) {
	filePath := req.PathParameter("filename")
	buf, err := web.Lookup(filePath)
	if err != nil {
		resp.WriteHeader(http.StatusNotFound)
		return
	}

	if strings.HasSuffix(filePath, ".html") {
		resp.Header().Set("Content-Type", "text/html")
	} else if strings.HasSuffix(filePath, ".js") {
		resp.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(filePath, ".css") {
		resp.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".png") {
		resp.Header().Set("Content-Type", "image/png")
	}

	if _, err := resp.Write(buf); err != nil {
		log.Error().Err(err).Msg("response write err")
	}
}

func (r *webHandler) defaultStatic(req *restful.Request, resp *restful.Response) {
	buf, err := web.Lookup("index.html")
	if err != nil {
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	resp.Header().Set("Content-Type", "text/html")
	if _, err := resp.Write(buf); err != nil {
		log.Error().Err(err).Msg("response write err")
	}
}
