package routers

import (
	"fmt"
	"net/http"
	"time"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/thepenn/devsys/routers/middleware/metrics"
	"github.com/thepenn/devsys/service"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	healthStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "app_health_status",
			Help: "Application health status (1 = healthy, 0 = unhealthy)",
		},
	)
)

type health struct {
	startTime time.Time
	services  *service.Services
}

func newHealth(services *service.Services) *health {
	return &health{
		startTime: time.Now(),
		services:  services,
	}
}

func (h *health) router(register func(path string) *restful.WebService, tags []string) []*restful.WebService {
	pingWs := register("").Path("/ping")
	pingWs.Route(pingWs.GET("").To(h.ping).Doc("ping").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Returns(200, "OK", nil).
		Returns(404, "Not Found", nil))

	healthyWs := register("").Path("/health")
	healthyWs.Route(healthyWs.GET("").To(h.healthy).Doc("health check").
		Returns(200, "OK", nil).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Returns(503, "Service Unavailable", nil))

	metricsWs := register("").Path("/metrics")
	metricsWs.Route(metricsWs.GET("").To(h.metrics).Doc("metrics").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Returns(200, "OK", nil))

	return []*restful.WebService{
		pingWs,
		healthyWs,
		metricsWs,
	}
}

func (h *health) ping(req *restful.Request, resp *restful.Response) {
	_ = resp.WriteHeaderAndEntity(http.StatusOK, map[string]string{"message": "pong"})
}

func (h *health) healthy(req *restful.Request, resp *restful.Response) {
	status := "healthy"
	httpStatus := http.StatusOK

	if h.services != nil && h.services.DB != nil {
		sqlDB, err := h.services.DB.GetDB().DB()
		if err != nil || sqlDB.PingContext(req.Request.Context()) != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	if httpStatus == http.StatusOK {
		healthStatus.Set(1)
	} else {
		healthStatus.Set(0)
	}

	data := map[string]interface{}{
		"status":    status,
		"uptime":    time.Since(h.startTime).String(),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	statusStr := fmt.Sprintf("%d", httpStatus)
	httpRequestsTotal.WithLabelValues("GET", "/health", statusStr).Inc()
	startTime, ok := metrics.StartTimeFromContext(req.Request.Context())
	if ok {
		httpRequestDuration.WithLabelValues("GET", "/health").Observe(time.Since(startTime).Seconds())
	}

	_ = resp.WriteHeaderAndEntity(httpStatus, data)
}

func (h *health) metrics(req *restful.Request, resp *restful.Response) {
	httpRequestsTotal.WithLabelValues("GET", "/metrics", "200").Inc()
	startTime, ok := metrics.StartTimeFromContext(req.Request.Context())
	if ok {
		httpRequestDuration.WithLabelValues("GET", "/metrics").Observe(time.Since(startTime).Seconds())
	}

	promhttp.Handler().ServeHTTP(resp.ResponseWriter, req.Request)
}
