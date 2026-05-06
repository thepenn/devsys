import request from '../../utils/request';

export function listRepositories(params: Record<string, unknown>) {
  return request({
    url: '/repos',
    method: 'get',
    params
  });
}

export function syncRepositories() {
  return request({
    url: '/repos/sync',
    method: 'post'
  });
}

export function syncRepository(remoteId: string | number) {
  return request({
    url: `/repos/${encodeURIComponent(String(remoteId))}/sync`,
    method: 'post'
  });
}
