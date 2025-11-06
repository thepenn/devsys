import request from '@/utils/request'

export function listRepositories(params) {
  return request({
    url: '/repos',
    method: 'get',
    params
  })
}

export function syncRepositories() {
  return request({
    url: '/repos/sync',
    method: 'post'
  })
}

export function syncRepository(remoteId) {
  return request({
    url: `/repos/${encodeURIComponent(remoteId)}/sync`,
    method: 'post'
  })
}
