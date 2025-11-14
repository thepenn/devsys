import request from '@/utils/request'

export function listClusters() {
  return request({
    url: '/admin/k8s/clusters',
    method: 'get'
  })
}

export function listNamespaces(clusterId) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/namespaces`,
    method: 'get'
  })
}

export function listResources(clusterId, params) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources`,
    method: 'get',
    params
  })
}

export function aggregateDeployment(clusterId, namespace, name) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/deployments/${namespace}/${name}/aggregate`,
    method: 'get'
  })
}

export function listDeploymentPods(clusterId, namespace, name) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/deployments/${namespace}/${name}/pods`,
    method: 'get'
  })
}

export function listWorkloadPods(clusterId, { kind, namespace, name }) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/pods`,
    method: 'get'
  })
}

export function getWorkloadDetails(clusterId, { kind, namespace, name }) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/details`,
    method: 'get'
  })
}

export function getWorkloadHistory(clusterId, { kind, namespace, name }) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/history`,
    method: 'get'
  })
}

export function rollbackWorkload(clusterId, { kind, namespace, name, revision }) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/rollback`,
    method: 'post',
    data: { revision }
  })
}

export function getWorkloadLogs(clusterId, { kind, namespace, name, labelSelector, containers, allContainers, tail }) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/workloads/${kind}/${namespace}/${name}/logs`,
    method: 'get',
    params: {
      labelSelector,
      containers: Array.isArray(containers) ? containers.join(',') : containers,
      allContainers,
      tail
    }
  })
}

export function getResource(clusterId, params) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/object`,
    method: 'get',
    params
  })
}

export function execPod(clusterId, namespace, name, data) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/pods/${namespace}/${name}/exec`,
    method: 'post',
    data
  })
}

export function listEvents(clusterId, params) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/events`,
    method: 'get',
    params
  })
}

export function fetchPodLogs(clusterId, params) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/pods/logs`,
    method: 'get',
    params
  })
}

export function applyManifest(clusterId, data) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/apply`,
    method: 'post',
    data
  })
}

export function deleteResource(clusterId, data) {
  return request({
    url: `/admin/k8s/clusters/${clusterId}/resources/object`,
    method: 'delete',
    data
  })
}
