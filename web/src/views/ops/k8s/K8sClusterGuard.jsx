import React, { useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button, Card } from 'antd';

const STORAGE_KEY = 'k8s.activeCluster';

const K8sClusterGuard = ({ children }) => {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const clusterId = params.get('cluster');
  const storedCluster = typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : null;

  useEffect(() => {
    if (clusterId && typeof window !== 'undefined') {
      window.localStorage.setItem(STORAGE_KEY, clusterId);
    }
    if (!clusterId && storedCluster) {
      const next = new URLSearchParams(params);
      next.set('cluster', storedCluster);
      setParams(next, { replace: true });
    }
  }, [clusterId, storedCluster, params, setParams]);

  if (!clusterId && storedCluster) {
    return null;
  }

  if (!clusterId) {
    return (
      <Card className="cluster-guard">
        <p>请先选择一个 K8s 集群。</p>
        <Button type="primary" onClick={() => navigate('/ops/k8s/clusters')}>
          前往集群列表
        </Button>
      </Card>
    );
  }

  return typeof children === 'function' ? children(clusterId) : children;
};

export default K8sClusterGuard;
