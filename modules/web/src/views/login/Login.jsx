import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Space, Spin } from 'antd';
import { useNavigate, useLocation } from 'react-router-dom';
import './login.less';
import { getToken } from 'utils/auth';
import { useAuth } from 'context/AuthContext';
import { AUTH_BASE_URL } from 'utils/request';
import { listProviders } from 'api/system/auth';

const LoginPage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, loading, refresh } = useAuth();
  const searchParams = useMemo(() => new URLSearchParams(location.search), [location.search]);
  const [error, setError] = useState(searchParams.get('error') || '');
  const [pending, setPending] = useState('');
  const [providers, setProviders] = useState([]);
  const [providersLoading, setProvidersLoading] = useState(true);

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
      navigate('/ops', { replace: true });
    }
  }, [user, loading, refresh, navigate]);

  // 启动时拉取后端激活的 OAuth provider 列表; 失败不阻塞用户操作 (回退到只显示
  // 一个泛化的"使用 Git 登录"按钮, 但实际跳转 URL 需要 provider 名称).
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await listProviders();
        if (!cancelled) {
          setProviders((resp && resp.providers) || []);
        }
      } catch (err) {
        if (!cancelled) {
          setError(prev => prev || '无法获取登录方式, 请检查后端配置');
        }
      } finally {
        if (!cancelled) {
          setProvidersLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleLogin = useCallback(name => {
    setPending(name);
    try {
      const redirect = `${window.location.origin}${window.location.pathname}#/ops`;
      const loginUrl = `${AUTH_BASE_URL}/auth/${name}/login?redirect=${encodeURIComponent(redirect)}`;
      window.location.href = loginUrl;
    } catch (err) {
      setError(err.message || '无法发起登录请求');
      setPending('');
    }
  }, []);

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
        {providersLoading ? (
          <div style={{ textAlign: 'center', padding: '16px 0' }}>
            <Spin />
          </div>
        ) : providers.length === 0 ? (
          <Alert
            type="warning"
            showIcon
            message="未检测到可用的登录方式"
            description="请联系管理员在后端配置 OAuth provider (SERVER_AUTH_PROVIDER) 并启用对应的 Git 集成."
          />
        ) : (
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {providers.map(p => (
              <Button
                key={p.name}
                type="primary"
                block
                size="large"
                loading={pending === p.name}
                onClick={() => handleLogin(p.name)}
              >
                {pending === p.name ? '跳转中…' : `使用 ${p.display_name || p.name} 登录`}
              </Button>
            ))}
          </Space>
        )}
      </Card>
    </div>
  );
};

export default LoginPage;
