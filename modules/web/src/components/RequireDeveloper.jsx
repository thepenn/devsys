import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import RequireAuth from './RequireAuth';
import { useAuth } from '../context/AuthContext';

const DeveloperGate = ({ children }) => {
  const location = useLocation();
  const { isAdmin } = useAuth();

  if (isAdmin) {
    return <Navigate to="/ops" replace state={{ from: location }} />;
  }
  return children;
};

const RequireDeveloper = ({ children }) => (
  <RequireAuth>
    <DeveloperGate>{children}</DeveloperGate>
  </RequireAuth>
);

export default RequireDeveloper;
