package routers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/label"
	"github.com/thepenn/devsys/model"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
)

var (
	errSystemServiceUnavailable = errors.New("system service unavailable")
	errInvalidCertificateID     = errors.New("certificate id is invalid")
)

type systemRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newSystemRouter(services *service.Services, authMW *authmw.Middleware) *systemRouter {
	return &systemRouter{
		services: services,
		authMW:   authMW,
	}
}

func (r *systemRouter) router(register func(path string) *restful.WebService, tags []string) []*restful.WebService {
	var webServices []*restful.WebService

	webServices = append(webServices, r.registerRSA(register, tags))

	if ws := r.registerCertificateRoutes(register, tags); ws != nil {
		webServices = append(webServices, ws)
	}

	return webServices
}

func (r *systemRouter) registerRSA(register func(path string) *restful.WebService, tags []string) *restful.WebService {
	ws := register("/sys/rsa")
	ws.Route(ws.GET("/public-key").To(r.getPublicKey).
		Doc("获取RSA公钥").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Produces(restful.MIME_JSON).
		Returns(http.StatusOK, "OK", map[string]string{}).
		Returns(http.StatusInternalServerError, "error", nil))
	return ws
}

func (r *systemRouter) registerCertificateRoutes(register func(path string) *restful.WebService, tags []string) *restful.WebService {
	if r.services == nil || r.services.System == nil || r.services.User == nil || r.authMW == nil {
		return nil
	}

	ws := register("/sys/certificates")
	ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)
	ws.Filter(r.authMW.RequireAuth)

	certLabels := []string{label.SystemCertificate}

	ws.Route(ws.GET("").To(r.listCertificates).
		Doc("列出凭证").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, certLabels).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes(certificateListResponse{}).
		Returns(http.StatusOK, "OK", certificateListResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.POST("").To(r.createCertificate).
		Doc("创建凭证").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, certLabels).
		Metadata(label.MetaModule, label.ModuleSystem).
		Reads(certificateCreateRequest{}).
		Writes(certificateResponse{}).
		Returns(http.StatusCreated, "created", certificateResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.GET("/{id}").To(r.getCertificate).
		Doc("获取凭证详情").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, certLabels).
		Metadata(label.MetaModule, label.ModuleSystem).
		Writes(certificateResponse{}).
		Returns(http.StatusOK, "OK", certificateResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.PUT("/{id}").To(r.updateCertificate).
		Doc("更新凭证").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, certLabels).
		Metadata(label.MetaModule, label.ModuleSystem).
		Reads(certificateUpdateRequest{}).
		Writes(certificateResponse{}).
		Returns(http.StatusOK, "OK", certificateResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	ws.Route(ws.DELETE("/{id}").To(r.deleteCertificate).
		Doc("删除凭证").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, certLabels).
		Metadata(label.MetaModule, label.ModuleSystem).
		Returns(http.StatusNoContent, "deleted", nil).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}).
		Returns(http.StatusNotFound, "not found", errorResponse{}).
		Returns(http.StatusInternalServerError, "error", errorResponse{}))

	return ws
}

func (r *systemRouter) getPublicKey(req *restful.Request, resp *restful.Response) {
	if r.services == nil || r.services.System == nil {
		writeError(resp, http.StatusInternalServerError, errSystemServiceUnavailable)
		return
	}
	key, err := r.services.System.GetPublicKey(req.Request.Context())
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, map[string]string{"public_key": key})
}

func (r *systemRouter) listCertificates(req *restful.Request, resp *restful.Response) {
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	opts := model.ListOptions{Page: page, PerPage: perPage}

	filter := model.CertificateFilter{
		Type: req.QueryParameter("type"),
		Name: req.QueryParameter("name"),
	}

	certs, total, err := r.services.System.ListCertificates(req.Request.Context(), opts, filter)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	items := make([]certificateResponse, 0, len(certs))
	for _, cert := range certs {
		items = append(items, newCertificateResponse(cert))
	}

	result := certificateListResponse{
		Items:   items,
		Page:    page,
		PerPage: perPage,
		Total:   total,
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, result)
}

func (r *systemRouter) createCertificate(req *restful.Request, resp *restful.Response) {
	var body certificateCreateRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	cert := &model.Certificate{
		Name:   strings.TrimSpace(body.Name),
		Type:   strings.TrimSpace(body.Type),
		Config: body.Config,
	}

	created, err := r.services.System.CreateCertificate(req.Request.Context(), cert)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "required") {
			status = http.StatusBadRequest
		}
		writeError(resp, status, err)
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusCreated, newCertificateResponse(created))
}

func (r *systemRouter) getCertificate(req *restful.Request, resp *restful.Response) {
	id, err := r.certificateID(req)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	var (
		cert   *model.Certificate
		reveal = strings.EqualFold(req.QueryParameter("reveal"), "true")
	)
	if reveal {
		cert, err = r.services.System.GetCertificateWithSecrets(req.Request.Context(), id)
	} else {
		cert, err = r.services.System.GetCertificate(req.Request.Context(), id)
	}
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	if cert == nil {
		writeError(resp, http.StatusNotFound, gorm.ErrRecordNotFound)
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, newCertificateResponse(cert))
}

func (r *systemRouter) updateCertificate(req *restful.Request, resp *restful.Response) {
	id, err := r.certificateID(req)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	var body certificateUpdateRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	patch := model.CertificatePatch{
		Config: body.Config,
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		patch.Name = &name
	}
	if body.Type != nil {
		typ := strings.TrimSpace(*body.Type)
		patch.Type = &typ
	}

	updated, err := r.services.System.UpdateCertificate(req.Request.Context(), id, patch)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(resp, http.StatusNotFound, err)
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "required") {
			status = http.StatusBadRequest
		}
		writeError(resp, status, err)
		return
	}
	if updated == nil {
		writeError(resp, http.StatusNotFound, gorm.ErrRecordNotFound)
		return
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, newCertificateResponse(updated))
}

func (r *systemRouter) deleteCertificate(req *restful.Request, resp *restful.Response) {
	id, err := r.certificateID(req)
	if err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}

	err = r.services.System.DeleteCertificate(req.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(resp, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}

	resp.WriteHeader(http.StatusNoContent)
}

func (r *systemRouter) certificateID(req *restful.Request) (int64, error) {
	raw := req.PathParameter("id")
	if raw == "" {
		return 0, errInvalidCertificateID
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errInvalidCertificateID
	}
	return id, nil
}

type certificateCreateRequest struct {
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

type certificateUpdateRequest struct {
	Name   *string                `json:"name,omitempty"`
	Type   *string                `json:"type,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}

type certificateResponse struct {
	ID           int64                  `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Config       map[string]interface{} `json:"config"`
	MaskedFields []string               `json:"masked_fields"`
	Created      int64                  `json:"created"`
	Updated      int64                  `json:"updated"`
}

type certificateListResponse struct {
	Items   []certificateResponse `json:"items"`
	Page    int                   `json:"page"`
	PerPage int                   `json:"per_page"`
	Total   int64                 `json:"total"`
}

func newCertificateResponse(cert *model.Certificate) certificateResponse {
	maskedConfig, maskedKeys := cert.MaskSecrets(model.DefaultSecretMask)
	return certificateResponse{
		ID:           cert.ID,
		Name:         cert.Name,
		Type:         cert.Type,
		Config:       maskedConfig,
		MaskedFields: maskedKeys,
		Created:      cert.Created,
		Updated:      cert.Updated,
	}
}
