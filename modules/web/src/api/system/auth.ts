import request, { AUTH_PROVIDER } from '../../utils/request';

export function getCurrentUser(provider: string = AUTH_PROVIDER) {
  return request({
    url: `/auth/${provider}/me`,
    method: 'get'
  });
}

export function listProviders() {
  return request({
    url: '/auth/providers',
    method: 'get'
  });
}
