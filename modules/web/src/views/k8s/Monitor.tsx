import React from 'react';
import OpsPlaceholder from '../pages/OpsPlaceholder';
import K8sClusterGuard from './K8sClusterGuard';

const K8sMonitor = () => (
  <K8sClusterGuard>
    {clusterId => <OpsPlaceholder title={`K8s · 集群监控（集群 ${clusterId}）`} />}
  </K8sClusterGuard>
);

export default K8sMonitor;
