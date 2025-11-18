import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { Spin } from 'antd';
import RequireAuth from './RequireAuth';
import { useAuth } from '../context/AuthContext';

const PageLoading = () => (
  <div style={{ minHeight: '60vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
    <Spin size="large" />
  </div>
);

const AdminGate = ({ children }) => {
  const location = useLocation();
  const { isAdmin, loading } = useAuth();

  if (loading) {
    return <PageLoading />;
  }

  if (!isAdmin) {
    return <Navigate to="/dev/dashboard" replace state={{ from: location }} />;
  }
  return children;
};

const RequireAdmin = ({ children }) => (
  <RequireAuth>
    <AdminGate>{children}</AdminGate>
  </RequireAuth>
);

export default RequireAdmin;
