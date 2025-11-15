import React from 'react';
import OpsPlaceholder from '../pages/OpsPlaceholder';
import K8sClusterGuard from './K8sClusterGuard';

const K8sVolumes = () => (
  <K8sClusterGuard>
    {clusterId => <OpsPlaceholder title={`K8s · Volumes（集群 ${clusterId}）`} />}
  </K8sClusterGuard>
);

export default K8sVolumes;
