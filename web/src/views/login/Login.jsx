import React, { useMemo, useState, useEffect } from 'react';
import { Button, Alert, Card } from 'antd';
import { useNavigate, useLocation } from 'react-router-dom';
import './login.less';
import { getToken } from 'utils/auth';
import { useAuth } from 'context/AuthContext';
import { AUTH_BASE_URL } from 'utils/request';

const LoginPage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, loading, refresh, isAdmin } = useAuth();
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search]);
  const [error, setError] = useState(searchParams.get('error') || '');
  const [pending, setPending] = useState(false);

  useEffect(() => {
    setError(searchParams.get('error') || '');
  }, [searchParams]);

  useEffect(() => {
    const token = getToken();
    if (!token) return;
    if (!user && !loading) {
      refresh();
    }
    if (user) {
      navigate(isAdmin ? '/ops' : '/dev', { replace: true });
    }
  }, [user, loading, refresh, navigate, isAdmin]);

  const handleLogin = () => {
    setPending(true);
    try {
      const redirect = `${window.location.origin}${window.location.pathname}#/dev/dashboard`;
      const loginUrl = `${AUTH_BASE_URL}/auth/gitlab/login?redirect=${encodeURIComponent(redirect)}`;
      window.location.href = loginUrl;
    } catch (err) {
      setError(err.message || '无法发起登录请求');
      setPending(false);
    }
  };

  return (
    <div className="login-page">
      {error && (
        <Alert
          type="error"
          message={error}
          showIcon
          closable
          className="login-alert"
          onClose={() => setError('')}
        />
      )}
      <Card className="login-card">
        <h1>欢迎使用 Go DevOps</h1>
        <p className="login-subtitle">使用您的 Git 账户登录以管理仓库和流水线。</p>
        <Button type="primary" block size="large" loading={pending} onClick={handleLogin}>
          {pending ? '跳转中…' : '使用 Git 登录'}
        </Button>
      </Card>
    </div>
  );
};

export default LoginPage;
