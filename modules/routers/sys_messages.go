package routers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/internal/label"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
	messageService "github.com/thepenn/devsys/service/message"
)

type messagesRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newMessagesRouter(services *service.Services, authMW *authmw.Middleware) *messagesRouter {
	return &messagesRouter{services: services, authMW: authMW}
}

type markReadRequest struct {
	IDs []int64 `json:"ids"`
}

type markReadResponse struct {
	Affected int64 `json:"affected"`
}

type unreadCountResponse struct {
	Count int64 `json:"count"`
}

func (r *messagesRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	if r.services == nil || r.services.Message == nil {
		return nil
	}

	ws := register("/messages")
	ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)
	ws.Filter(r.authMW.Authenticate)
	ws.Filter(r.authMW.RequireAuth)

	// 列表 / 标读 走 message:read label, 是"读自己消息"的能力, 默认所有
	// 角色都给 (guest 也行); 但 ACL 中间件仍会检查角色是否拥有该 label.
	read := []string{label.MessageRead}

	ws.Route(ws.GET("").To(r.list).
		Doc("列出当前用户的消息").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleMessage).
		Writes(messageService.ListResult{}).
		Returns(http.StatusOK, "OK", messageService.ListResult{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}).
		Returns(http.StatusForbidden, "forbidden", errorResponse{}))

	ws.Route(ws.GET("/unread-count").To(r.unreadCount).
		Doc("当前用户未读消息数, 用于全局头部 badge").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		// 不挂 ACL: 仅返回当前用户自己的 count, 没有泄漏其它用户数据风险.
		Writes(unreadCountResponse{}).
		Returns(http.StatusOK, "OK", unreadCountResponse{}).
		Returns(http.StatusUnauthorized, "unauthorized", errorResponse{}))

	ws.Route(ws.POST("/read").To(r.markRead).
		Doc("批量标记已读 (按 id 列表)").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleMessage).
		Reads(markReadRequest{}).
		Writes(markReadResponse{}).
		Returns(http.StatusOK, "OK", markReadResponse{}).
		Returns(http.StatusBadRequest, "bad request", errorResponse{}))

	ws.Route(ws.POST("/read-all").To(r.markAllRead).
		Doc("把所有未读标记已读").
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(label.MetaACL, true).
		Metadata(label.MetaLabels, read).
		Metadata(label.MetaModule, label.ModuleMessage).
		Writes(markReadResponse{}).
		Returns(http.StatusOK, "OK", markReadResponse{}))

	return []*restful.WebService{ws}
}

func (r *messagesRouter) list(req *restful.Request, resp *restful.Response) {
	uid, ok := r.currentUserID(req, resp)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("per_page"))
	unread := strings.EqualFold(req.QueryParameter("unread"), "true")
	result, err := r.services.Message.List(req.Request.Context(), uid, messageService.ListOptions{
		Page:       page,
		PerPage:    perPage,
		UnreadOnly: unread,
	})
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, result)
}

func (r *messagesRouter) unreadCount(req *restful.Request, resp *restful.Response) {
	uid, ok := r.currentUserID(req, resp)
	if !ok {
		return
	}
	count, err := r.services.Message.UnreadCount(req.Request.Context(), uid)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, unreadCountResponse{Count: count})
}

func (r *messagesRouter) markRead(req *restful.Request, resp *restful.Response) {
	uid, ok := r.currentUserID(req, resp)
	if !ok {
		return
	}
	var body markReadRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if len(body.IDs) == 0 {
		_ = resp.WriteHeaderAndEntity(http.StatusOK, markReadResponse{Affected: 0})
		return
	}
	n, err := r.services.Message.MarkRead(req.Request.Context(), uid, body.IDs)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, markReadResponse{Affected: n})
}

func (r *messagesRouter) markAllRead(req *restful.Request, resp *restful.Response) {
	uid, ok := r.currentUserID(req, resp)
	if !ok {
		return
	}
	n, err := r.services.Message.MarkAllRead(req.Request.Context(), uid)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteHeaderAndEntity(http.StatusOK, markReadResponse{Affected: n})
}

// currentUserID 抽出公共的 401 处理. 写错误后返回 false 让 caller 直接 return.
func (r *messagesRouter) currentUserID(req *restful.Request, resp *restful.Response) (int64, bool) {
	claims, ok := authmw.FromContext(req.Request.Context())
	if !ok || claims == nil {
		writeError(resp, http.StatusUnauthorized, errors.New("unauthorized"))
		return 0, false
	}
	return claims.UserID, true
}
