import React from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Button, Card } from 'antd';

const K8sClusterGuard = ({ children }) => {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const clusterId = params.get('cluster');

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
