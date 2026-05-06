// 前端 RBAC 工具函数. 与后端 internal/label/label.go 中的常量对应:
//   - `*`        通配标签 (superadmin)
//   - `k8s:read` 等具体标签
// 由 `/me` 接口下发后端预计算的 effective label 集合到 user.labels.

export const WILDCARD = '*';

export interface AuthUser {
  labels?: string[];
  roles?: string[];
  login?: string;
  email?: string;
  name?: string;
  provider?: string;
  avatar_url?: string;
}

export const hasLabel = (user: AuthUser | null | undefined, labelName: string | undefined | null) => {
  if (!labelName) return true;
  const labels = (user && user.labels) || [];
  if (labels.includes(WILDCARD)) return true;
  return labels.includes(labelName);
};

export const hasAnyLabel = (user: AuthUser | null | undefined, labelList: string[] | undefined | null) => {
  if (!labelList || labelList.length === 0) return true;
  const labels = (user && user.labels) || [];
  if (labels.includes(WILDCARD)) return true;
  return labelList.some(name => labels.includes(name));
};

export const isSuperAdmin = (user: AuthUser | null | undefined) => {
  const labels = (user && user.labels) || [];
  return labels.includes(WILDCARD);
};

export const hasRole = (user: AuthUser | null | undefined, roleName: string | undefined | null) => {
  if (!roleName) return false;
  const roles = (user && user.roles) || [];
  return roles.includes(roleName);
};
