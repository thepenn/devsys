import request, { AUTH_PROVIDER } from '../../utils/request';

export function getCurrentUser(provider = AUTH_PROVIDER) {
  return request({
    url: `/auth/${provider}/me`,
    method: 'get'
  });
}

// listProviders 返回 { active, providers: [{name, display_name}] }.
// Login.jsx 用它动态渲染按钮, 不再依赖编译期 REACT_APP_AUTH_PROVIDER.
export function listProviders() {
  return request({
    url: '/auth/providers',
    method: 'get'
  });
}
