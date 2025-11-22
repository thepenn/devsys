export const opsNavItems = [
  {
    key: 'k8s',
    label: 'K8s 管理',
    children: [
      { key: 'clusters', label: '集群列表', path: '/ops/k8s/clusters' },
      { key: 'workloads', label: '工作负载', path: '/ops/k8s/workloads' },
      { key: 'services', label: '服务路由', path: '/ops/k8s/services' },
      { key: 'pods', label: 'Pods', path: '/ops/k8s/pods' },
      { key: 'jobs', label: '计划任务', path: '/ops/k8s/jobs' },
      { key: 'volumes', label: 'Volumes', path: '/ops/k8s/volumes' },
      { key: 'nodes', label: '节点管理', path: '/ops/k8s/nodes' },
      { key: 'monitor', label: '集群监控', path: '/ops/k8s/monitor' }
    ]
  },
  {
    key: 'project',
    label: '项目管理',
    children: [
      { key: 'list', label: '项目列表', path: '/ops/projects/list' },
      { key: 'pipeline', label: '项目构建', path: '/ops/projects/pipeline' },
      { key: 'monitor', label: '项目监控', path: '/ops/projects/monitor' }
    ]
  },
  {
    key: 'message',
    label: '消息通知',
    children: [
      { key: 'notification', label: '消息通知', path: '/ops/messages/notification' },
      { key: 'alert', label: '告警管理', path: '/ops/messages/alert' }
    ]
  },
  {
    key: 'database',
    label: '数据库管理',
    children: [
      { key: 'mysql', label: 'MySQL', path: '/ops/db/mysql' },
      { key: 'redis', label: 'Redis', path: '/ops/db/redis' },
      { key: 'mongo', label: 'Mongo', path: '/ops/db/mongo' },
      { key: 'postgres', label: 'Postgres', path: '/ops/db/postgres' }
    ]
  },
  {
    key: 'system',
    label: '系统管理',
    children: [
      { key: 'credentials', label: '凭证管理', path: '/ops/system/credentials' },
      { key: 'roles', label: '角色管理', path: '/ops/system/roles' },
      { key: 'audit', label: '操作审计', path: '/ops/system/audit' }
    ]
  }
];
