import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Input, Space, Table, Tag, message } from 'antd';
import { listResources } from '../../../api/admin/k8s';
import { formatPodAge, formatTime } from '../../../utils/time';
import TablePagination from '../../../components/TablePagination';
import K8sClusterGuard from './K8sClusterGuard';
import './resource-tables.less';

const NodesContent = ({ clusterId }) => {
  const [loading, setLoading] = useState(false);
  const [nodes, setNodes] = useState([]);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const fetchNodes = useCallback(async () => {
    if (!clusterId) return;
    setLoading(true);
    try {
      const resp = await listResources(clusterId, {
        group: '',
        version: 'v1',
        resource: 'nodes'
      });
      setNodes(formatNodes(resp || []));
    } catch (err) {
      message.error(err.message || '加载节点失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId]);

  useEffect(() => {
    fetchNodes();
  }, [fetchNodes]);

  useEffect(() => {
    setPage(1);
  }, [search]);

  const filteredNodes = useMemo(() => filterByKeyword(nodes, search), [nodes, search]);
  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(filteredNodes.length / pageSize));
    if (page > maxPage) {
      setPage(maxPage);
    }
  }, [filteredNodes.length, page, pageSize]);
  const pagedNodes = useMemo(() => {
    const start = (page - 1) * pageSize;
    return filteredNodes.slice(start, start + pageSize);
  }, [filteredNodes, page, pageSize]);

  const columns = [
    { title: '名称', dataIndex: 'name', render: value => <button type="button" className="k8s-link">{value}</button>, width: 220 },
    {
      title: '状态',
      dataIndex: 'ready',
      width: 120,
      render: value => <Tag color={value === 'Ready' ? 'green' : 'red'}>{value}</Tag>
    },
    { title: '角色', dataIndex: 'roles', width: 200 },
    { title: 'K8s 版本', dataIndex: 'version', width: 160 },
    { title: '内网 IP', dataIndex: 'internalIP', width: 180 },
    { title: 'Pod CIDR', dataIndex: 'podCIDR', width: 160 },
    { title: 'CPU/内存', dataIndex: 'capacity', width: 200 },
    { title: '创建时间', dataIndex: 'createdAt', width: 200 },
    { title: '运行时长', dataIndex: 'age', width: 160 }
  ];

  return (
    <Card
      className="k8s-resource-card"
      title="节点管理"
      extra={
        <Space className="k8s-resource-toolbar">
          <Input.Search
            placeholder="搜索节点名称"
            value={search}
            allowClear
            onChange={e => setSearch(e.target.value)}
            style={{ width: 260 }}
          />
          <button type="button" className="k8s-link" onClick={fetchNodes}>
            刷新
          </button>
        </Space>
      }
    >
      <Table className="k8s-table" rowKey="id" loading={loading} columns={columns} dataSource={pagedNodes} pagination={false} />
      <TablePagination
        page={page}
        pageSize={pageSize}
        total={filteredNodes.length}
        onChange={(nextPage, nextSize) => {
          setPage(nextPage);
          setPageSize(nextSize);
        }}
        className="table-pagination--flush"
      />
    </Card>
  );
};

const K8sNodes = () => (
  <K8sClusterGuard>
    {clusterId => <NodesContent clusterId={clusterId} />}
  </K8sClusterGuard>
);

export default K8sNodes;

function filterByKeyword(list, keyword) {
  if (!keyword.trim()) return list;
  const lower = keyword.trim().toLowerCase();
  return list.filter(item => `${item.name}`.toLowerCase().includes(lower));
}

function formatNodes(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const status = item.status || {};
    const nodeInfo = status.nodeInfo || {};
    const conditions = status.conditions || [];
    const readyCondition = conditions.find(cond => cond.type === 'Ready');
    const addresses = status.addresses || [];
    const internalIP = addresses.find(addr => addr.type === 'InternalIP');
    const capacity = status.capacity || {};
    const roles = extractRoles(metadata.labels || {});
    const cpu = capacity.cpu || '-';
    const memory = capacity.memory || '-';
    return {
      id: metadata.uid || metadata.name,
      name: metadata.name,
      ready: readyCondition?.status === 'True' ? 'Ready' : readyCondition?.status || 'Unknown',
      roles: roles.length ? roles.join(', ') : '—',
      version: nodeInfo.kubeletVersion || '-',
      internalIP: internalIP ? internalIP.address : '—',
      podCIDR: status.podCIDR || '-',
      capacity: `CPU ${cpu} / 内存 ${memory}`,
      createdAt: formatTime(metadata.creationTimestamp) || '—',
      age: formatPodAge(metadata.creationTimestamp)
    };
  });
}

function extractRoles(labels) {
  const roles = [];
  Object.keys(labels).forEach(key => {
    if (key === 'kubernetes.io/role') {
      roles.push(labels[key]);
    }
    if (key.startsWith('node-role.kubernetes.io/')) {
      const role = key.split('/')[1] || 'node';
      roles.push(role);
    }
  });
  return Array.from(new Set(roles));
}
