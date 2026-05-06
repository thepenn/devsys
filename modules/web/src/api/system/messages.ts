import request from '../../utils/request';

export function listMessages(params: Record<string, unknown>) {
  return request({
    url: '/messages',
    method: 'get',
    params
  });
}

export function unreadCount() {
  return request({
    url: '/messages/unread-count',
    method: 'get'
  });
}

export function markRead(ids: unknown[]) {
  return request({
    url: '/messages/read',
    method: 'post',
    data: { ids }
  });
}

export function markAllRead() {
  return request({
    url: '/messages/read-all',
    method: 'post'
  });
}
