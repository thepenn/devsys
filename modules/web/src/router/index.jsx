import React from 'react';
import { useRoutes, Navigate } from 'react-router-dom';
import RequireAuth from '../components/RequireAuth';
import RequireLabel from '../components/RequireLabel';
import LoginPage from '../views/login/Login';
import AppLayout from '../views/sidebar/Layout';
import K8sClusters from '../views/k8s/ClusterList';
import K8sWorkloads from '../views/k8s/Workloads';
import K8sServices from '../views/k8s/Services';
import K8sPods from '../views/k8s/Pods';
import K8sJobs from '../views/k8s/Jobs';
import K8sVolumes from '../views/k8s/Volumes';
import K8sNodes from '../views/k8s/Nodes';
import K8sMonitor from '../views/k8s/Monitor';
import ProjectList from '../views/project/ProjectList';
import ProjectBuild from '../views/project/ProjectBuild';
import ProjectMonitor from '../views/project/ProjectMonitor';
import ProjectBuildDetail from '../views/project/ProjectBuildDetail';
import MessageNotification from '../views/notice/MessageNotification';
import AlertManagement from '../views/notice/AlertManagement';
import DatabaseMySQL from '../views/database/DatabaseMySQL';
import DatabaseRedis from '../views/database/DatabaseRedis';
import DatabaseMongo from '../views/database/DatabaseMongo';
import DatabasePostgres from '../views/database/DatabasePostgres';
import SystemCertificate from '../views/system/Certificate';
import SystemRoles from '../views/system/Roles';
import SystemAudit from '../views/system/Audit';
import SystemProfile from '../views/system/Profile';
import PipelineTemplateList from '../views/pipelineTemplates/PipelineTemplateList';
import PipelineTemplateEditor from '../views/pipelineTemplates/PipelineTemplateEditor';
import PipelineJobList from '../views/pipelineJobs/PipelineJobList';
import PipelineJobEditor from '../views/pipelineJobs/PipelineJobEditor';
import PipelineJobRunDetail from '../views/pipelineJobs/PipelineJobRunDetail';

// labels 与后端 internal/label/label.go 中的常量一一对应
const LBL = {
  K8sRead: 'k8s:read',
  ProjectRead: 'project:read',
  MessageRead: 'message:read',
  AlertRead: 'alert:read',
  AlertWrite: 'alert:write',
  DBRead: 'db:read',
  SystemCertificate: 'system:certificate',
  SystemRoleWrite: 'system:role_write',
  SystemAudit: 'system:audit',
  PipelineTemplateRead: 'pipeline_template:read',
  PipelineTemplateWrite: 'pipeline_template:write',
  PipelineJobRead: 'pipeline_job:read',
  PipelineJobWrite: 'pipeline_job:write',
  PipelineJobTrigger: 'pipeline_job:trigger'
};

const guard = (labels, element) => <RequireLabel labels={labels}>{element}</RequireLabel>;

const AppRoutes = () => {
  return useRoutes([
    { path: '/login', element: <LoginPage /> },
    { path: '/dev/*', element: <Navigate to="/ops" replace /> },
    {
      path: '/ops',
      element: (
        <RequireAuth>
          <AppLayout />
        </RequireAuth>
      ),
      children: [
        { index: true, element: <Navigate to="k8s/clusters" replace /> },
        { path: 'k8s/clusters', element: guard([LBL.K8sRead], <K8sClusters />) },
        { path: 'k8s/workloads', element: guard([LBL.K8sRead], <K8sWorkloads />) },
        { path: 'k8s/services', element: guard([LBL.K8sRead], <K8sServices />) },
        { path: 'k8s/pods', element: guard([LBL.K8sRead], <K8sPods />) },
        { path: 'k8s/jobs', element: guard([LBL.K8sRead], <K8sJobs />) },
        { path: 'k8s/volumes', element: guard([LBL.K8sRead], <K8sVolumes />) },
        { path: 'k8s/nodes', element: guard([LBL.K8sRead], <K8sNodes />) },
        { path: 'k8s/monitor', element: guard([LBL.K8sRead], <K8sMonitor />) },
        { path: 'profile', element: <SystemProfile /> },
        { path: 'projects/list', element: guard([LBL.ProjectRead], <ProjectList />) },
        { path: 'projects/pipeline', element: guard([LBL.ProjectRead], <ProjectBuild />) },
        { path: 'projects/build/:repoId/:runId', element: guard([LBL.ProjectRead], <ProjectBuildDetail />) },
        { path: 'projects/monitor', element: guard([LBL.ProjectRead], <ProjectMonitor />) },
        { path: 'pipeline-templates', element: guard([LBL.PipelineTemplateRead], <PipelineTemplateList />) },
        { path: 'pipeline-templates/:id', element: guard([LBL.PipelineTemplateRead], <PipelineTemplateEditor />) },
        { path: 'pipeline-jobs', element: guard([LBL.PipelineJobRead], <PipelineJobList />) },
        { path: 'pipeline-jobs/:id', element: guard([LBL.PipelineJobRead], <PipelineJobEditor />) },
        { path: 'pipeline-jobs/:id/runs/:runId', element: guard([LBL.PipelineJobRead], <PipelineJobRunDetail />) },
        { path: 'messages/notification', element: guard([LBL.MessageRead], <MessageNotification />) },
        { path: 'messages/alert', element: guard([LBL.AlertRead, LBL.AlertWrite], <AlertManagement />) },
        { path: 'db/mysql', element: guard([LBL.DBRead], <DatabaseMySQL />) },
        { path: 'db/redis', element: guard([LBL.DBRead], <DatabaseRedis />) },
        { path: 'db/mongo', element: guard([LBL.DBRead], <DatabaseMongo />) },
        { path: 'db/postgres', element: guard([LBL.DBRead], <DatabasePostgres />) },
        { path: 'system/credentials', element: guard([LBL.SystemCertificate], <SystemCertificate />) },
        { path: 'system/roles', element: guard([LBL.SystemRoleWrite], <SystemRoles />) },
        { path: 'system/audit', element: guard([LBL.SystemAudit], <SystemAudit />) }
      ]
    },
    { path: '/', element: <Navigate to="/ops" replace /> },
    { path: '*', element: <Navigate to="/" replace /> }
  ]);
};

export default AppRoutes;
