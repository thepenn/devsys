// 前端 RBAC 工具函数. 与后端 internal/label/label.go 中的常量对应:
//   - `*`        通配标签 (superadmin)
//   - `k8s:read` 等具体标签
// 由 `/me` 接口下发后端预计算的 effective label 集合到 user.labels.
// 任意 React 上下文之外 (例如菜单常量过滤) 都可以直接使用本模块.

export const WILDCARD = '*';

export const hasLabel = (user, labelName) => {
  if (!labelName) return true;
  const labels = (user && user.labels) || [];
  if (labels.includes(WILDCARD)) return true;
  return labels.includes(labelName);
};

export const hasAnyLabel = (user, labelList) => {
  if (!labelList || labelList.length === 0) return true;
  const labels = (user && user.labels) || [];
  if (labels.includes(WILDCARD)) return true;
  return labelList.some(name => labels.includes(name));
};

export const isSuperAdmin = user => {
  const labels = (user && user.labels) || [];
  return labels.includes(WILDCARD);
};

export const hasRole = (user, roleName) => {
  if (!roleName) return false;
  const roles = (user && user.roles) || [];
  return roles.includes(roleName);
};
