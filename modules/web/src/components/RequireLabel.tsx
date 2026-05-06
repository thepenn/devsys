import React from 'react';
import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { Result, Spin } from 'antd';
import RequireAuth from './RequireAuth';
import { useAuth } from '../context/AuthContext';

const PageLoading = () => (
  <div style={{ minHeight: '60vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
    <Spin size="large" />
  </div>
);

const Forbidden = () => (
  <Result
    status="403"
    title="403"
    subTitle="您没有访问该页面的权限, 请联系管理员分配相应角色."
  />
);

type LabelFallback = 'forbidden' | 'redirect';

const LabelGate = ({
  labels = [],
  fallback = 'forbidden' as LabelFallback,
  children
}: {
  labels?: string[];
  fallback?: LabelFallback;
  children: ReactNode;
}) => {
  const location = useLocation();
  const { hasAnyLabel, loading } = useAuth();

  if (loading) {
    return <PageLoading />;
  }

  if (labels.length === 0 || hasAnyLabel(labels)) {
    return <>{children}</>;
  }

  if (fallback === 'redirect') {
    return <Navigate to="/ops" replace state={{ from: location }} />;
  }
  return <Forbidden />;
};

const RequireLabel = ({
  labels,
  fallback,
  children
}: {
  labels: string[];
  fallback?: LabelFallback;
  children: ReactNode;
}) => (
  <RequireAuth>
    <LabelGate labels={labels} fallback={fallback}>
      {children}
    </LabelGate>
  </RequireAuth>
);

export default RequireLabel;
