import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Input, Select, Space, Table, Tag, message } from 'antd';
import { listNamespaces, listResources } from '../../../api/admin/k8s';
import { formatPodAge, formatTime } from '../../../utils/time';
import TablePagination from '../../../components/TablePagination';
import K8sClusterGuard from './K8sClusterGuard';
import './resource-tables.less';

const ALL_NAMESPACE = '__all__';

const PodsContent = ({ clusterId }) => {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState(ALL_NAMESPACE);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [pods, setPods] = useState([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const fetchNamespaces = useCallback(async () => {
    try {
      const list = await listNamespaces(clusterId);
      setNamespaces(Array.isArray(list) ? list : []);
    } catch (err) {
      message.error(err.message || '加载命名空间失败');
    }
  }, [clusterId]);

  useEffect(() => {
    fetchNamespaces();
  }, [fetchNamespaces]);

  const fetchPods = useCallback(async () => {
    if (!clusterId) return;
    setLoading(true);
    const ns = namespace === ALL_NAMESPACE ? '' : namespace;
    try {
      const resp = await listResources(clusterId, {
        group: '',
        version: 'v1',
        resource: 'pods',
        namespace: ns
      });
      setPods(formatPods(resp || []));
    } catch (err) {
      message.error(err.message || '加载 Pod 列表失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId, namespace]);

  useEffect(() => {
    fetchPods();
  }, [fetchPods]);

  useEffect(() => {
    setPage(1);
  }, [namespace, search]);

  const namespaceOptions = useMemo(() => {
    const base = [{ value: ALL_NAMESPACE, label: '全部命名空间' }];
    return base.concat((namespaces || []).map(item => ({ value: item.name, label: item.name })));
  }, [namespaces]);

  const filteredPods = useMemo(() => filterByKeyword(pods, search), [pods, search]);
  const totalPods = filteredPods.length;
  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(totalPods / pageSize));
    if (page > maxPage) {
      setPage(maxPage);
    }
  }, [totalPods, page, pageSize]);
  const pagedPods = useMemo(() => {
    const start = (page - 1) * pageSize;
    return filteredPods.slice(start, start + pageSize);
  }, [filteredPods, page, pageSize]);

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: value => <button type="button" className="k8s-link">{value}</button>,
      width: 260
    },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 140,
      render: value => <Tag color={value === 'Running' ? 'green' : value === 'Pending' ? 'orange' : 'red'}>{value}</Tag>
    },
    { title: 'READY', dataIndex: 'ready', width: 140 },
    { title: '节点', dataIndex: 'node', width: 200 },
    { title: 'Pod IP', dataIndex: 'podIP', width: 160 },
    { title: '重启次数', dataIndex: 'restarts', width: 120 },
    { title: '创建时间', dataIndex: 'createdAt', width: 200 },
    { title: '运行时长', dataIndex: 'age', width: 160 }
  ];

  return (
    <Card
      className="k8s-resource-card"
      title="Pods"
      extra={
        <Space className="k8s-resource-toolbar">
          <Select style={{ width: 200 }} value={namespace} options={namespaceOptions} onChange={setNamespace} />
          <Input.Search
            placeholder="搜索名称/命名空间"
            value={search}
            allowClear
            onChange={e => setSearch(e.target.value)}
            style={{ width: 260 }}
          />
          <button type="button" className="k8s-link" onClick={fetchPods}>
            刷新
          </button>
        </Space>
      }
    >
      <Table
        className="k8s-table"
        loading={loading}
        rowKey="id"
        columns={columns}
        dataSource={pagedPods}
        pagination={false}
      />
      <TablePagination
        page={page}
        pageSize={pageSize}
        total={totalPods}
        onChange={(nextPage, nextSize) => {
          setPage(nextPage);
          setPageSize(nextSize);
        }}
        className="table-pagination--flush"
      />
    </Card>
  );
};

const K8sPods = () => (
  <K8sClusterGuard>
    {clusterId => <PodsContent clusterId={clusterId} />}
  </K8sClusterGuard>
);

export default K8sPods;

function filterByKeyword(list, keyword) {
  if (!keyword.trim()) return list;
  const lower = keyword.trim().toLowerCase();
  return list.filter(item => `${item.name}`.toLowerCase().includes(lower) || `${item.namespace}`.toLowerCase().includes(lower));
}

function formatPods(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const status = item.status || {};
    const containerStatuses = status.containerStatuses || [];
    const readyCount = containerStatuses.filter(cs => cs.ready).length;
    const total = containerStatuses.length || (item.spec?.containers || []).length || 0;
    const nodeName = item.spec?.nodeName || '-';
    const hostIP = status.hostIP;
    return {
      id: metadata.uid || `${metadata.namespace}:${metadata.name}`,
      name: metadata.name,
      namespace: metadata.namespace || '-',
      status: status.phase || '-',
      ready: `${readyCount}/${total || (readyCount || 1)}`,
      node: hostIP ? `${nodeName} (${hostIP})` : nodeName,
      podIP: status.podIP || '-',
      restarts: containerStatuses.reduce((sum, cs) => sum + (cs.restartCount || 0), 0),
      createdAt: formatTime(metadata.creationTimestamp) || '—',
      age: formatPodAge(metadata.creationTimestamp),
    };
  });
}
