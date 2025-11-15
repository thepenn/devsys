import React, { useMemo } from 'react';
import { useRoutes, Navigate } from 'react-router-dom';
import RequireDeveloper from '../components/RequireDeveloper';
import RequireAdmin from '../components/RequireAdmin';
import LoginPage from '../views/login/Login';
import DevLayout from '../views/dev/DevLayout';
import DashboardPage from '../views/dev/dashboard/Dashboard';
import ProfilePage from '../views/dev/profile/Profile';
import ProjectLayout from '../views/dev/project/ProjectLayout';
import ProjectPipeline from '../views/dev/project/ProjectPipeline';
import ProjectRunDetail from '../views/dev/project/ProjectRunDetail';
import ProjectPlaceholder from '../views/dev/project/ProjectPlaceholder';
import OpsLayout from '../views/ops/OpsLayout';
import K8sClusters from '../views/ops/k8s/ClusterList';
import K8sWorkloads from '../views/ops/k8s/Workloads';
import K8sServices from '../views/ops/k8s/Services';
import K8sPods from '../views/ops/k8s/Pods';
import K8sJobs from '../views/ops/k8s/Jobs';
import K8sVolumes from '../views/ops/k8s/Volumes';
import K8sNodes from '../views/ops/k8s/Nodes';
import K8sMonitor from '../views/ops/k8s/Monitor';
import ProjectList from '../views/ops/project/ProjectList';
import ProjectBuild from '../views/ops/project/ProjectBuild';
import ProjectMonitor from '../views/ops/project/ProjectMonitor';
import MessageNotification from '../views/ops/notice/MessageNotification';
import AlertManagement from '../views/ops/notice/AlertManagement';
import DatabaseMySQL from '../views/ops/database/DatabaseMySQL';
import DatabaseRedis from '../views/ops/database/DatabaseRedis';
import DatabaseMongo from '../views/ops/database/DatabaseMongo';
import DatabasePostgres from '../views/ops/database/DatabasePostgres';
import SystemCertificate from '../views/ops/system/Certificate';
import SystemRoles from '../views/ops/system/Roles';
import SystemAudit from '../views/ops/system/Audit';
import SystemProfile from '../views/ops/system/Profile';
import { useAuth } from '../context/AuthContext';

const AppRoutes = () => {
  const { isAdmin } = useAuth();
  const landingPath = isAdmin ? '/ops' : '/dev';

  const routes = useMemo(
    () => [
      { path: '/', element: <Navigate to={landingPath} replace /> },
      { path: '/login', element: <LoginPage /> },
      {
        path: '/dev',
        element: (
          <RequireDeveloper>
            <DevLayout />
          </RequireDeveloper>
        ),
        children: [
          { index: true, element: <Navigate to="dashboard" replace /> },
          { path: 'dashboard', element: <DashboardPage /> },
          { path: 'profile', element: <ProfilePage /> },
          {
            path: 'projects/:owner/:name',
            element: <ProjectLayout />,
            children: [
              { index: true, element: <Navigate to="pipeline" replace /> },
              { path: 'pipeline', element: <ProjectPipeline /> },
              { path: 'pipeline/:runId', element: <ProjectRunDetail /> },
              { path: 'deployment', element: <ProjectPlaceholder section="deployment" /> },
              { path: 'monitor', element: <ProjectPlaceholder section="monitor" /> },
              { path: '*', element: <Navigate to="pipeline" replace /> }
            ]
          }
        ]
      },
      {
        path: '/ops',
        element: (
          <RequireAdmin>
            <OpsLayout />
          </RequireAdmin>
        ),
        children: [
          { index: true, element: <Navigate to="k8s/clusters" replace /> },
          { path: 'k8s/clusters', element: <K8sClusters /> },
          { path: 'k8s/workloads', element: <K8sWorkloads /> },
          { path: 'k8s/services', element: <K8sServices /> },
          { path: 'k8s/pods', element: <K8sPods /> },
          { path: 'k8s/jobs', element: <K8sJobs /> },
          { path: 'k8s/volumes', element: <K8sVolumes /> },
          { path: 'k8s/nodes', element: <K8sNodes /> },
          { path: 'k8s/monitor', element: <K8sMonitor /> },
          { path: 'profile', element: <SystemProfile /> },
          { path: 'projects/list', element: <ProjectList /> },
          { path: 'projects/pipeline', element: <ProjectBuild /> },
          { path: 'projects/monitor', element: <ProjectMonitor /> },
          { path: 'messages/notification', element: <MessageNotification /> },
          { path: 'messages/alert', element: <AlertManagement /> },
          { path: 'db/mysql', element: <DatabaseMySQL /> },
          { path: 'db/redis', element: <DatabaseRedis /> },
          { path: 'db/mongo', element: <DatabaseMongo /> },
          { path: 'db/postgres', element: <DatabasePostgres /> },
          { path: 'system/credentials', element: <SystemCertificate /> },
          { path: 'system/roles', element: <SystemRoles /> },
          { path: 'system/audit', element: <SystemAudit /> }
        ]
      },
      { path: '*', element: <Navigate to="/login" replace /> }
    ],
    [landingPath]
  );

  const element = useRoutes(routes);

  return element;
};

export default AppRoutes;
