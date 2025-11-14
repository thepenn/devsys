package k8s

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	sigyaml "sigs.k8s.io/yaml"

	"github.com/thepenn/devsys/model"
	systemService "github.com/thepenn/devsys/service/system"
)

// Service exposes helper APIs to work with Kubernetes clusters stored as certificates.
type Service struct {
	system *systemService.Service

	mu          sync.RWMutex
	clientCache map[int64]*rest.Config
	dynCache    map[int64]dynamic.Interface
	discoCache  map[int64]discovery.DiscoveryInterface
}

// New creates a new Kubernetes helper service.
func New(system *systemService.Service) *Service {
	return &Service{
		system:      system,
		clientCache: map[int64]*rest.Config{},
		dynCache:    map[int64]dynamic.Interface{},
		discoCache:  map[int64]discovery.DiscoveryInterface{},
	}
}

// ListClusters lists all kubernetes certificates.
func (s *Service) ListClusters(ctx context.Context) ([]model.KubernetesClusterSummary, error) {
	if s.system == nil {
		return nil, fmt.Errorf("system service unavailable")
	}
	certs, _, err := s.system.ListCertificates(ctx, model.ListOptions{All: true}, model.CertificateFilter{
		Type: model.CertificateTypeKubernetes,
	})
	if err != nil {
		return nil, err
	}
	clusters := make([]model.KubernetesClusterSummary, 0, len(certs))
	for _, cert := range certs {
		kube, err := cert.AsKubernetesCertificate()
		if err != nil {
			continue
		}
		server := kube.Server
		if server == "" {
			server = extractServerFromKubeconfig(kube.KubeConfig)
		}
		clusters = append(clusters, model.KubernetesClusterSummary{
			ID:      cert.ID,
			Name:    cert.Name,
			Server:  server,
			Scope:   cert.Scope,
			Updated: cert.Updated,
		})
	}
	return clusters, nil
}

// ListNamespaces returns namespaces for cluster.
func (s *Service) ListNamespaces(ctx context.Context, clusterID int64) ([]model.KubernetesNamespace, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]model.KubernetesNamespace, 0, len(list.Items))
	for _, item := range list.Items {
		result = append(result, model.KubernetesNamespace{
			Name:   item.Name,
			Labels: item.Labels,
		})
	}
	return result, nil
}

// ListResources lists resources by query.
func (s *Service) ListResources(ctx context.Context, clusterID int64, query model.KubernetesResourceQuery) ([]map[string]interface{}, error) {
	if strings.TrimSpace(query.Resource) == "" {
		return nil, fmt.Errorf("resource is required")
	}
	client, err := s.dynamicClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	gvr := resolveGVR(query.Group, query.Version, query.Resource)
	resource := client.Resource(gvr)
	target := dynamic.ResourceInterface(resource)
	if ns := strings.TrimSpace(query.Namespace); ns != "" {
		target = resource.Namespace(ns)
	}
	list, err := target.List(ctx, metav1.ListOptions{
		LabelSelector: query.LabelSelector,
		FieldSelector: query.FieldSelector,
	})
	if err != nil {
		return nil, err
	}
	results := make([]map[string]interface{}, 0, len(list.Items))
	for _, item := range list.Items {
		results = append(results, item.UnstructuredContent())
	}
	return results, nil
}

// GetResource returns a single resource.
func (s *Service) GetResource(ctx context.Context, clusterID int64, query model.KubernetesResourceQuery) (*model.KubernetesObjectResponse, error) {
	if strings.TrimSpace(query.Resource) == "" || strings.TrimSpace(query.Name) == "" {
		return nil, fmt.Errorf("resource and name are required")
	}
	client, err := s.dynamicClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	gvr := resolveGVR(query.Group, query.Version, query.Resource)
	resource := client.Resource(gvr)
	target := dynamic.ResourceInterface(resource)
	if ns := strings.TrimSpace(query.Namespace); ns != "" {
		target = resource.Namespace(ns)
	}
	obj, err := target.Get(ctx, query.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return buildObjectResponse(obj)
}

// ApplyManifest applies manifest to cluster.
func (s *Service) ApplyManifest(ctx context.Context, clusterID int64, req model.KubernetesManifestRequest) (*model.KubernetesObjectResponse, error) {
	manifest := strings.TrimSpace(req.Manifest)
	if manifest == "" {
		return nil, fmt.Errorf("manifest is required")
	}
	if strings.TrimSpace(req.Resource) == "" {
		return nil, fmt.Errorf("resource is required")
	}
	gvr := resolveGVR(req.Group, req.Version, req.Resource)
	obj, namespace, err := decodeManifest(manifest, req.Namespace)
	if err != nil {
		return nil, err
	}
	client, err := s.dynamicClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	resource := client.Resource(gvr)
	target := dynamic.ResourceInterface(resource)
	if namespace != "" {
		target = resource.Namespace(namespace)
	}
	current, err := target.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		created, err := target.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		return buildObjectResponse(created)
	}
	obj.SetResourceVersion(current.GetResourceVersion())
	updated, err := target.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return buildObjectResponse(updated)
}

// DeleteResource deletes resource.
func (s *Service) DeleteResource(ctx context.Context, clusterID int64, req model.KubernetesResourceDeleteRequest) error {
	if strings.TrimSpace(req.Resource) == "" || strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("resource and name are required")
	}
	client, err := s.dynamicClient(ctx, clusterID)
	if err != nil {
		return err
	}
	gvr := resolveGVR(req.Group, req.Version, req.Resource)
	resource := client.Resource(gvr)
	target := dynamic.ResourceInterface(resource)
	if ns := strings.TrimSpace(req.Namespace); ns != "" {
		target = resource.Namespace(ns)
	}
	return target.Delete(ctx, req.Name, metav1.DeleteOptions{})
}

// AggregateDeployment collects deployment and related resources.
func (s *Service) AggregateDeployment(ctx context.Context, clusterID int64, namespace, name string) ([]model.KubernetesObjectResponse, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	configNames := collectDeploymentConfigMaps(deployment)
	configResponses := make([]model.KubernetesObjectResponse, 0, len(configNames))
	for _, cfg := range configNames {
		obj, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, cfg, metav1.GetOptions{})
		if err != nil {
			continue
		}
		resp, err := responseFromObject(obj)
		if err == nil {
			configResponses = append(configResponses, *resp)
		}
	}
	sort.Slice(configResponses, func(i, j int) bool {
		return getObjectName(configResponses[i].Object) < getObjectName(configResponses[j].Object)
	})
	deployResp, err := responseFromObject(deployment)
	if err != nil {
		return nil, err
	}
	serviceResponses := []model.KubernetesObjectResponse{}
	if deployment.Spec.Selector != nil {
		matchLabels := labelsFromSelector(deployment.Spec.Selector)
		serviceList, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, svc := range serviceList.Items {
				if len(svc.Spec.Selector) == 0 {
					continue
				}
				if selectorMatches(matchLabels, svc.Spec.Selector) {
					resp, err := responseFromObject(&svc)
					if err == nil {
						serviceResponses = append(serviceResponses, *resp)
					}
				}
			}
		}
		sort.Slice(serviceResponses, func(i, j int) bool {
			return getObjectName(serviceResponses[i].Object) < getObjectName(serviceResponses[j].Object)
		})
	}
	result := make([]model.KubernetesObjectResponse, 0, len(configResponses)+1+len(serviceResponses))
	result = append(result, configResponses...)
	result = append(result, *deployResp)
	result = append(result, serviceResponses...)
	return result, nil
}

// ListDeploymentPods lists pods for deployment.
func (s *Service) ListDeploymentPods(ctx context.Context, clusterID int64, namespace, name string) ([]model.KubernetesPodSummary, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]model.KubernetesPodSummary, 0, len(podList.Items))
	for _, pod := range podList.Items {
		containers := make([]string, 0, len(pod.Spec.Containers))
		for _, c := range pod.Spec.Containers {
			containers = append(containers, c.Name)
		}
		status := string(pod.Status.Phase)
		if pod.Status.Reason != "" {
			status = fmt.Sprintf("%s (%s)", status, pod.Status.Reason)
		}
		result = append(result, model.KubernetesPodSummary{
			Name:       pod.Name,
			Namespace:  pod.Namespace,
			Status:     status,
			Containers: containers,
		})
	}
	return result, nil
}

// ListWorkloadPods lists pods for workloads (deployment/statefulset/daemonset).
func (s *Service) ListWorkloadPods(ctx context.Context, clusterID int64, kind, namespace, name string) ([]model.KubernetesPodRow, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	labelSelector, err := s.selectorForWorkload(ctx, client, kind, namespace, name)
	if err != nil {
		return nil, err
	}
	if labelSelector == nil {
		return nil, fmt.Errorf("workload %s has no selector", name)
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return nil, err
	}
	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	rows := make([]model.KubernetesPodRow, 0, len(podList.Items))
	for _, pod := range podList.Items {
		rows = append(rows, buildPodRow(&pod))
	}
	return rows, nil
}

// ExecPod executes a command within a pod container.
func (s *Service) ExecPod(ctx context.Context, clusterID int64, req model.KubernetesPodExecRequest) (*model.KubernetesPodExecResult, error) {
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	if req.Namespace == "" || req.Name == "" {
		return nil, fmt.Errorf("namespace and name are required")
	}
	if len(req.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}
	cfg, err := s.restConfig(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	container := strings.TrimSpace(req.Container)
	if container == "" {
		pod, err := client.CoreV1().Pods(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if len(pod.Spec.Containers) == 0 {
			return nil, fmt.Errorf("pod %s has no containers", req.Name)
		}
		container = pod.Spec.Containers[0].Name
	}
	execReq := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(req.Name).
		Namespace(req.Namespace).
		SubResource("exec")
	execReq.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   req.Command,
		Stdout:    true,
		Stderr:    true,
		TTY:       req.TTY,
	}, scheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", execReq.URL())
	if err != nil {
		return nil, err
	}
	if err := exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    req.TTY,
	}); err != nil {
		return nil, err
	}
	return &model.KubernetesPodExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, nil
}

// StreamPodExec establishes a streaming exec session.
func (s *Service) StreamPodExec(
	ctx context.Context,
	clusterID int64,
	req model.KubernetesPodExecRequest,
	stdin io.Reader,
	stdout, stderr io.Writer,
	sizeQueue remotecommand.TerminalSizeQueue,
) error {
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	if req.Namespace == "" || req.Name == "" {
		return fmt.Errorf("namespace and name are required")
	}
	cfg, err := s.restConfig(ctx, clusterID)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	container := strings.TrimSpace(req.Container)
	if container == "" {
		pod, err := client.CoreV1().Pods(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(pod.Spec.Containers) == 0 {
			return fmt.Errorf("pod %s has no containers", req.Name)
		}
		container = pod.Spec.Containers[0].Name
	}
	command := req.Command
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}
	request := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(req.Name).
		Namespace(req.Namespace).
		SubResource("exec")
	request.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdin:     stdin != nil,
		Stdout:    stdout != nil,
		Stderr:    stderr != nil,
		TTY:       req.TTY,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", request.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               req.TTY,
		TerminalSizeQueue: sizeQueue,
	})
}

// StreamPodLogs streams pod logs with follow enabled.
func (s *Service) StreamPodLogs(ctx context.Context, clusterID int64, namespace, name, container string, tailLines int64) (io.ReadCloser, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{
		Follow:    true,
		Container: strings.TrimSpace(container),
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}
	req := client.CoreV1().Pods(strings.TrimSpace(namespace)).GetLogs(strings.TrimSpace(name), opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// ListEvents returns events for resource.
func (s *Service) ListEvents(ctx context.Context, clusterID int64, namespace, kind, name string, opts model.ListOptions) ([]model.KubernetesEvent, int64, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, 0, err
	}
	ns := namespace
	if strings.TrimSpace(ns) == "" {
		ns = metav1.NamespaceAll
	}
	fieldSelectors := []string{}
	if kind != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("involvedObject.kind=%s", kind))
	}
	if name != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("involvedObject.name=%s", name))
	}
	if namespace != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("involvedObject.namespace=%s", namespace))
	}
	events, err := client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: strings.Join(fieldSelectors, ","),
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]model.KubernetesEvent, 0, len(events.Items))
	for _, evt := range events.Items {
		first := evt.FirstTimestamp.Unix()
		if first == 0 {
			first = evt.EventTime.Unix()
		}
		last := evt.LastTimestamp.Unix()
		if last == 0 {
			last = evt.EventTime.Unix()
		}
		items = append(items, model.KubernetesEvent{
			Type:           evt.Type,
			Reason:         evt.Reason,
			Message:        evt.Message,
			Count:          evt.Count,
			FirstTimestamp: first,
			LastTimestamp:  last,
		})
	}
	total := int64(len(items))
	page := opts.Page
	perPage := opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	start := (page - 1) * perPage
	if start >= len(items) {
		return []model.KubernetesEvent{}, total, nil
	}
	end := start + perPage
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}

// WorkloadDetails returns related resources for workload kinds (deployment/statefulset/daemonset).
func (s *Service) WorkloadDetails(ctx context.Context, clusterID int64, kind, namespace, name string) (*model.KubernetesWorkloadDetails, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	result := &model.KubernetesWorkloadDetails{}

	var (
		selector *metav1.LabelSelector
		template *corev1.PodTemplateSpec
	)

	switch kind {
	case "deployment":
		dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		selector = dep.Spec.Selector
		template = &dep.Spec.Template
		result.Workload = buildNamedResourceFromMeta(dep.ObjectMeta, "Deployment", "apps", "v1", "deployments")
		result.Overview = buildDeploymentOverview(dep)
	case "statefulset":
		sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		selector = sts.Spec.Selector
		template = &sts.Spec.Template
		result.Workload = buildNamedResourceFromMeta(sts.ObjectMeta, "StatefulSet", "apps", "v1", "statefulsets")
		result.Overview = buildStatefulSetOverview(sts)
	case "daemonset":
		ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		selector = ds.Spec.Selector
		template = &ds.Spec.Template
		result.Workload = buildNamedResourceFromMeta(ds.ObjectMeta, "DaemonSet", "apps", "v1", "daemonsets")
		result.Overview = buildDaemonSetOverview(ds)
	default:
		return nil, fmt.Errorf("unsupported workload kind %s", kind)
	}

	if selector == nil {
		return nil, fmt.Errorf("workload %s has no selector", name)
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	labelsMap := labelsFromSelector(selector)
	result.Overview.Selector = labelsMap
	result.Overview.Containers = summarizeTemplateContainers(template)
	if result.Overview.Labels == nil && result.Workload.Labels != nil {
		result.Overview.Labels = result.Workload.Labels
	}

	// pods
	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector.String()})
	if err == nil {
		for _, pod := range podList.Items {
			result.Pods = append(result.Pods, buildPodRow(&pod))
		}
	}

	matchedServices := map[string]struct{}{}
	// services & endpoints
	if svcList, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, svc := range svcList.Items {
			if len(svc.Spec.Selector) == 0 {
				continue
			}
			if selectorMatches(labelsMap, svc.Spec.Selector) {
				resource := buildNamedResourceFromMeta(svc.ObjectMeta, "Service", "", "v1", "services")
				resource.Type = string(svc.Spec.Type)
				result.Services = append(result.Services, resource)
				matchedServices[svc.Name] = struct{}{}
				appendEndpointsForService(ctx, client, namespace, &svc, &result.Endpoints)
			}
		}
	}

	// ingresses referencing matched services
	if len(matchedServices) > 0 {
		if ingList, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, ing := range ingList.Items {
				if ingressMatchesServices(&ing, matchedServices) {
					result.Ingresses = append(result.Ingresses, buildNamedResourceFromMeta(ing.ObjectMeta, "Ingress", "networking.k8s.io", "v1", "ingresses"))
				}
			}
		}
	}

	// configmaps / secrets / pvc
	if template != nil {
		configs := collectTemplateConfigMaps(template)
		for _, cfg := range configs {
			if cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, cfg, metav1.GetOptions{}); err == nil {
				result.ConfigMaps = append(result.ConfigMaps, buildNamedResourceFromMeta(cm.ObjectMeta, "ConfigMap", "", "v1", "configmaps"))
			}
		}
		secrets := collectTemplateSecrets(template)
		for _, sec := range secrets {
			if secret, err := client.CoreV1().Secrets(namespace).Get(ctx, sec, metav1.GetOptions{}); err == nil {
				resource := buildNamedResourceFromMeta(secret.ObjectMeta, "Secret", "", "v1", "secrets")
				resource.Type = string(secret.Type)
				result.Secrets = append(result.Secrets, resource)
			}
		}
		pvcs := collectTemplatePVCs(template)
		for _, claim := range pvcs {
			if pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, claim, metav1.GetOptions{}); err == nil {
				result.PVCs = append(result.PVCs, buildNamedResourceFromMeta(pvc.ObjectMeta, "PersistentVolumeClaim", "", "v1", "persistentvolumeclaims"))
			}
		}
		result.Volumes = append(result.Volumes, describeTemplateVolumes(template)...)
	}
	return result, nil
}

func buildNamedResourceFromMeta(meta metav1.ObjectMeta, kind, group, version, resource string) model.KubernetesNamedResource {
	return model.KubernetesNamedResource{
		Name:       meta.Name,
		Namespace:  meta.Namespace,
		Labels:     meta.Labels,
		Kind:       kind,
		Group:      group,
		Version:    version,
		Resource:   resource,
		Namespaced: meta.Namespace != "",
	}
}

func buildDeploymentOverview(dep *appsv1.Deployment) model.KubernetesWorkloadOverview {
	if dep == nil {
		return model.KubernetesWorkloadOverview{}
	}
	var desired int32
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	strategy := model.KubernetesWorkloadStrategy{Type: string(dep.Spec.Strategy.Type)}
	if dep.Spec.Strategy.RollingUpdate != nil {
		strategy.MaxSurge = formatIntOrString(dep.Spec.Strategy.RollingUpdate.MaxSurge)
		strategy.MaxUnavailable = formatIntOrString(dep.Spec.Strategy.RollingUpdate.MaxUnavailable)
	}
	conditions := convertDeploymentConditions(dep.Status.Conditions)
	return model.KubernetesWorkloadOverview{
		Kind:        "Deployment",
		Name:        dep.Name,
		Namespace:   dep.Namespace,
		Labels:      dep.Labels,
		Annotations: dep.Annotations,
		Strategy:    strategy,
		Replica: model.KubernetesReplicaStatus{
			Desired:   desired,
			Ready:     dep.Status.ReadyReplicas,
			Available: dep.Status.AvailableReplicas,
			Updated:   dep.Status.UpdatedReplicas,
		},
		Conditions:        conditions,
		CreationTimestamp: dep.CreationTimestamp.Unix(),
		UpdateTimestamp:   latestConditionTimestamp(conditions, dep.CreationTimestamp.Unix()),
	}
}

func buildStatefulSetOverview(sts *appsv1.StatefulSet) model.KubernetesWorkloadOverview {
	if sts == nil {
		return model.KubernetesWorkloadOverview{}
	}
	var desired int32
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}
	strategy := model.KubernetesWorkloadStrategy{Type: string(sts.Spec.UpdateStrategy.Type)}
	if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		strategy.Partition = sts.Spec.UpdateStrategy.RollingUpdate.Partition
	}
	conditions := convertStatefulSetConditions(sts.Status.Conditions)
	return model.KubernetesWorkloadOverview{
		Kind:        "StatefulSet",
		Name:        sts.Name,
		Namespace:   sts.Namespace,
		Labels:      sts.Labels,
		Annotations: sts.Annotations,
		Strategy:    strategy,
		Replica: model.KubernetesReplicaStatus{
			Desired:   desired,
			Ready:     sts.Status.ReadyReplicas,
			Available: sts.Status.CurrentReplicas,
			Updated:   sts.Status.UpdatedReplicas,
		},
		Conditions:        conditions,
		CreationTimestamp: sts.CreationTimestamp.Unix(),
		UpdateTimestamp:   latestConditionTimestamp(conditions, sts.CreationTimestamp.Unix()),
	}
}

func buildDaemonSetOverview(ds *appsv1.DaemonSet) model.KubernetesWorkloadOverview {
	if ds == nil {
		return model.KubernetesWorkloadOverview{}
	}
	strategy := model.KubernetesWorkloadStrategy{Type: string(ds.Spec.UpdateStrategy.Type)}
	if ds.Spec.UpdateStrategy.RollingUpdate != nil {
		strategy.MaxUnavailable = formatIntOrString(ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	}
	conditions := convertDaemonSetConditions(ds.Status.Conditions)
	return model.KubernetesWorkloadOverview{
		Kind:        "DaemonSet",
		Name:        ds.Name,
		Namespace:   ds.Namespace,
		Labels:      ds.Labels,
		Annotations: ds.Annotations,
		Strategy:    strategy,
		Replica: model.KubernetesReplicaStatus{
			Desired:   ds.Status.DesiredNumberScheduled,
			Ready:     ds.Status.NumberReady,
			Available: ds.Status.NumberAvailable,
			Updated:   ds.Status.UpdatedNumberScheduled,
		},
		Conditions:        conditions,
		CreationTimestamp: ds.CreationTimestamp.Unix(),
		UpdateTimestamp:   latestConditionTimestamp(conditions, ds.CreationTimestamp.Unix()),
	}
}

func convertDeploymentConditions(conds []appsv1.DeploymentCondition) []model.KubernetesCondition {
	result := make([]model.KubernetesCondition, 0, len(conds))
	for _, c := range conds {
		result = append(result, model.KubernetesCondition{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastTransitionTime: c.LastUpdateTime.Unix(),
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}

func convertStatefulSetConditions(conds []appsv1.StatefulSetCondition) []model.KubernetesCondition {
	result := make([]model.KubernetesCondition, 0, len(conds))
	for _, c := range conds {
		result = append(result, model.KubernetesCondition{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastTransitionTime: c.LastTransitionTime.Unix(),
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}

func convertDaemonSetConditions(conds []appsv1.DaemonSetCondition) []model.KubernetesCondition {
	result := make([]model.KubernetesCondition, 0, len(conds))
	for _, c := range conds {
		result = append(result, model.KubernetesCondition{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastTransitionTime: c.LastTransitionTime.Unix(),
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}

func latestConditionTimestamp(conds []model.KubernetesCondition, fallback int64) int64 {
	latest := fallback
	for _, c := range conds {
		if c.LastTransitionTime > latest {
			latest = c.LastTransitionTime
		}
	}
	return latest
}

func summarizeTemplateContainers(t *corev1.PodTemplateSpec) []model.KubernetesContainerSummary {
	if t == nil {
		return nil
	}
	summaries := make([]model.KubernetesContainerSummary, 0, len(t.Spec.Containers)+len(t.Spec.InitContainers))
	appendContainers := func(list []corev1.Container, init bool) {
		for _, c := range list {
			summaries = append(summaries, model.KubernetesContainerSummary{
				Name:      c.Name,
				Image:     c.Image,
				Command:   append([]string{}, c.Command...),
				Args:      append([]string{}, c.Args...),
				Ports:     formatContainerPorts(c.Ports),
				Liveness:  c.LivenessProbe != nil,
				Readiness: c.ReadinessProbe != nil,
				Env:       summarizeEnvVars(c.Env),
				Init:      init,
			})
		}
	}
	appendContainers(t.Spec.InitContainers, true)
	appendContainers(t.Spec.Containers, false)
	return summaries
}

func formatContainerPorts(ports []corev1.ContainerPort) []string {
	if len(ports) == 0 {
		return nil
	}
	result := make([]string, 0, len(ports))
	for _, port := range ports {
		proto := string(port.Protocol)
		if proto == "" {
			proto = string(corev1.ProtocolTCP)
		}
		result = append(result, fmt.Sprintf("%d/%s", port.ContainerPort, proto))
	}
	return result
}

func summarizeEnvVars(vars []corev1.EnvVar) []string {
	if len(vars) == 0 {
		return nil
	}
	result := make([]string, 0, len(vars))
	for _, env := range vars {
		value := env.Value
		if env.ValueFrom != nil {
			switch {
			case env.ValueFrom.SecretKeyRef != nil:
				value = fmt.Sprintf("secret:%s/%s", env.ValueFrom.SecretKeyRef.Name, env.ValueFrom.SecretKeyRef.Key)
			case env.ValueFrom.ConfigMapKeyRef != nil:
				value = fmt.Sprintf("config:%s/%s", env.ValueFrom.ConfigMapKeyRef.Name, env.ValueFrom.ConfigMapKeyRef.Key)
			case env.ValueFrom.FieldRef != nil:
				value = fmt.Sprintf("field:%s", env.ValueFrom.FieldRef.FieldPath)
			case env.ValueFrom.ResourceFieldRef != nil:
				value = fmt.Sprintf("resource:%s", env.ValueFrom.ResourceFieldRef.Resource)
			}
		}
		result = append(result, fmt.Sprintf("%s=%s", env.Name, value))
	}
	return result
}

func formatIntOrString(value *intstr.IntOrString) string {
	if value == nil {
		return ""
	}
	if value.Type == intstr.String {
		return value.StrVal
	}
	return fmt.Sprintf("%d", value.IntValue())
}

func ingressMatchesServices(ing *networkingv1.Ingress, services map[string]struct{}) bool {
	if ing == nil {
		return false
	}
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		if _, ok := services[ing.Spec.DefaultBackend.Service.Name]; ok {
			return true
		}
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				if _, ok := services[path.Backend.Service.Name]; ok {
					return true
				}
			}
		}
	}
	return false
}

func appendEndpointsForService(ctx context.Context, client kubernetes.Interface, namespace string, svc *corev1.Service, target *[]model.KubernetesNamedResource) {
	if svc == nil || target == nil || svc.Name == "" {
		return
	}
	labelSelector := fmt.Sprintf("kubernetes.io/service-name=%s", svc.Name)
	if slices, err := client.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	}); err == nil && len(slices.Items) > 0 {
		for _, slice := range slices.Items {
			*target = append(*target, buildNamedResourceFromMeta(slice.ObjectMeta, "EndpointSlice", discoveryv1.GroupName, "v1", "endpointslices"))
		}
		return
	}
	if ep, err := client.CoreV1().Endpoints(namespace).Get(ctx, svc.Name, metav1.GetOptions{}); err == nil {
		*target = append(*target, buildNamedResourceFromMeta(ep.ObjectMeta, "Endpoints", "", "v1", "endpoints"))
	}
}

// WorkloadHistory returns rollout history entries for supported workloads.
func (s *Service) WorkloadHistory(ctx context.Context, clusterID int64, kind, namespace, name string) ([]model.KubernetesWorkloadHistoryEntry, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "deployment":
		dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return deploymentHistoryEntries(ctx, client, dep)
	default:
		return nil, fmt.Errorf("history for %s not supported", kind)
	}
}

// RollbackWorkload rolls workload back to a previous revision (deployment only).
func (s *Service) RollbackWorkload(ctx context.Context, clusterID int64, kind, namespace, name string, revision int64) error {
	if revision <= 0 {
		return fmt.Errorf("revision must be greater than zero")
	}
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "deployment":
		dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		rs, err := findDeploymentReplicaSetByRevision(ctx, client, dep, revision)
		if err != nil {
			return err
		}
		dep.Spec.Template = rs.Spec.Template
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = map[string]string{}
		}
		dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		dep.Spec.Template.Annotations["devsys.dev/rollback-revision"] = fmt.Sprintf("%d", revision)
		_, err = client.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("rollback for %s not implemented", kind)
	}
}

// AggregateWorkloadLogs returns concatenated logs for pods matching selector/workload.
func (s *Service) AggregateWorkloadLogs(ctx context.Context, clusterID int64, kind, namespace, name, selectorOverride string, containers []string, allContainers bool, tailLines int64) (string, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return "", err
	}
	labelSelector := strings.TrimSpace(selectorOverride)
	if labelSelector == "" {
		sel, err := s.selectorForWorkload(ctx, client, kind, namespace, name)
		if err != nil {
			return "", err
		}
		if sel == nil {
			return "", fmt.Errorf("workload %s has no selector", name)
		}
		selector, err := metav1.LabelSelectorAsSelector(sel)
		if err != nil {
			return "", err
		}
		labelSelector = selector.String()
	}
	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return "", err
	}
	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no pods matched selector %s", labelSelector)
	}
	const maxPods = 20
	truncated := false
	if len(podList.Items) > maxPods {
		podList.Items = podList.Items[:maxPods]
		truncated = true
	}
	var builder strings.Builder
	for _, pod := range podList.Items {
		targetContainers := containers
		if len(targetContainers) == 0 {
			if allContainers {
				targetContainers = containerNamesFromPod(&pod)
			} else if len(pod.Spec.Containers) > 0 {
				targetContainers = []string{pod.Spec.Containers[0].Name}
			}
		}
		if len(targetContainers) == 0 {
			continue
		}
		for _, c := range targetContainers {
			builder.WriteString(fmt.Sprintf(">>> %s/%s\n", pod.Name, c))
			logs, err := s.PodLogs(ctx, clusterID, pod.Namespace, pod.Name, c, tailLines)
			if err != nil {
				builder.WriteString(fmt.Sprintf("error: %v\n\n", err))
				continue
			}
			builder.WriteString(logs)
			if !strings.HasSuffix(logs, "\n") {
				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}
	if truncated {
		builder.WriteString(fmt.Sprintf("[truncated to %d pods]\n", maxPods))
	}
	return builder.String(), nil
}

func deploymentHistoryEntries(ctx context.Context, client kubernetes.Interface, dep *appsv1.Deployment) ([]model.KubernetesWorkloadHistoryEntry, error) {
	rsList, err := client.AppsV1().ReplicaSets(dep.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	entries := make([]model.KubernetesWorkloadHistoryEntry, 0, len(rsList.Items))
	for _, rs := range rsList.Items {
		if !ownedBy(rs.OwnerReferences, dep.UID) {
			continue
		}
		revisionStr := rs.Annotations["deployment.kubernetes.io/revision"]
		rev, err := strconv.ParseInt(revisionStr, 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, model.KubernetesWorkloadHistoryEntry{
			Revision:  rev,
			Images:    collectTemplateImages(&rs.Spec.Template),
			CreatedAt: rs.CreationTimestamp.Unix(),
			Source:    rs.Name,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Revision == entries[j].Revision {
			return entries[i].CreatedAt > entries[j].CreatedAt
		}
		return entries[i].Revision > entries[j].Revision
	})
	return entries, nil
}

func findDeploymentReplicaSetByRevision(ctx context.Context, client kubernetes.Interface, dep *appsv1.Deployment, revision int64) (*appsv1.ReplicaSet, error) {
	rsList, err := client.AppsV1().ReplicaSets(dep.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, rs := range rsList.Items {
		if !ownedBy(rs.OwnerReferences, dep.UID) {
			continue
		}
		if rev, err := strconv.ParseInt(rs.Annotations["deployment.kubernetes.io/revision"], 10, 64); err == nil && rev == revision {
			copy := rs
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("revision %d not found", revision)
}

func ownedBy(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func collectTemplateImages(t *corev1.PodTemplateSpec) []string {
	if t == nil {
		return nil
	}
	images := map[string]struct{}{}
	add := func(list []corev1.Container) {
		for _, c := range list {
			if c.Image != "" {
				images[c.Image] = struct{}{}
			}
		}
	}
	add(t.Spec.Containers)
	add(t.Spec.InitContainers)
	result := make([]string, 0, len(images))
	for image := range images {
		result = append(result, image)
	}
	sort.Strings(result)
	return result
}

func containerNamesFromPod(pod *corev1.Pod) []string {
	if pod == nil {
		return nil
	}
	names := make([]string, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	for _, c := range pod.Spec.InitContainers {
		names = append(names, c.Name)
	}
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	return names
}

func collectTemplateConfigMaps(t *corev1.PodTemplateSpec) []string {
	set := map[string]struct{}{}
	if t == nil {
		return nil
	}
	add := func(name string) {
		if name == "" {
			return
		}
		set[name] = struct{}{}
	}
	for _, vol := range t.Spec.Volumes {
		if vol.ConfigMap != nil {
			add(vol.ConfigMap.Name)
		}
		if vol.Projected != nil {
			for _, src := range vol.Projected.Sources {
				if src.ConfigMap != nil {
					add(src.ConfigMap.Name)
				}
			}
		}
	}
	for _, container := range append([]corev1.Container{}, append(t.Spec.Containers, t.Spec.InitContainers...)...) {
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				add(envFrom.ConfigMapRef.Name)
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				add(env.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	return names
}

func collectTemplateSecrets(t *corev1.PodTemplateSpec) []string {
	set := map[string]struct{}{}
	if t == nil {
		return nil
	}
	add := func(name string) {
		if name == "" {
			return
		}
		set[name] = struct{}{}
	}
	for _, vol := range t.Spec.Volumes {
		if vol.Secret != nil {
			add(vol.Secret.SecretName)
		}
		if vol.Projected != nil {
			for _, src := range vol.Projected.Sources {
				if src.Secret != nil {
					add(src.Secret.Name)
				}
			}
		}
	}
	for _, container := range append([]corev1.Container{}, append(t.Spec.Containers, t.Spec.InitContainers...)...) {
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				add(envFrom.SecretRef.Name)
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				add(env.ValueFrom.SecretKeyRef.Name)
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	return names
}

func collectTemplatePVCs(t *corev1.PodTemplateSpec) []string {
	set := map[string]struct{}{}
	if t == nil {
		return nil
	}
	for _, vol := range t.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			name := vol.PersistentVolumeClaim.ClaimName
			if name != "" {
				set[name] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	return names
}

func describeTemplateVolumes(t *corev1.PodTemplateSpec) []model.KubernetesVolumeReference {
	vols := make([]model.KubernetesVolumeReference, 0)
	if t == nil {
		return vols
	}
	for _, vol := range t.Spec.Volumes {
		ref := model.KubernetesVolumeReference{Name: vol.Name}
		if vol.PersistentVolumeClaim != nil {
			ref.Kind = "PersistentVolumeClaim"
			ref.SourceName = vol.PersistentVolumeClaim.ClaimName
		} else if vol.ConfigMap != nil {
			ref.Kind = "ConfigMap"
			ref.SourceName = vol.ConfigMap.Name
		} else if vol.Secret != nil {
			ref.Kind = "Secret"
			ref.SourceName = vol.Secret.SecretName
		} else if vol.EmptyDir != nil {
			ref.Kind = "EmptyDir"
		} else if vol.HostPath != nil {
			ref.Kind = "HostPath"
			ref.SourceName = vol.HostPath.Path
		} else if vol.Projected != nil {
			ref.Kind = "Projected"
		} else {
			ref.Kind = "Other"
		}
		vols = append(vols, ref)
	}
	return vols
}

// PodLogs returns logs for pod.
func (s *Service) PodLogs(ctx context.Context, clusterID int64, namespace, pod, container string, tailLines int64) (string, error) {
	client, err := s.typedClient(ctx, clusterID)
	if err != nil {
		return "", err
	}
	if pod == "" {
		return "", fmt.Errorf("pod is required")
	}
	options := &corev1.PodLogOptions{}
	if container != "" {
		options.Container = container
	}
	if tailLines > 0 {
		options.TailLines = &tailLines
	}
	req := client.CoreV1().Pods(namespace).GetLogs(pod, options)
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	var builder strings.Builder
	if _, err := io.Copy(&builder, stream); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func (s *Service) typedClient(ctx context.Context, clusterID int64) (kubernetes.Interface, error) {
	cfg, err := s.restConfig(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (s *Service) dynamicClient(ctx context.Context, clusterID int64) (dynamic.Interface, error) {
	s.mu.RLock()
	if client, ok := s.dynCache[clusterID]; ok {
		s.mu.RUnlock()
		return client, nil
	}
	s.mu.RUnlock()
	cfg, err := s.restConfig(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.dynCache[clusterID] = client
	s.mu.Unlock()
	return client, nil
}

func (s *Service) restConfig(ctx context.Context, clusterID int64) (*rest.Config, error) {
	if s.system == nil {
		return nil, fmt.Errorf("system service unavailable")
	}
	s.mu.RLock()
	if cfg, ok := s.clientCache[clusterID]; ok {
		s.mu.RUnlock()
		return cfg, nil
	}
	s.mu.RUnlock()

	cert, err := s.system.GetCertificateWithSecrets(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, fmt.Errorf("cluster %d not found", clusterID)
	}
	kubeCert, err := cert.AsKubernetesCertificate()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(kubeCert.KubeConfig) == "" {
		return nil, fmt.Errorf("kubeconfig is empty")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeCert.KubeConfig))
	if err != nil {
		return nil, err
	}
	cfg.QPS = 50
	cfg.Burst = 100
	cfg.Timeout = 30 * time.Second

	s.mu.Lock()
	s.clientCache[clusterID] = cfg
	s.mu.Unlock()
	return cfg, nil
}

func resolveGVR(group, version, resource string) schema.GroupVersionResource {
	gvr := schema.GroupVersionResource{
		Group:    strings.TrimSpace(group),
		Version:  strings.TrimSpace(version),
		Resource: strings.TrimSpace(resource),
	}
	if gvr.Version == "" {
		gvr.Version = "v1"
	}
	return gvr
}

func decodeManifest(manifest, fallbackNamespace string) (*unstructured.Unstructured, string, error) {
	raw, err := yamlutil.ToJSON([]byte(manifest))
	if err != nil {
		return nil, "", err
	}
	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(raw); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return nil, "", fmt.Errorf("manifest metadata.name is required")
	}
	ns := strings.TrimSpace(obj.GetNamespace())
	if ns == "" {
		ns = strings.TrimSpace(fallbackNamespace)
	}
	return &obj, ns, nil
}

func buildObjectResponse(obj *unstructured.Unstructured) (*model.KubernetesObjectResponse, error) {
	if obj == nil {
		return nil, fmt.Errorf("object is nil")
	}
	yamlBody, err := sigyaml.Marshal(obj.Object)
	if err != nil {
		return nil, err
	}
	return &model.KubernetesObjectResponse{
		Object: obj.Object,
		YAML:   string(yamlBody),
	}, nil
}

func extractServerFromKubeconfig(kubeconfig string) string {
	if strings.TrimSpace(kubeconfig) == "" {
		return ""
	}
	cfg, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil || cfg == nil {
		return ""
	}
	for _, cluster := range cfg.Clusters {
		if cluster != nil && cluster.Server != "" {
			return cluster.Server
		}
	}
	return ""
}

func maybeDecodeBase64(value string) []byte {
	if value == "" {
		return nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) > 0 {
		return decoded
	}
	if decoded, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(value); err == nil && len(decoded) > 0 {
		return decoded
	}
	return []byte(value)
}

func collectDeploymentConfigMaps(dep *appsv1.Deployment) []string {
	if dep == nil {
		return nil
	}
	configs := map[string]struct{}{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" {
			configs[name] = struct{}{}
		}
	}
	for _, vol := range dep.Spec.Template.Spec.Volumes {
		if vol.ConfigMap != nil {
			add(vol.ConfigMap.Name)
		}
	}
	containers := append([]corev1.Container{}, dep.Spec.Template.Spec.Containers...)
	containers = append(containers, dep.Spec.Template.Spec.InitContainers...)
	for _, c := range containers {
		for _, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				add(envFrom.ConfigMapRef.Name)
			}
		}
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				add(env.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
	}
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func labelsFromSelector(sel *metav1.LabelSelector) map[string]string {
	if sel == nil {
		return nil
	}
	result := map[string]string{}
	for k, v := range sel.MatchLabels {
		result[k] = v
	}
	return result
}

func selectorMatches(target map[string]string, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if target[key] != value {
			return false
		}
	}
	return true
}

func (s *Service) selectorForWorkload(ctx context.Context, client kubernetes.Interface, kind, namespace, name string) (*metav1.LabelSelector, error) {
	switch strings.ToLower(kind) {
	case "deployment":
		deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return deploy.Spec.Selector, nil
	case "statefulset":
		sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return sts.Spec.Selector, nil
	case "daemonset":
		ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return ds.Spec.Selector, nil
	default:
		return nil, fmt.Errorf("unsupported workload kind %s", kind)
	}
}

func buildPodRow(pod *corev1.Pod) model.KubernetesPodRow {
	if pod == nil {
		return model.KubernetesPodRow{}
	}
	totalContainers := len(pod.Spec.Containers)
	readyContainers := 0
	restarts := int32(0)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			readyContainers++
		}
		restarts += cs.RestartCount
	}
	status := string(pod.Status.Phase)
	if pod.Status.Reason != "" && pod.Status.Reason != status {
		status = fmt.Sprintf("%s (%s)", status, pod.Status.Reason)
	}
	return model.KubernetesPodRow{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		Ready:      fmt.Sprintf("%d/%d", readyContainers, totalContainers),
		Status:     status,
		Restarts:   restarts,
		Node:       pod.Spec.NodeName,
		CreatedAt:  pod.GetCreationTimestamp().Unix(),
		Containers: collectContainerNames(pod),
	}
}

func collectContainerNames(pod *corev1.Pod) []string {
	if pod == nil {
		return nil
	}
	names := make([]string, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	return names
}

func responseFromObject(obj runtime.Object) (*model.KubernetesObjectResponse, error) {
	if obj == nil {
		return nil, fmt.Errorf("object is nil")
	}
	if unstr, ok := obj.(*unstructured.Unstructured); ok {
		return buildObjectResponse(unstr)
	}
	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return buildObjectResponse(&unstructured.Unstructured{Object: unstructuredMap})
}

func getObjectName(obj map[string]interface{}) string {
	if obj == nil {
		return ""
	}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		if name, ok := meta["name"].(string); ok {
			return name
		}
	}
	return ""
}
