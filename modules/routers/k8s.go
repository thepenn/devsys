package routers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	restfulOpenapi "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/gorilla/websocket"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/thepenn/devsys/model"
	adminmw "github.com/thepenn/devsys/routers/middleware/admin"
	authmw "github.com/thepenn/devsys/routers/middleware/auth"
	"github.com/thepenn/devsys/service"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type k8sRouter struct {
	services *service.Services
	authMW   *authmw.Middleware
}

func newK8sRouter(services *service.Services, authMW *authmw.Middleware) *k8sRouter {
	return &k8sRouter{services: services, authMW: authMW}
}

func (r *k8sRouter) router(register func(string) *restful.WebService, tags []string) []*restful.WebService {
	ws := register("/admin/k8s")
	ws.Filter(r.authMW.Authenticate)

	ws.Route(ws.GET("/clusters").To(r.listClusters).
		Doc("List kubernetes clusters").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]model.KubernetesClusterSummary{}).
		Returns(http.StatusOK, "clusters", []model.KubernetesClusterSummary{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/namespaces").To(r.listNamespaces).
		Doc("List namespaces for a cluster").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]model.KubernetesNamespace{}).
		Returns(http.StatusOK, "namespaces", []model.KubernetesNamespace{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/resources").To(r.listResources).
		Doc("List resources for a cluster").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]map[string]interface{}{}).
		Returns(http.StatusOK, "resources", []map[string]interface{}{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/resources/object").To(r.getResource).
		Doc("Get single resource").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes(model.KubernetesObjectResponse{}).
		Returns(http.StatusOK, "resource", model.KubernetesObjectResponse{}))

	ws.Route(ws.POST("/clusters/{cluster_id}/resources/apply").To(r.applyManifest).
		Doc("Apply manifest").
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Reads(model.KubernetesManifestRequest{}).
		Writes(model.KubernetesObjectResponse{}).
		Returns(http.StatusOK, "resource", model.KubernetesObjectResponse{}))

	ws.Route(ws.DELETE("/clusters/{cluster_id}/resources/object").To(r.deleteResource).
		Doc("Delete resource").
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Reads(model.KubernetesResourceDeleteRequest{}).
		Returns(http.StatusNoContent, "deleted", nil))

	ws.Route(ws.GET("/clusters/{cluster_id}/deployments/{namespace}/{name}/aggregate").To(r.aggregateDeployment).
		Doc("Aggregate deployment with related resources").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]model.KubernetesObjectResponse{}).
		Returns(http.StatusOK, "aggregate", []model.KubernetesObjectResponse{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/deployments/{namespace}/{name}/pods").To(r.listDeploymentPods).
		Doc("List pods for deployment").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]model.KubernetesPodSummary{}).
		Returns(http.StatusOK, "pods", []model.KubernetesPodSummary{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/workloads/{kind}/{namespace}/{name}/pods").To(r.listWorkloadPods).
		Doc("List pods for a workload").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]model.KubernetesPodRow{}).
		Returns(http.StatusOK, "pods", []model.KubernetesPodRow{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/workloads/{kind}/{namespace}/{name}/details").To(r.workloadDetails).
		Doc("Get workload related resources").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes(model.KubernetesWorkloadDetails{}).
		Returns(http.StatusOK, "details", model.KubernetesWorkloadDetails{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/workloads/{kind}/{namespace}/{name}/history").To(r.workloadHistory).
		Doc("Get workload history").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes([]model.KubernetesWorkloadHistoryEntry{}).
		Returns(http.StatusOK, "history", []model.KubernetesWorkloadHistoryEntry{}))

	ws.Route(ws.POST("/clusters/{cluster_id}/workloads/{kind}/{namespace}/{name}/rollback").To(r.workloadRollback).
		Doc("Rollback workload to revision").
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Reads(model.KubernetesWorkloadRollbackRequest{}).
		Returns(http.StatusNoContent, "rolled back", nil))

	ws.Route(ws.GET("/clusters/{cluster_id}/workloads/{kind}/{namespace}/{name}/logs").To(r.workloadLogs).
		Doc("Aggregate logs for workload").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes(model.KubernetesLogResponse{}).
		Returns(http.StatusOK, "logs", model.KubernetesLogResponse{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/resources/events").To(r.listEvents).
		Doc("List events for resource").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes(model.KubernetesEventPage{}).
		Returns(http.StatusOK, "events", model.KubernetesEventPage{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/pods/logs").To(r.podLogs).
		Doc("Fetch pod logs").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Writes(model.KubernetesLogResponse{}).
		Returns(http.StatusOK, "logs", model.KubernetesLogResponse{}))

	ws.Route(ws.POST("/clusters/{cluster_id}/pods/{namespace}/{name}/exec").To(r.execPod).
		Doc("Execute a command in pod").
		Filter(r.authMW.RequireAuth).
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Reads(model.KubernetesPodExecRequest{}).
		Writes(model.KubernetesPodExecResult{}).
		Returns(http.StatusOK, "output", model.KubernetesPodExecResult{}))

	ws.Route(ws.GET("/clusters/{cluster_id}/pods/{namespace}/{name}/exec/stream").To(r.execPodStream).
		Doc("Websocket interactive exec").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Produces(restful.MIME_OCTET).
		Returns(http.StatusSwitchingProtocols, "stream", nil))

	ws.Route(ws.GET("/clusters/{cluster_id}/pods/{namespace}/{name}/logs/stream").To(r.podLogsStream).
		Doc("Stream pod logs via websocket").
		Filter(r.authMW.RequireAuth).
		Metadata(restfulOpenapi.KeyOpenAPITags, tags).
		Metadata(adminmw.AdminEnable, true).
		Produces(restful.MIME_OCTET).
		Returns(http.StatusSwitchingProtocols, "stream", nil))

	return []*restful.WebService{ws}
}

func (r *k8sRouter) listClusters(req *restful.Request, resp *restful.Response) {
	list, err := r.services.K8s.ListClusters(req.Request.Context())
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(list)
}

func (r *k8sRouter) listNamespaces(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	list, err := r.services.K8s.ListNamespaces(req.Request.Context(), clusterID)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(list)
}

func (r *k8sRouter) listResources(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	query := model.KubernetesResourceQuery{
		Group:         req.QueryParameter("group"),
		Version:       req.QueryParameter("version"),
		Resource:      req.QueryParameter("resource"),
		Namespace:     req.QueryParameter("namespace"),
		LabelSelector: req.QueryParameter("labelSelector"),
		FieldSelector: req.QueryParameter("fieldSelector"),
	}
	if strings.TrimSpace(query.Resource) == "" {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("resource is required"))
		return
	}
	list, err := r.services.K8s.ListResources(req.Request.Context(), clusterID, query)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(list)
}

func (r *k8sRouter) getResource(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	query := model.KubernetesResourceQuery{
		Group:     req.QueryParameter("group"),
		Version:   req.QueryParameter("version"),
		Resource:  req.QueryParameter("resource"),
		Namespace: req.QueryParameter("namespace"),
		Name:      req.QueryParameter("name"),
	}
	result, err := r.services.K8s.GetResource(req.Request.Context(), clusterID, query)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			writeError(resp, http.StatusNotFound, err)
			return
		}
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(result)
}

func (r *k8sRouter) applyManifest(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	var body model.KubernetesManifestRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	result, err := r.services.K8s.ApplyManifest(req.Request.Context(), clusterID, body)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(result)
}

func (r *k8sRouter) deleteResource(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	var body model.KubernetesResourceDeleteRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if err := r.services.K8s.DeleteResource(req.Request.Context(), clusterID, body); err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *k8sRouter) aggregateDeployment(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	result, err := r.services.K8s.AggregateDeployment(req.Request.Context(), clusterID, namespace, name)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(result)
}

func (r *k8sRouter) listDeploymentPods(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	list, err := r.services.K8s.ListDeploymentPods(req.Request.Context(), clusterID, namespace, name)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(list)
}

func (r *k8sRouter) listWorkloadPods(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	kind := req.PathParameter("kind")
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	if strings.TrimSpace(kind) == "" {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("kind is required"))
		return
	}
	list, err := r.services.K8s.ListWorkloadPods(req.Request.Context(), clusterID, kind, namespace, name)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(list)
}

func (r *k8sRouter) workloadDetails(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	kind := req.PathParameter("kind")
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	if strings.TrimSpace(kind) == "" {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("kind is required"))
		return
	}
	details, err := r.services.K8s.WorkloadDetails(req.Request.Context(), clusterID, kind, namespace, name)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(details)
}

func (r *k8sRouter) workloadHistory(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	kind := req.PathParameter("kind")
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	history, err := r.services.K8s.WorkloadHistory(req.Request.Context(), clusterID, kind, namespace, name)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(history)
}

func (r *k8sRouter) workloadRollback(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	kind := req.PathParameter("kind")
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	var body model.KubernetesWorkloadRollbackRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if body.Revision <= 0 {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("revision is required"))
		return
	}
	if err := r.services.K8s.RollbackWorkload(req.Request.Context(), clusterID, kind, namespace, name, body.Revision); err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}

func (r *k8sRouter) workloadLogs(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	kind := req.PathParameter("kind")
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	labelSelector := req.QueryParameter("labelSelector")
	allContainers := parseBoolQuery(req.QueryParameter("allContainers"))
	var tailLines int64
	if tail := strings.TrimSpace(req.QueryParameter("tail")); tail != "" {
		if parsed, err := strconv.ParseInt(tail, 10, 64); err == nil {
			tailLines = parsed
		}
	}
	var containerList []string
	if raw := strings.TrimSpace(req.QueryParameter("containers")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				containerList = append(containerList, part)
			}
		}
	}
	content, err := r.services.K8s.AggregateWorkloadLogs(req.Request.Context(), clusterID, kind, namespace, name, labelSelector, containerList, allContainers, tailLines)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(model.KubernetesLogResponse{Content: content})
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (r *k8sRouter) listEvents(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.QueryParameter("namespace")
	kind := req.QueryParameter("kind")
	name := req.QueryParameter("name")
	page, _ := strconv.Atoi(req.QueryParameter("page"))
	perPage, _ := strconv.Atoi(req.QueryParameter("perPage"))
	items, total, err := r.services.K8s.ListEvents(req.Request.Context(), clusterID, namespace, kind, name, model.ListOptions{
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	response := model.KubernetesEventPage{
		Items: items,
		Page:  page,
		Total: total,
	}
	_ = resp.WriteEntity(response)
}

func (r *k8sRouter) podLogs(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.QueryParameter("namespace")
	pod := req.QueryParameter("pod")
	container := req.QueryParameter("container")
	tailParam := req.QueryParameter("tail")
	var tailLines int64
	if tailParam != "" {
		if parsed, err := strconv.ParseInt(tailParam, 10, 64); err == nil {
			tailLines = parsed
		}
	}
	logs, err := r.services.K8s.PodLogs(req.Request.Context(), clusterID, namespace, pod, container, tailLines)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(model.KubernetesLogResponse{Content: logs})
}

func (r *k8sRouter) execPod(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	var body model.KubernetesPodExecRequest
	if err := req.ReadEntity(&body); err != nil {
		writeError(resp, http.StatusBadRequest, err)
		return
	}
	if body.Namespace == "" {
		body.Namespace = namespace
	}
	if body.Name == "" {
		body.Name = name
	}
	result, err := r.services.K8s.ExecPod(req.Request.Context(), clusterID, body)
	if err != nil {
		writeError(resp, http.StatusInternalServerError, err)
		return
	}
	_ = resp.WriteEntity(result)
}

func (r *k8sRouter) execPodStream(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	shell := req.QueryParameter("shell")
	if shell == "" {
		shell = "/bin/bash"
	}
	conn, err := wsUpgrader.Upgrade(resp.ResponseWriter, req.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(req.Request.Context())
	defer cancel()

	stdinReader, stdinWriter := io.Pipe()
	sizeQueue := newTerminalSizeQueue()
	go r.handleExecInput(conn, stdinWriter, sizeQueue, cancel)
	stdoutWriter := &websocketJSONWriter{conn: conn, op: "stdout"}
	stderrWriter := &websocketJSONWriter{conn: conn, op: "stderr"}
	err = r.services.K8s.StreamPodExec(ctx, clusterID, model.KubernetesPodExecRequest{
		Namespace: namespace,
		Name:      name,
		Container: req.QueryParameter("container"),
		Command:   []string{shell, "-il"},
		TTY:       true,
	}, stdinReader, stdoutWriter, stderrWriter, sizeQueue)
	if err != nil && !isNormalClosure(err) {
		_ = writeShellFrame(conn, shellFrame{Op: "error", Data: err.Error()})
	}
}

func (r *k8sRouter) handleExecInput(conn *websocket.Conn, stdin io.WriteCloser, queue *terminalSizeQueue, cancel context.CancelFunc) {
	defer func() {
		stdin.Close()
		queue.Close()
		cancel()
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame shellFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}
		switch strings.ToLower(frame.Op) {
		case "stdin":
			if frame.Data == "" {
				continue
			}
			if _, err := stdin.Write([]byte(frame.Data)); err != nil {
				return
			}
		case "resize":
			if frame.Cols > 0 && frame.Rows > 0 {
				queue.Push(frame.Cols, frame.Rows)
			}
		case "close":
			return
		}
	}
}

func (r *k8sRouter) podLogsStream(req *restful.Request, resp *restful.Response) {
	clusterID, ok := parseClusterID(req, resp)
	if !ok {
		return
	}
	namespace := req.PathParameter("namespace")
	name := req.PathParameter("name")
	container := req.QueryParameter("container")
	var tailLines int64
	if tail := strings.TrimSpace(req.QueryParameter("tail")); tail != "" {
		if parsed, err := strconv.ParseInt(tail, 10, 64); err == nil {
			tailLines = parsed
		}
	}
	conn, err := wsUpgrader.Upgrade(resp.ResponseWriter, req.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(req.Request.Context())
	defer cancel()

	stream, err := r.services.K8s.StreamPodLogs(ctx, clusterID, namespace, name, container, tailLines)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("error: %v", err)))
		return
	}
	defer stream.Close()

	buf := make([]byte, 4096)
	for {
		n, readErr := stream.Read(buf)
		if n > 0 {
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("error: %v", readErr)))
			}
			break
		}
	}
}

type websocketJSONWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
	op   string
}

func (w *websocketJSONWriter) Write(p []byte) (int, error) {
	frame := shellFrame{Op: w.op, Data: string(p)}
	if err := writeShellFrame(w.conn, frame); err != nil {
		return 0, err
	}
	return len(p), nil
}

type shellFrame struct {
	Op   string `json:"op"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

func writeShellFrame(conn *websocket.Conn, frame shellFrame) error {
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func isNormalClosure(err error) bool {
	if err == nil {
		return true
	}
	// remotecommand returns io.EOF when remote side closes
	return err == io.EOF
}

type terminalSizeQueue struct {
	ch chan remotecommand.TerminalSize
}

func newTerminalSizeQueue() *terminalSizeQueue {
	return &terminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 1)}
}

func (q *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return &size
}

func (q *terminalSizeQueue) Push(cols, rows uint16) {
	select {
	case q.ch <- remotecommand.TerminalSize{Width: cols, Height: rows}:
	default:
	}
}

func (q *terminalSizeQueue) Close() {
	close(q.ch)
}

func parseClusterID(req *restful.Request, resp *restful.Response) (int64, bool) {
	raw := req.PathParameter("cluster_id")
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		writeError(resp, http.StatusBadRequest, fmt.Errorf("invalid cluster_id"))
		return 0, false
	}
	return id, true
}
