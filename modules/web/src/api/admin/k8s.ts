import request from '../../utils/request';

export function listClusters() {
  return request({
    url: '/admin/k8s/clusters',
    method: 'get'
  });
}

export function listNamespaces(clusterId: string | number) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/namespaces`,
    method: 'get'
  });
}

export function listResources(clusterId: string | number, params: Record<string, unknown>) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources`,
    method: 'get',
    params
  });
}

export function getResource(clusterId: string | number, params: Record<string, unknown>) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/object`,
    method: 'get',
    params
  });
}

export function applyManifest(clusterId: string | number, data: unknown) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/apply`,
    method: 'post',
    data
  });
}

export function deleteResource(clusterId: string | number, data: unknown) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/object`,
    method: 'delete',
    data
  });
}

export function listWorkloadPods(
  clusterId: string | number,
  payload: { kind: string; namespace: string; name: string }
) {
  const { kind, namespace, name } = payload;
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/pods`,
    method: 'get'
  });
}

export function getWorkloadDetails(
  clusterId: string | number,
  payload: { kind: string; namespace: string; name: string }
) {
  const { kind, namespace, name } = payload;
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/details`,
    method: 'get'
  });
}

export function getWorkloadHistory(
  clusterId: string | number,
  payload: { kind: string; namespace: string; name: string }
) {
  const { kind, namespace, name } = payload;
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/history`,
    method: 'get'
  });
}

export function rollbackWorkload(
  clusterId: string | number,
  payload: { kind: string; namespace: string; name: string; revision: unknown }
) {
  const { kind, namespace, name, revision } = payload;
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/rollback`,
    method: 'post',
    data: { revision }
  });
}

export function getWorkloadLogs(
  clusterId: string | number,
  payload: {
    kind: string;
    namespace: string;
    name: string;
    labelSelector?: unknown;
    containers?: unknown;
    allContainers?: unknown;
    tail?: unknown;
  }
) {
  const { kind, namespace, name, labelSelector, containers, allContainers, tail } = payload;
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/logs`,
    method: 'get',
    params: {
      labelSelector,
      containers: Array.isArray(containers) ? containers.join(',') : containers,
      allContainers,
      tail
    }
  });
}

export function listResourceEvents(
  clusterId: string | number,
  payload: { namespace?: unknown; kind?: unknown; name?: unknown; page?: unknown; perPage?: unknown }
) {
  const { namespace, kind, name, page, perPage } = payload;
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/events`,
    method: 'get',
    params: {
      namespace,
      kind,
      name,
      page,
      perPage
    }
  });
}

export function getPodLogs(clusterId: string | number, params: Record<string, unknown>) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/pods/logs`,
    method: 'get',
    params
  });
}
