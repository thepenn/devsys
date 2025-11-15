import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import RequireAuth from './RequireAuth';
import { useAuth } from '../context/AuthContext';

const AdminGate = ({ children }) => {
  const location = useLocation();
  const { isAdmin } = useAuth();

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
