import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, Dropdown, Input, Modal, Select, Space, Table, Tag, message } from 'antd';
import { EllipsisOutlined } from '@ant-design/icons';
import { createStyles } from 'antd-style';
import { listNamespaces, listResources, listWorkloadPods, getResource, applyManifest, deleteResource } from '../../../api/admin/k8s';
import { formatPodAge, formatTime } from '../../../utils/time';
import K8sClusterGuard from './K8sClusterGuard';
import './workloads.less';

const ALL_NAMESPACE = '__all__';
const STATUS_COLORS = {
  success: 'green',
  warning: 'orange',
  danger: 'red',
  default: 'default'
};

const workloadTargets = [
  { key: 'deployments', kind: 'Deployment', group: 'apps', version: 'v1', resource: 'deployments' },
  { key: 'statefulsets', kind: 'StatefulSet', group: 'apps', version: 'v1', resource: 'statefulsets' },
  { key: 'daemonsets', kind: 'DaemonSet', group: 'apps', version: 'v1', resource: 'daemonsets' }
];

const useStyle = createStyles(({ css, token }) => {
  const { antCls } = token;
  return {
    customTable: css`
      ${antCls}-table {
        ${antCls}-table-container {
          ${antCls}-table-body,
          ${antCls}-table-content {
            scrollbar-width: thin;
            scrollbar-color: #eaeaea transparent;
            scrollbar-gutter: stable;
          }
        }
      }
    `,
  };
});

const PodsTable = ({ loading, pods }) => {
  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '命名空间', dataIndex: 'namespace', width: 140 },
    { title: 'READY', dataIndex: 'ready', width: 100 },
    { title: '状态', dataIndex: 'status', width: 160 },
    { title: '重启', dataIndex: 'restarts', width: 80 },
    { title: '节点', dataIndex: 'node', width: 160 },
    { title: '运行时间', dataIndex: 'age', width: 120 }
  ];
  return <Table rowKey="name" size="small" loading={loading} columns={columns} dataSource={pods} pagination={false} />;
};

const WorkloadsContent = ({ clusterId }) => {
  const { styles } = useStyle();
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState(ALL_NAMESPACE);
  const [loading, setLoading] = useState(false);
  const [resources, setResources] = useState([]);
  const [search, setSearch] = useState('');
  const [podsCache, setPodsCache] = useState({});
  const [podsLoading, setPodsLoading] = useState({});
  const [expandedRowKeys, setExpandedRowKeys] = useState([]);
  const [page, setPage] = useState(1);

  const fetchNamespaces = useCallback(async () => {
    try {
      const list = await listNamespaces(clusterId);
      setNamespaces(Array.isArray(list) ? list : []);
      if (namespace !== ALL_NAMESPACE && !(list || []).some(ns => ns.name === namespace)) {
        setNamespace(ALL_NAMESPACE);
      }
    } catch (err) {
      message.error(err.message || '加载命名空间失败');
    }
  }, [clusterId, namespace]);

  const fetchResources = useCallback(async () => {
    if (!clusterId) return;
    setLoading(true);
    setPodsCache({});
    const ns = namespace === ALL_NAMESPACE ? '' : namespace;
    try {
      const requests = workloadTargets.map(target =>
        listResources(clusterId, {
          group: target.group,
          version: target.version,
          resource: target.resource,
          namespace: ns
        }).then(items => (items || []).map(item => decorateResource(item, target)))
      );
      const results = await Promise.all(requests);
      setResources(results.flat());
      setExpandedRowKeys([]);
      setPage(1);
    } catch (err) {
      message.error(err.message || '加载工作负载失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId, namespace]);

  useEffect(() => {
    fetchNamespaces();
  }, [fetchNamespaces]);

  useEffect(() => {
    fetchResources();
  }, [fetchResources]);

  useEffect(() => {
    setPage(1);
  }, [search]);

  const filteredResources = useMemo(() => {
    if (!search.trim()) return resources;
    const keyword = search.trim().toLowerCase();
    return resources.filter(
      item =>
        item.name.toLowerCase().includes(keyword) ||
        (item.namespace && item.namespace.toLowerCase().includes(keyword)) ||
        item.kind.toLowerCase().includes(keyword)
    );
  }, [resources, search]);

  const namespaceOptions = useMemo(() => {
    const base = [{ label: '全部命名空间', value: ALL_NAMESPACE }];
    return base.concat((namespaces || []).map(ns => ({ value: ns.name, label: ns.name })));
  }, [namespaces]);

  const loadPods = useCallback(
    async (record, force = false) => {
      const key = record.key;
      if (!force && podsCache[key]) return;
      setPodsLoading(prev => ({ ...prev, [key]: true }));
      try {
        const pods = await listWorkloadPods(clusterId, {
          kind: record.kind.toLowerCase(),
          namespace: record.namespace,
          name: record.name
        });
        const normalized = (pods || []).map(pod => ({
          ...pod,
          age: formatPodAge((pod.created_at || 0) * 1000)
        }));
        setPodsCache(prev => ({ ...prev, [key]: normalized }));
      } catch (err) {
        message.error(err.message || '加载 Pods 失败');
      } finally {
        setPodsLoading(prev => ({ ...prev, [key]: false }));
      }
    },
    [clusterId, podsCache]
  );

  const handleTogglePods = useCallback(
    record => {
      setExpandedRowKeys(prev => {
        const exists = prev.includes(record.key);
        if (exists) {
          return prev.filter(key => key !== record.key);
        }
        loadPods(record);
        return prev.concat(record.key);
      });
    },
    [loadPods]
  );

  const buildResourceRequest = useCallback(record => ({
    group: record.group,
    version: record.version,
    resource: record.resource,
    namespace: record.namespace || '',
    name: record.name
  }), []);

  const handleRecreate = useCallback(async record => {
    const hide = message.loading({ content: '重新创建中...', duration: 0 });
    try {
      const detail = await getResource(clusterId, buildResourceRequest(record));
      const manifest = detail?.yaml || detail?.YAML;
      if (!manifest) {
        throw new Error('未获取到资源配置');
      }
      await applyManifest(clusterId, {
        group: record.group,
        version: record.version,
        resource: record.resource,
        namespace: record.namespace || '',
        manifest
      });
      message.success('已重新创建');
      fetchResources();
    } catch (err) {
      message.error(err.message || '重新创建失败');
    } finally {
      if (typeof hide === 'function') {
        hide();
      }
    }
  }, [clusterId, buildResourceRequest, fetchResources]);

  const handleDelete = useCallback(async record => {
    const hide = message.loading({ content: '删除中...', duration: 0 });
    try {
      await deleteResource(clusterId, buildResourceRequest(record));
      message.success('已删除');
      fetchResources();
    } catch (err) {
      message.error(err.message || '删除失败');
      throw err;
    } finally {
      if (typeof hide === 'function') {
        hide();
      }
    }
  }, [clusterId, buildResourceRequest, fetchResources]);

  const confirmDelete = useCallback(
    record => {
      Modal.confirm({
        title: `确认删除 ${record.name}?`,
        content: '该操作不可恢复，请谨慎操作。',
        okText: '删除',
        cancelText: '取消',
        okButtonProps: { danger: true },
        centered: true,
        onOk: () => handleDelete(record)
      });
    },
    [handleDelete]
  );

  const handleRowAction = useCallback(
    (action, record) => {
      if (action === 'pods') {
        handleTogglePods(record);
      } else if (action === 'refresh') {
        loadPods(record, true);
      } else if (action === 'recreate') {
        return handleRecreate(record);
      } else if (action === 'delete') {
        confirmDelete(record);
      }
      return null;
    },
    [confirmDelete, handleRecreate, handleTogglePods, loadPods]
  );

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: text => (
        <div className="workload-name">
          <strong className="workload-name__title">{text}</strong>
        </div>
      ),
      width: 320,
      fixed: 'left'
    },
    {
      title: '类型',
      dataIndex: 'kind',
      render: kind => <span className="workload-kind">{kind}</span>,
      width: 140
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      ellipsis: true,
      render: (_, record) => record.namespaceDisplay,
      width: 200
    },
    {
      title: '镜像',
      dataIndex: 'image',
      render: value => <span className="workload-image" title={value}>{value || '—'}</span>
    },
    {
      title: '标签',
      dataIndex: 'labels',
      render: labels =>
        labels && labels.length ? (
          <Space size={4} wrap>
            <Tag bordered={false} className="workload-tag">
              {labels[0]}
            </Tag>
          </Space>
        ) : (
          '—'
        ),
      width: 320
    },
    {
      title: '状态',
      dataIndex: 'status',
      render: status => (
        <Tag color={STATUS_COLORS[status.type] || 'default'}>{status.text}</Tag>
      ),
      width: 180
    },
    {
      title: '副本',
      dataIndex: 'replicas',
      width: 100
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      render: value => formatTime(value),
      width: 200
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 100,
      fixed: 'right',
      render: (_, record) => (
        <Dropdown
          trigger={['click']}
          menu={{
            items: [
              { key: 'pods', label: expandedRowKeys.includes(record.key) ? '收起 Pod' : '查看 Pod' },
              { key: 'refresh', label: '刷新 Pod' },
              { type: 'divider' },
              { key: 'recreate', label: '重新创建' },
              { key: 'delete', label: '删除', danger: true }
            ],
            onClick: ({ key }) => handleRowAction(key, record)
          }}
        >
          <Button type="text" icon={<EllipsisOutlined />} />
        </Dropdown>
      )
    }
  ];

  return (
    <Card
      className="workloads-card"
      title="工作负载"
      extra={
        <Space className="workloads-toolbar" size={12}>
          <Select
            value={namespace}
            options={namespaceOptions}
            onChange={value => setNamespace(value)}
            style={{ width: 200 }}
          />
          <Input.Search
            placeholder="搜索名称/命名空间"
            value={search}
            allowClear
            onChange={e => setSearch(e.target.value)}
            style={{ width: 260 }}
          />
          <Button onClick={fetchResources}>刷新</Button>
        </Space>
      }
    >
      <Table
        className={`workloads-table ${styles.customTable}`}
        rowKey="key"
        loading={loading}
        columns={columns}
        dataSource={filteredResources}
        size="middle"
        scroll={{ x: 'max-content' }}
        expandable={{
          expandedRowKeys,
          expandedRowRender: record => (
            <PodsTable pods={podsCache[record.key] || []} loading={!!podsLoading[record.key]} />
          ),
          onExpand: (expanded, record) => {
            if (expanded) {
              loadPods(record);
              setExpandedRowKeys(prev => (prev.includes(record.key) ? prev : prev.concat(record.key)));
            } else {
              setExpandedRowKeys(prev => prev.filter(key => key !== record.key));
            }
          },
          expandIconColumnIndex: 0
        }}
        pagination={{
          current: page,
          pageSize: 10,
          total: filteredResources.length,
          showSizeChanger: false,
          showTotal: total => `共 ${total} 条`,
          onChange: setPage
        }}
      />
    </Card>
  );
};

const K8sWorkloads = () => (
  <K8sClusterGuard>
    {clusterId => <WorkloadsContent clusterId={clusterId} />}
  </K8sClusterGuard>
);

function decorateResource(item, target) {
  const metadata = item.metadata || {};
  const spec = item.spec || {};
  const status = item.status || {};
  const namespace = metadata.namespace || '';
  const key = metadata.uid || `${namespace}:${metadata.name}`;
  const statusInfo = buildStatus(target.kind, spec, status);
  return {
    key,
    raw: item,
    kind: target.kind,
    group: target.group || '',
    version: target.version,
    resource: target.resource,
    name: metadata.name || '-',
    namespace,
    namespaceDisplay: namespace || '—',
    image: extractMainImage(spec),
    labels: extractLabels(metadata),
    status: statusInfo,
    replicas: statusInfo.scaleText,
    createdAt: metadata.creationTimestamp
  };
}

function buildStatus(kind, spec, status) {
  const make = (text, type, scaleText) => ({ text, type, scaleText });
  if (kind === 'Deployment') {
    const desired = spec.replicas ?? status.replicas ?? 0;
    const available = status.availableReplicas ?? 0;
    if (desired === 0) {
      return make('已停止', 'default', '0 / 0');
    }
    if (available >= desired) {
      return make(`运行中 (${available}/${desired})`, 'success', `${available} / ${desired}`);
    }
    if (available > 0) {
      return make(`部分可用 (${available}/${desired})`, 'warning', `${available} / ${desired}`);
    }
    return make(`未就绪 (${available}/${desired})`, 'danger', `${available} / ${desired}`);
  }
  if (kind === 'StatefulSet') {
    const desired = spec.replicas ?? status.replicas ?? 0;
    const ready = status.readyReplicas ?? 0;
    const type = ready >= desired ? 'success' : ready > 0 ? 'warning' : 'danger';
    return make(`运行中 (${ready}/${desired})`, type, `${ready} / ${desired}`);
  }
  if (kind === 'DaemonSet') {
    const desired = status.desiredNumberScheduled ?? 0;
    const ready = status.numberReady ?? 0;
    const type = ready >= desired ? 'success' : ready > 0 ? 'warning' : 'danger';
    return make(`节点 (${ready}/${desired})`, type, `${ready} / ${desired}`);
  }
  return make('运行中', 'default', '-');
}

function extractMainImage(spec) {
  const template = spec.template || {};
  const podSpec = template.spec || spec;
  const containers = podSpec.containers || [];
  return containers.length ? containers[0].image : '';
}

function extractLabels(metadata = {}) {
  const labels = metadata.labels || {};
  return Object.keys(labels).map(key => `${key}: ${labels[key]}`);
}

export default K8sWorkloads;
