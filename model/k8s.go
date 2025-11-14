package model

// KubernetesClusterSummary describes a registered cluster.
type KubernetesClusterSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Server  string `json:"server"`
	Scope   string `json:"scope"`
	Updated int64  `json:"updated"`
}

// KubernetesNamespace describes a namespace entry.
type KubernetesNamespace struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

// KubernetesResourceQuery captures resource query parameters.
type KubernetesResourceQuery struct {
	Group         string `json:"group"`
	Version       string `json:"version"`
	Resource      string `json:"resource"`
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	LabelSelector string `json:"label_selector"`
	FieldSelector string `json:"field_selector"`
}

// KubernetesManifestRequest carries manifest payload for apply operations.
type KubernetesManifestRequest struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Manifest  string `json:"manifest"`
}

// KubernetesResourceDeleteRequest describes delete parameters.
type KubernetesResourceDeleteRequest struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// KubernetesObjectResponse wraps a single resource object along with its YAML representation.
type KubernetesObjectResponse struct {
	Object map[string]interface{} `json:"object"`
	YAML   string                 `json:"yaml"`
}

// KubernetesAggregateResponse bundles multiple objects.
type KubernetesAggregateResponse struct {
	Objects []KubernetesObjectResponse `json:"objects"`
}

// KubernetesEvent describes a single event entry.
type KubernetesEvent struct {
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Count          int32  `json:"count"`
	FirstTimestamp int64  `json:"first_timestamp"`
	LastTimestamp  int64  `json:"last_timestamp"`
}

// KubernetesEventPage response.
type KubernetesEventPage struct {
	Items []KubernetesEvent `json:"items"`
	Page  int               `json:"page"`
	Total int64             `json:"total"`
}

// KubernetesPodSummary describes pod info for log viewing.
type KubernetesPodSummary struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Status     string   `json:"status"`
	Containers []string `json:"containers"`
}

// KubernetesLogResponse wraps pod logs.
type KubernetesLogResponse struct {
	Content string `json:"content"`
}

// KubernetesWorkloadDetails groups related resources for a workload.
type KubernetesWorkloadDetails struct {
	Workload   KubernetesNamedResource     `json:"workload"`
	Overview   KubernetesWorkloadOverview  `json:"overview"`
	Pods       []KubernetesPodRow          `json:"pods"`
	Services   []KubernetesNamedResource   `json:"services"`
	Endpoints  []KubernetesNamedResource   `json:"endpoints"`
	Ingresses  []KubernetesNamedResource   `json:"ingresses"`
	ConfigMaps []KubernetesNamedResource   `json:"configmaps"`
	Secrets    []KubernetesNamedResource   `json:"secrets"`
	Volumes    []KubernetesVolumeReference `json:"volumes"`
	PVCs       []KubernetesNamedResource   `json:"pvcs"`
}

// KubernetesWorkloadOverview summarizes the primary workload object.
type KubernetesWorkloadOverview struct {
	Kind              string                       `json:"kind"`
	Name              string                       `json:"name"`
	Namespace         string                       `json:"namespace"`
	Labels            map[string]string            `json:"labels"`
	Annotations       map[string]string            `json:"annotations"`
	Selector          map[string]string            `json:"selector"`
	Strategy          KubernetesWorkloadStrategy   `json:"strategy"`
	Replica           KubernetesReplicaStatus      `json:"replica"`
	Conditions        []KubernetesCondition        `json:"conditions"`
	Containers        []KubernetesContainerSummary `json:"containers"`
	CreationTimestamp int64                        `json:"creation_timestamp"`
	UpdateTimestamp   int64                        `json:"update_timestamp"`
}

// KubernetesReplicaStatus describes desired/ready replicas.
type KubernetesReplicaStatus struct {
	Desired   int32 `json:"desired"`
	Ready     int32 `json:"ready"`
	Available int32 `json:"available"`
	Updated   int32 `json:"updated"`
}

// KubernetesWorkloadStrategy describes rollout strategy info.
type KubernetesWorkloadStrategy struct {
	Type           string `json:"type"`
	MaxSurge       string `json:"max_surge,omitempty"`
	MaxUnavailable string `json:"max_unavailable,omitempty"`
	Partition      *int32 `json:"partition,omitempty"`
}

// KubernetesCondition represents a condition summary.
type KubernetesCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastTransitionTime int64  `json:"last_transition_time"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

// KubernetesContainerSummary summarizes container spec/status.
type KubernetesContainerSummary struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Command   []string `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	Ports     []string `json:"ports,omitempty"`
	Liveness  bool     `json:"liveness_probe"`
	Readiness bool     `json:"readiness_probe"`
	Env       []string `json:"env,omitempty"`
	Init      bool     `json:"init"`
}

// KubernetesWorkloadHistoryEntry describes a historical revision entry.
type KubernetesWorkloadHistoryEntry struct {
	Revision  int64    `json:"revision"`
	Images    []string `json:"images"`
	CreatedAt int64    `json:"created_at"`
	Source    string   `json:"source"`
}

// KubernetesWorkloadRollbackRequest describes rollback input.
type KubernetesWorkloadRollbackRequest struct {
	Revision int64 `json:"revision"`
}

// KubernetesNamedResource provides minimal metadata for a resource.
type KubernetesNamedResource struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels,omitempty"`
	Type       string            `json:"type,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Group      string            `json:"group,omitempty"`
	Version    string            `json:"version,omitempty"`
	Resource   string            `json:"resource,omitempty"`
	Namespaced bool              `json:"namespaced"`
}

// KubernetesVolumeReference describes a volume reference in a pod template.
type KubernetesVolumeReference struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	SourceName string `json:"source_name"`
}

// KubernetesPodRow describes pod data for workload sub tables.
type KubernetesPodRow struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Ready      string   `json:"ready"`
	Status     string   `json:"status"`
	Restarts   int32    `json:"restarts"`
	Node       string   `json:"node"`
	CreatedAt  int64    `json:"created_at"`
	Containers []string `json:"containers"`
}

// KubernetesPodExecRequest represents a remote exec invocation.
type KubernetesPodExecRequest struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Container string   `json:"container"`
	Command   []string `json:"command"`
	TTY       bool     `json:"tty"`
}

// KubernetesPodExecResult contains exec output.
type KubernetesPodExecResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}
