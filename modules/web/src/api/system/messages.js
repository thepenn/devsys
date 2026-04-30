import request from '../../utils/request';

// listMessages 查询当前用户的消息. params: page, per_page, unread (boolean).
export function listMessages(params) {
  return request({
    url: '/messages',
    method: 'get',
    params
  });
}

// unreadCount 全局头部 badge 用; 比 listMessages 轻量.
export function unreadCount() {
  return request({
    url: '/messages/unread-count',
    method: 'get'
  });
}

// markRead 批量标记已读.
export function markRead(ids) {
  return request({
    url: '/messages/read',
    method: 'post',
    data: { ids }
  });
}

// markAllRead 一键全部标记已读.
export function markAllRead() {
  return request({
    url: '/messages/read-all',
    method: 'post'
  });
}
