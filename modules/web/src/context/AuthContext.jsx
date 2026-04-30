import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { getCurrentUser, listProviders } from 'api/system/auth';
import { getToken } from 'utils/auth';
import {
  hasAnyLabel as hasAnyLabelHelper,
  hasLabel as hasLabelHelper,
  hasRole as hasRoleHelper,
  isSuperAdmin as isSuperAdminHelper
} from 'utils/permission';

const AuthContext = createContext({
  user: null,
  loading: false,
  refresh: async () => null,
  isAdmin: false,
  isSuperAdmin: false,
  roles: [],
  labels: [],
  hasLabel: () => false,
  hasAnyLabel: () => false,
  hasRole: () => false
});

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);

  const fetchUser = useCallback(async () => {
    const token = getToken();
    if (!token) {
      setUser(null);
      setLoading(false);
      return null;
    }
    setLoading(true);
    try {
      const providersResp = await listProviders();
      const provider = providersResp?.active || providersResp?.providers?.[0]?.name;
      const info = await getCurrentUser(provider);
      setUser(info || null);
      return info || null;
    } catch (err) {
      setUser(null);
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchUser();
  }, [fetchUser]);

  const value = useMemo(() => {
    const roles = (user && user.roles) || [];
    const labels = (user && user.labels) || [];
    // RBAC 是唯一权限来源: isAdmin 等价于 isSuperAdmin (拥有通配 label `*`).
    // 后端 OAuth 同步的 user.admin 字段只用作"首次登录是否自动晋升 superadmin"
    // 的输入信号, 不再参与前端鉴权判断.
    const superAdmin = isSuperAdminHelper(user);
    return {
      user,
      loading,
      refresh: fetchUser,
      isAdmin: superAdmin,
      isSuperAdmin: superAdmin,
      roles,
      labels,
      hasLabel: name => hasLabelHelper(user, name),
      hasAnyLabel: list => hasAnyLabelHelper(user, list),
      hasRole: name => hasRoleHelper(user, name)
    };
  }, [user, loading, fetchUser]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = () => useContext(AuthContext);
