import request from '../../utils/request';

export function listRoles() {
  return request({ url: '/rbac/roles', method: 'get' });
}

export function createRole(data: unknown) {
  return request({ url: '/rbac/roles', method: 'post', data });
}

export function updateRole(id: string | number, data: unknown) {
  return request({ url: `/rbac/roles/${id}`, method: 'put', data });
}

export function deleteRole(id: string | number) {
  return request({ url: `/rbac/roles/${id}`, method: 'delete' });
}

export function listLabels() {
  return request({ url: '/rbac/labels', method: 'get' });
}

export function listEndpoints() {
  return request({ url: '/rbac/endpoints', method: 'get' });
}

export function listUserRoles() {
  return request({ url: '/rbac/users', method: 'get' });
}

export function assignUserRoles(userId: string | number, roles: unknown[]) {
  return request({
    url: `/rbac/users/${userId}/roles`,
    method: 'put',
    data: { roles }
  });
}
