import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Input, Select, Space, Table, Tabs, Tag, message } from 'antd';
import { listNamespaces, listResources } from '../../../api/admin/k8s';
import { formatPodAge, formatTime } from '../../../utils/time';
import K8sClusterGuard from './K8sClusterGuard';
import './resource-tables.less';

const ALL_NAMESPACE = '__all__';

const JobsContent = ({ clusterId }) => {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState(ALL_NAMESPACE);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [jobs, setJobs] = useState([]);
  const [cronJobs, setCronJobs] = useState([]);

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
      const [jobList, cronList] = await Promise.all([
        listResources(clusterId, {
          group: 'batch',
          version: 'v1',
          resource: 'jobs',
          namespace: ns
        }),
        listResources(clusterId, {
          group: 'batch',
          version: 'v1',
          resource: 'cronjobs',
          namespace: ns
        })
      ]);
      setJobs(formatJobs(jobList || []));
      setCronJobs(formatCronJobs(cronList || []));
    } catch (err) {
      message.error(err.message || '加载计划任务失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId, namespace]);

  useEffect(() => {
    fetchResources();
  }, [fetchResources]);

  const namespaceOptions = useMemo(() => {
    const base = [{ value: ALL_NAMESPACE, label: '全部命名空间' }];
    return base.concat((namespaces || []).map(item => ({ value: item.name, label: item.name })));
  }, [namespaces]);

  const filteredJobs = useMemo(() => filterByKeyword(jobs, search), [jobs, search]);
  const filteredCronJobs = useMemo(() => filterByKeyword(cronJobs, search), [cronJobs, search]);

  const jobColumns = [
    { title: '名称', dataIndex: 'name', render: value => <button type="button" className="k8s-link">{value}</button>, width: 240 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '完成情况', dataIndex: 'completions', width: 140 },
    { title: '活动实例', dataIndex: 'active', width: 120 },
    { title: '启动时间', dataIndex: 'startTime', width: 200 },
    { title: '完成时间', dataIndex: 'completionTime', width: 200 },
    { title: '运行时长', dataIndex: 'age', width: 140 }
  ];

  const cronColumns = [
    { title: '名称', dataIndex: 'name', render: value => <button type="button" className="k8s-link">{value}</button>, width: 260 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '调度表达式', dataIndex: 'schedule', width: 160 },
    {
      title: '状态',
      dataIndex: 'suspend',
      width: 120,
      render: value => <Tag color={value ? 'red' : 'green'}>{value ? '暂停' : '运行中'}</Tag>
    },
    { title: '上次运行', dataIndex: 'lastSchedule', width: 200 },
    { title: '活动 Job', dataIndex: 'active', width: 120 },
    { title: '创建时间', dataIndex: 'createdAt', width: 200 }
  ];

  const tabs = [
    {
      key: 'jobs',
      label: `Job（${filteredJobs.length}）`,
      children: (
        <Table
          className="k8s-table"
          rowKey="id"
          columns={jobColumns}
          loading={loading}
          dataSource={filteredJobs}
          pagination={{ pageSize: 10 }}
        />
      )
    },
    {
      key: 'cronjobs',
      label: `CronJob（${filteredCronJobs.length}）`,
      children: (
        <Table
          className="k8s-table"
          rowKey="id"
          columns={cronColumns}
          loading={loading}
          dataSource={filteredCronJobs}
          pagination={{ pageSize: 10 }}
        />
      )
    }
  ];

  return (
    <Card
      className="k8s-resource-card"
      title="计划任务"
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

const K8sJobs = () => (
  <K8sClusterGuard>
    {clusterId => <JobsContent clusterId={clusterId} />}
  </K8sClusterGuard>
);

export default K8sJobs;

function filterByKeyword(list, keyword) {
  if (!keyword.trim()) return list;
  const lower = keyword.trim().toLowerCase();
  return list.filter(item => `${item.name}`.toLowerCase().includes(lower) || `${item.namespace}`.toLowerCase().includes(lower));
}

function formatJobs(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const spec = item.spec || {};
    const status = item.status || {};
    const desired = (spec.completions ?? spec.parallelism) || 1;
    const succeeded = status.succeeded ?? 0;
    return {
      id: metadata.uid || `${metadata.namespace}:${metadata.name}`,
      name: metadata.name,
      namespace: metadata.namespace || '-',
      completions: `${succeeded}/${desired}`,
      active: status.active || 0,
      startTime: formatTime(status.startTime) || '—',
      completionTime: formatTime(status.completionTime) || '—',
      age: formatPodAge(metadata.creationTimestamp)
    };
  });
}

function formatCronJobs(items) {
  return items.map(item => {
    const metadata = item.metadata || {};
    const spec = item.spec || {};
    const status = item.status || {};
    return {
      id: metadata.uid || metadata.name,
      name: metadata.name,
      namespace: metadata.namespace || '-',
      schedule: spec.schedule || '-',
      suspend: !!spec.suspend,
      lastSchedule: formatTime(status.lastScheduleTime) || '—',
      active: (status.active || []).length || 0,
      createdAt: formatTime(metadata.creationTimestamp) || '—'
    };
  });
}
