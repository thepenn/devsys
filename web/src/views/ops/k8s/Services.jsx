import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Input, Select, Space, Table, Tabs, Tag, message } from 'antd';
import { listNamespaces, listResources } from '../../../api/admin/k8s';
import { formatTime } from '../../../utils/time';
import TablePagination from '../../../components/TablePagination';
import K8sClusterGuard from './K8sClusterGuard';
import './resource-tables.less';

const ALL_NAMESPACE = '__all__';

const ServiceRoutesContent = ({ clusterId }) => {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState(ALL_NAMESPACE);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [services, setServices] = useState([]);
  const [ingresses, setIngresses] = useState([]);
  const [servicePage, setServicePage] = useState(1);
  const [servicePageSize, setServicePageSize] = useState(10);
  const [ingressPage, setIngressPage] = useState(1);
  const [ingressPageSize, setIngressPageSize] = useState(10);

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
    const ns = namespace === ALL_NAMESPACE ? '' : namespace;
    setLoading(true);
    try {
      const [svcList, ingList] = await Promise.all([
        listResources(clusterId, {
          group: '',
          version: 'v1',
          resource: 'services',
          namespace: ns
        }),
        listResources(clusterId, {
          group: 'networking.k8s.io',
          version: 'v1',
          resource: 'ingresses',
          namespace: ns
        })
      ]);
      setServices(formatServices(svcList || []));
      setIngresses(formatIngresses(ingList || []));
    } catch (err) {
      message.error(err.message || '加载服务和路由失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId, namespace]);

  useEffect(() => {
    fetchResources();
  }, [fetchResources]);

  useEffect(() => {
    setServicePage(1);
    setIngressPage(1);
  }, [search, namespace]);

  const namespaceOptions = useMemo(() => {
    const base = [{ value: ALL_NAMESPACE, label: '全部命名空间' }];
    return base.concat((namespaces || []).map(item => ({ value: item.name, label: item.name })));
  }, [namespaces]);

  const filteredServices = useMemo(() => filterByKeyword(services, search), [services, search]);
  const filteredIngresses = useMemo(() => filterByKeyword(ingresses, search), [ingresses, search]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(filteredServices.length / servicePageSize));
    if (servicePage > maxPage) {
      setServicePage(maxPage);
    }
  }, [filteredServices.length, servicePage, servicePageSize]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(filteredIngresses.length / ingressPageSize));
    if (ingressPage > maxPage) {
      setIngressPage(maxPage);
    }
  }, [filteredIngresses.length, ingressPage, ingressPageSize]);

  const pagedServices = useMemo(() => {
    const start = (servicePage - 1) * servicePageSize;
    return filteredServices.slice(start, start + servicePageSize);
  }, [filteredServices, servicePage, servicePageSize]);

  const pagedIngresses = useMemo(() => {
    const start = (ingressPage - 1) * ingressPageSize;
    return filteredIngresses.slice(start, start + ingressPageSize);
  }, [filteredIngresses, ingressPage, ingressPageSize]);

  const serviceColumns = [
    { title: '名称', dataIndex: 'name', render: value => <button className="k8s-link" type="button">{value}</button>, width: 240 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '类型', dataIndex: 'type', width: 140 },
    { title: '关联数', dataIndex: 'selectorCount', width: 100 },
    { title: '集群 IP', dataIndex: 'clusterIP', width: 160 },
    { title: '内部端点', dataIndex: 'ports', render: value => value || '—' },
    {
      title: '外部端点',
      dataIndex: 'external',
      width: 240,
      render: items =>
        items && items.length ? (
          <Space size={[4, 4]} wrap>
            {items.map(item => (
              <Tag key={item} bordered={false} color="blue">
                {item}
              </Tag>
            ))}
          </Space>
        ) : (
          '—'
        )
    }
  ];

  const ingressColumns = [
    { title: '名称', dataIndex: 'name', render: value => <button className="k8s-link" type="button">{value}</button>, width: 280 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '监听协议', dataIndex: 'protocol', width: 140 },
    { title: 'VIP', dataIndex: 'vip', width: 220 },
    { title: '后端服务', dataIndex: 'service', render: value => value || '—' },
    { title: '创建时间', dataIndex: 'createdAt', width: 200 }
  ];

  const tabs = [
    {
      key: 'services',
      label: `服务 (${filteredServices.length})`,
      children: (
        <>
          <Table
            className="k8s-table"
            rowKey="id"
            columns={serviceColumns}
            loading={loading}
            dataSource={pagedServices}
            pagination={false}
          />
          <TablePagination
            page={servicePage}
            pageSize={servicePageSize}
            total={filteredServices.length}
            onChange={(nextPage, nextSize) => {
              setServicePage(nextPage);
              setServicePageSize(nextSize);
            }}
            className="table-pagination--flush"
          />
        </>
      )
    },
    {
      key: 'ingresses',
      label: `路由 (${filteredIngresses.length})`,
      children: (
        <>
          <Table
            className="k8s-table"
            rowKey="id"
            columns={ingressColumns}
            loading={loading}
            dataSource={pagedIngresses}
            pagination={false}
          />
          <TablePagination
            page={ingressPage}
            pageSize={ingressPageSize}
            total={filteredIngresses.length}
            onChange={(nextPage, nextSize) => {
              setIngressPage(nextPage);
              setIngressPageSize(nextSize);
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
      title="服务路由"
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

const K8sServices = () => (
  <K8sClusterGuard>
    {clusterId => <ServiceRoutesContent clusterId={clusterId} />}
  </K8sClusterGuard>
);

export default K8sServices;

function filterByKeyword(list, keyword) {
  if (!keyword.trim()) return list;
  const lower = keyword.trim().toLowerCase();
  return list.filter(item => `${item.name}`.toLowerCase().includes(lower) || `${item.namespace}`.toLowerCase().includes(lower));
}

function formatServices(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const spec = item.spec || {};
    const ports = (spec.ports || []).map(port => `${port.name ? `${port.name} ` : ''}${port.port}:${port.protocol || 'TCP'}`).join('、');
    const externalIngress = ((item.status || {}).loadBalancer?.ingress || []).map(ing => ing.ip || ing.hostname).filter(Boolean);
    const externalIPs = spec.externalIPs || [];
    return {
      id: metadata.uid || `${metadata.namespace}:${metadata.name}`,
      name: metadata.name,
      namespace: metadata.namespace || '-',
      type: spec.type || '-',
      selectorCount: spec.selector ? Object.keys(spec.selector).length : 0,
      clusterIP: spec.clusterIP || '-',
      ports: ports || '—',
      external: [...externalIPs, ...externalIngress],
    };
  });
}

function formatIngresses(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const spec = item.spec || {};
    const vip = ((item.status || {}).loadBalancer?.ingress || [])[0];
    const paths = (spec.rules || []).flatMap(rule =>
      (rule.http?.paths || []).map(path => {
        const backend = path.backend?.service;
        return backend ? `${backend.name}:${backend.port?.number || backend.port?.name || ''}` : '';
      })
    ).filter(Boolean);
    const createdAt = formatTime(metadata.creationTimestamp) || '—';
    return {
      id: metadata.uid || metadata.name,
      name: metadata.name,
      namespace: metadata.namespace || '-',
      protocol: spec.tls && spec.tls.length > 0 ? 'HTTPS' : 'HTTP',
      vip: vip ? vip.ip || vip.hostname : '—',
      service: paths.length ? paths.join('，') : '—',
      createdAt
    };
  });
}
