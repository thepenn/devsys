package routers

import (
	"net/http"
	"path"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"

	"github.com/thepenn/devsys/web"
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
	rootWs.Route(rootWs.GET("/favicon.ico").To(r.favicon).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Doc("favicon").
		Returns(200, "OK", nil).
		Returns(404, "Not Found", nil))

	return []*restful.WebService{staticWs, rootWs}
}

func (r *webHandler) static(req *restful.Request, resp *restful.Response) {
	filePath := strings.TrimPrefix(req.PathParameter("filename"), "/")
	if filePath == "" {
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	cleanPath := path.Clean(filePath)
	if strings.HasPrefix(cleanPath, "..") {
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	lookupPath := path.Join("static", cleanPath)

	buf, err := web.Lookup(lookupPath)
	if err != nil {
		resp.WriteHeader(http.StatusNotFound)
		return
	}

	if strings.HasSuffix(lookupPath, ".html") {
		resp.Header().Set("Content-Type", "text/html")
	} else if strings.HasSuffix(lookupPath, ".js") {
		resp.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(lookupPath, ".css") {
		resp.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(lookupPath, ".jpg") || strings.HasSuffix(lookupPath, ".png") {
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

func (r *webHandler) favicon(req *restful.Request, resp *restful.Response) {
	buf, err := web.Lookup("favicon.ico")
	if err != nil {
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	resp.Header().Set("Content-Type", "image/x-icon")
	if _, err := resp.Write(buf); err != nil {
		log.Error().Err(err).Msg("response write err")
	}
}
