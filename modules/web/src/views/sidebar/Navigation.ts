// 单一菜单结构. 每个 child.labels 中任意一个命中用户 effective labels 即显示.
// 后端 internal/label/label.go 中的常量与此处 labels 字符串保持同步.
export const opsNavItems = [
  {
    key: 'k8s',
    label: 'K8s 管理',
    children: [
      { key: 'clusters', label: '集群列表', path: '/ops/k8s/clusters', labels: ['k8s:read'] },
      { key: 'workloads', label: '工作负载', path: '/ops/k8s/workloads', labels: ['k8s:read'] },
      { key: 'services', label: '服务路由', path: '/ops/k8s/services', labels: ['k8s:read'] },
      { key: 'pods', label: 'Pods', path: '/ops/k8s/pods', labels: ['k8s:read'] },
      { key: 'jobs', label: '计划任务', path: '/ops/k8s/jobs', labels: ['k8s:read'] },
      { key: 'volumes', label: 'Volumes', path: '/ops/k8s/volumes', labels: ['k8s:read'] },
      { key: 'nodes', label: '节点管理', path: '/ops/k8s/nodes', labels: ['k8s:read'] },
      { key: 'monitor', label: '集群监控', path: '/ops/k8s/monitor', labels: ['k8s:read'] }
    ]
  },
  {
    key: 'project',
    label: '项目管理',
    children: [
      { key: 'list', label: '项目列表', path: '/ops/projects/list', labels: ['project:read'] },
      { key: 'pipeline', label: '项目构建', path: '/ops/projects/pipeline', labels: ['project:read'] },
      { key: 'monitor', label: '项目监控', path: '/ops/projects/monitor', labels: ['project:read'] }
    ]
  },
  {
    key: 'pipeline_template',
    label: '通用 Pipeline',
    children: [
      { key: 'templates', label: '模板管理', path: '/ops/pipeline-templates', labels: ['pipeline_template:read'] },
      { key: 'jobs', label: '独立 Job', path: '/ops/pipeline-jobs', labels: ['pipeline_job:read'] }
    ]
  },
  {
    key: 'message',
    label: '消息通知',
    children: [
      { key: 'notification', label: '消息通知', path: '/ops/messages/notification', labels: ['message:read'] },
      { key: 'alert', label: '告警管理', path: '/ops/messages/alert', labels: ['alert:read', 'alert:write'] }
    ]
  },
  {
    key: 'database',
    label: '数据库管理',
    children: [
      { key: 'mysql', label: 'MySQL', path: '/ops/db/mysql', labels: ['db:read'] },
      { key: 'redis', label: 'Redis', path: '/ops/db/redis', labels: ['db:read'] },
      { key: 'mongo', label: 'Mongo', path: '/ops/db/mongo', labels: ['db:read'] },
      { key: 'postgres', label: 'Postgres', path: '/ops/db/postgres', labels: ['db:read'] }
    ]
  },
  {
    key: 'system',
    label: '系统管理',
    children: [
      { key: 'credentials', label: '凭证管理', path: '/ops/system/credentials', labels: ['system:certificate'] },
      { key: 'roles', label: '角色管理', path: '/ops/system/roles', labels: ['system:role_write'] },
      { key: 'audit', label: '操作审计', path: '/ops/system/audit', labels: ['system:audit'] }
    ]
  }
];
