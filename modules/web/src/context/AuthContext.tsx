import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { getCurrentUser, listProviders } from 'api/system/auth';
import { getToken } from 'utils/auth';
import {
  hasAnyLabel as hasAnyLabelHelper,
  hasLabel as hasLabelHelper,
  hasRole as hasRoleHelper,
  isSuperAdmin as isSuperAdminHelper,
  type AuthUser
} from 'utils/permission';

export interface AuthContextValue {
  user: AuthUser | null;
  loading: boolean;
  refresh: () => Promise<AuthUser | null>;
  isAdmin: boolean;
  isSuperAdmin: boolean;
  roles: string[];
  labels: string[];
  hasLabel: (name: string | null | undefined) => boolean;
  hasAnyLabel: (list: string[]) => boolean;
  hasRole: (name: string | null | undefined) => boolean;
}

const defaultValue: AuthContextValue = {
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
};

const AuthContext = createContext<AuthContextValue>(defaultValue);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<AuthUser | null>(null);
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
      const providersResp = (await listProviders()) as {
        active?: string;
        providers?: { name: string }[];
      };
      const provider = providersResp?.active || providersResp?.providers?.[0]?.name;
      const info = (await getCurrentUser(provider)) as AuthUser | null;
      setUser(info || null);
      return info || null;
    } catch {
      setUser(null);
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchUser();
  }, [fetchUser]);

  const value = useMemo((): AuthContextValue => {
    const roles = (user && user.roles) || [];
    const labels = (user && user.labels) || [];
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
