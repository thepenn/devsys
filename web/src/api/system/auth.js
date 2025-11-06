import request from '@/utils/request'

export function getCurrentUser() {
  return request({
    url: '/auth/gitlab/me',
    method: 'get'
  })
}
