import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { getCurrentUser } from 'api/system/auth';
import { getToken } from 'utils/auth';

const AuthContext = createContext({
  user: null,
  loading: false,
  refresh: async () => null,
  isAdmin: false
});

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(false);

  const fetchUser = useCallback(async () => {
    if (!getToken()) {
      setUser(null);
      return null;
    }
    setLoading(true);
    try {
      const info = await getCurrentUser();
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

  const value = {
    user,
    loading,
    refresh: fetchUser,
    isAdmin: Boolean(user?.admin)
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = () => useContext(AuthContext);
