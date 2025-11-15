import React, { useEffect } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { Spin } from 'antd';
import { getToken } from '../utils/auth';
import { useAuth } from '../context/AuthContext';

const PageLoading = () => (
  <div style={{ minHeight: '60vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
    <Spin size="large" />
  </div>
);

const RequireAuth = ({ children }) => {
  const location = useLocation();
  const token = getToken();
  const { user, loading, refresh } = useAuth();

  useEffect(() => {
    if (token && !user && !loading) {
      refresh();
    }
  }, [token, user, loading, refresh]);

  if (!token) {
    return <Navigate to={`/login?error=${encodeURIComponent('请先登录')}`} replace state={{ from: location }} />;
  }

  if (loading && !user) {
    return <PageLoading />;
  }

  return children;
};

export default RequireAuth;
