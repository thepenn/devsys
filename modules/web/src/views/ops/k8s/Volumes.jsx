import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Input, Select, Space, Table, Tabs, Tag, message } from 'antd';
import { listNamespaces, listResources } from '../../../api/admin/k8s';
import { formatTime } from '../../../utils/time';
import TablePagination from '../../../components/TablePagination';
import K8sClusterGuard from './K8sClusterGuard';
import './resource-tables.less';

const ALL_NAMESPACE = '__all__';

const VolumesContent = ({ clusterId }) => {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState(ALL_NAMESPACE);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [pvcs, setPvcs] = useState([]);
  const [pvs, setPvs] = useState([]);
  const [pvcPage, setPvcPage] = useState(1);
  const [pvcPageSize, setPvcPageSize] = useState(10);
  const [pvPage, setPvPage] = useState(1);
  const [pvPageSize, setPvPageSize] = useState(10);

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

  const fetchResources = useCallback(async () => {
    if (!clusterId) return;
    setLoading(true);
    const ns = namespace === ALL_NAMESPACE ? '' : namespace;
    try {
      const [pvcList, pvList] = await Promise.all([
        listResources(clusterId, {
          group: '',
          version: 'v1',
          resource: 'persistentvolumeclaims',
          namespace: ns
        }),
        listResources(clusterId, {
          group: '',
          version: 'v1',
          resource: 'persistentvolumes'
        })
      ]);
      setPvcs(formatPVCs(pvcList || []));
      setPvs(formatPVs(pvList || []));
    } catch (err) {
      message.error(err.message || '加载 Volume 列表失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId, namespace]);

  useEffect(() => {
    fetchResources();
  }, [fetchResources]);

  useEffect(() => {
    setPvcPage(1);
    setPvPage(1);
  }, [namespace, search]);

  const namespaceOptions = useMemo(() => {
    const base = [{ value: ALL_NAMESPACE, label: '全部命名空间' }];
    return base.concat((namespaces || []).map(item => ({ value: item.name, label: item.name })));
  }, [namespaces]);

  const filteredPVCs = useMemo(() => filterByKeyword(pvcs, search), [pvcs, search]);
  const filteredPVs = useMemo(() => filterByKeyword(pvs, search), [pvs, search]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(filteredPVCs.length / pvcPageSize));
    if (pvcPage > maxPage) {
      setPvcPage(maxPage);
    }
  }, [filteredPVCs.length, pvcPage, pvcPageSize]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(filteredPVs.length / pvPageSize));
    if (pvPage > maxPage) {
      setPvPage(maxPage);
    }
  }, [filteredPVs.length, pvPage, pvPageSize]);

  const pagedPVCs = useMemo(() => {
    const start = (pvcPage - 1) * pvcPageSize;
    return filteredPVCs.slice(start, start + pvcPageSize);
  }, [filteredPVCs, pvcPage, pvcPageSize]);

  const pagedPVs = useMemo(() => {
    const start = (pvPage - 1) * pvPageSize;
    return filteredPVs.slice(start, start + pvPageSize);
  }, [filteredPVs, pvPage, pvPageSize]);

  const pvcColumns = [
    { title: '名称', dataIndex: 'name', render: value => <button type="button" className="k8s-link">{value}</button>, width: 260 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 140,
      render: value => <Tag color={value === 'Bound' ? 'green' : value === 'Pending' ? 'orange' : 'red'}>{value}</Tag>
    },
    { title: '容量', dataIndex: 'capacity', width: 140 },
    { title: '卷', dataIndex: 'volume', width: 220 },
    { title: '存储类', dataIndex: 'storageClass', width: 160 },
    { title: '访问模式', dataIndex: 'accessModes', width: 180 },
    { title: '创建时间', dataIndex: 'createdAt', width: 200 }
  ];

  const pvColumns = [
    { title: '名称', dataIndex: 'name', render: value => <button type="button" className="k8s-link">{value}</button>, width: 260 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 140,
      render: value => <Tag color={value === 'Bound' ? 'green' : value === 'Available' ? 'blue' : 'red'}>{value}</Tag>
    },
    { title: '容量', dataIndex: 'capacity', width: 140 },
    { title: '存储类', dataIndex: 'storageClass', width: 160 },
    { title: '回收策略', dataIndex: 'reclaimPolicy', width: 160 },
    { title: '访问模式', dataIndex: 'accessModes', width: 180 },
    { title: '绑定 PVC', dataIndex: 'claim', width: 240 },
    { title: '创建时间', dataIndex: 'createdAt', width: 200 }
  ];

  const tabs = [
    {
      key: 'pvcs',
      label: `PVC (${filteredPVCs.length})`,
      children: (
        <>
          <Table
            className="k8s-table"
            rowKey="id"
            columns={pvcColumns}
            loading={loading}
            dataSource={pagedPVCs}
            pagination={false}
          />
          <TablePagination
            page={pvcPage}
            pageSize={pvcPageSize}
            total={filteredPVCs.length}
            onChange={(nextPage, nextSize) => {
              setPvcPage(nextPage);
              setPvcPageSize(nextSize);
            }}
            className="table-pagination--flush"
          />
        </>
      )
    },
    {
      key: 'pvs',
      label: `PV (${filteredPVs.length})`,
      children: (
        <>
          <Table
            className="k8s-table"
            rowKey="id"
            columns={pvColumns}
            loading={loading}
            dataSource={pagedPVs}
            pagination={false}
          />
          <TablePagination
            page={pvPage}
            pageSize={pvPageSize}
            total={filteredPVs.length}
            onChange={(nextPage, nextSize) => {
              setPvPage(nextPage);
              setPvPageSize(nextSize);
            }}
            className="table-pagination--flush"
          />
        </>
      )
    }
  ];

  return (
    <Card
      className="k8s-resource-card"
      title="存储卷"
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
          <button type="button" className="k8s-link" onClick={fetchResources}>
            刷新
          </button>
        </Space>
      }
    >
      <Tabs items={tabs} />
    </Card>
  );
};

const K8sVolumes = () => (
  <K8sClusterGuard>
    {clusterId => <VolumesContent clusterId={clusterId} />}
  </K8sClusterGuard>
);

export default K8sVolumes;

function filterByKeyword(list, keyword) {
  if (!keyword.trim()) return list;
  const lower = keyword.trim().toLowerCase();
  return list.filter(item => `${item.name}`.toLowerCase().includes(lower) || `${item.namespace || ''}`.toLowerCase().includes(lower));
}

function formatPVCs(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const spec = item.spec || {};
    const status = item.status || {};
    return {
      id: metadata.uid || `${metadata.namespace}:${metadata.name}`,
      name: metadata.name,
      namespace: metadata.namespace || '-',
      status: status.phase || '-',
      capacity: spec.resources?.requests?.storage || status.capacity?.storage || '-',
      volume: spec.volumeName || '-',
      storageClass: spec.storageClassName || '-',
      accessModes: (spec.accessModes || []).join(', ') || '-',
      createdAt: formatTime(metadata.creationTimestamp) || '—'
    };
  });
}

function formatPVs(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const spec = item.spec || {};
    const status = item.status || {};
    const annotations = metadata.annotations || {};
    const claimRef = spec.claimRef;
    return {
      id: metadata.uid || metadata.name,
      name: metadata.name,
      status: status.phase || '-',
      capacity: spec.capacity?.storage || '-',
      storageClass: spec.storageClassName || annotations['volume.beta.kubernetes.io/storage-class'] || '-',
      reclaimPolicy: spec.persistentVolumeReclaimPolicy || '-',
      accessModes: (spec.accessModes || []).join(', ') || '-',
      claim: claimRef ? `${claimRef.namespace}/${claimRef.name}` : '—',
      createdAt: formatTime(metadata.creationTimestamp) || '—'
    };
  });
}
