import React, { useEffect } from 'react';
import { HashRouter } from 'react-router-dom';
import AppRoutes from './router';
import { syncTokenFromUrl } from './utils/auth';
import { AuthProvider } from './context/AuthContext';

const App = () => {
  useEffect(() => {
    syncTokenFromUrl();
  }, []);

  return (
    <HashRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </HashRouter>
  );
};

export default App;
