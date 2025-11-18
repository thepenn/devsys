import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Card, Drawer, Dropdown, Empty, Input, InputNumber, Modal, Select, Space, Spin, Switch, Table, Tabs, Tag, message } from 'antd';
import { EllipsisOutlined } from '@ant-design/icons';
import { createStyles } from 'antd-style';
import {
  listNamespaces,
  listResources,
  listWorkloadPods,
  getResource,
  applyManifest,
  deleteResource,
  getWorkloadDetails,
  getWorkloadHistory,
  rollbackWorkload,
  getWorkloadLogs,
  listResourceEvents,
  getPodLogs
} from '../../../api/admin/k8s';
import { formatPodAge, formatTime } from '../../../utils/time';
import TablePagination from '../../../components/TablePagination';
import { API_BASE_URL } from '../../../utils/request';
import { getToken } from '../../../utils/auth';
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

const PodsTable = ({ loading, pods, onTerminal, onLogs, onDelete }) => {
  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: value => (
        <span className="pod-name" title={value}>
          {value}
        </span>
      ),
      ellipsis: true
    },
    { title: '命名空间', dataIndex: 'namespace', ellipsis: true },
    { title: 'READY', dataIndex: 'ready' },
    {
      title: '状态',
      dataIndex: 'status',
      render: value => (
        <Tag bordered={false} color={value?.toLowerCase() === 'running' ? 'green' : 'default'}>
          {value || '—'}
        </Tag>
      )
    },
    { title: '重启', dataIndex: 'restarts', width: 80 },
    { title: '节点', dataIndex: 'node', ellipsis: true },
    { title: '运行时间', dataIndex: 'age', width: 120 },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 140,
      render: (_, record) => (
        <Dropdown
          trigger={['click']}
          menu={{
            items: [
              { key: 'bash', label: 'Bash 终端' },
              { key: 'sh', label: 'Sh 终端' },
              { key: 'logs', label: '查看日志' },
              { type: 'divider' },
              { key: 'delete', label: '删除', danger: true }
            ],
            onClick: ({ key }) => {
              if (key === 'bash' || key === 'sh') {
                onTerminal?.(record, key);
              } else if (key === 'logs') {
                onLogs?.(record);
              } else if (key === 'delete') {
                onDelete?.(record);
              }
            }
          }}
        >
          <Button type="link" size="small">
            操作
          </Button>
        </Dropdown>
      )
    }
  ];
  return (
    <Table
      rowKey="name"
      size="small"
      loading={loading}
      columns={columns}
      dataSource={pods}
      pagination={false}
      scroll={{ x: 'max-content' }}
    />
  );
};

const ensureAbsoluteBase = base => {
  if (/^https?:\/\//i.test(base)) {
    return base.replace(/\/+$/, '');
  }
  if (typeof window === 'undefined') {
    return `http://localhost${base.startsWith('/') ? '' : '/'}${base}`;
  }
  const origin = window.location.origin.replace(/\/+$/, '');
  const suffix = base.startsWith('/') ? base : `/${base}`;
  return `${origin}${suffix}`.replace(/\/+$/, '');
};

const buildWsUrl = (path, params = {}) => {
  if (typeof window === 'undefined') return '';
  const base = ensureAbsoluteBase(API_BASE_URL).replace(/\/+$/, '');
  const suffix = path.startsWith('/') ? path : `/${path}`;
  const url = new URL(`${base}${suffix}`);
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      url.searchParams.set(key, value);
    }
  });
  const token = getToken();
  if (token) {
    url.searchParams.set('token', token);
  }
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
};

const ANSI_ESCAPE = String.fromCharCode(27);
const BELL = String.fromCharCode(7);
const cleanTerminalOutput = value => {
  if (!value) return '';
  const ansiRegex = new RegExp(`${ANSI_ESCAPE}\\[[0-9;?]*[ -/]*[@-~]`, 'g');
  return value.replace(ansiRegex, '').replace(new RegExp(BELL, 'g'), '');
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
  const [pageSize, setPageSize] = useState(10);
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false);
  const [detailTab, setDetailTab] = useState('overview');
  const [detailRecord, setDetailRecord] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [workloadDetail, setWorkloadDetail] = useState(null);
  const [history, setHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [events, setEvents] = useState([]);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [eventsPage, setEventsPage] = useState(1);
  const [eventsTotal, setEventsTotal] = useState(0);
  const [logsContent, setLogsContent] = useState('');
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsTail, setLogsTail] = useState(200);
  const [logWrap, setLogWrap] = useState(true);
  const [autoRefreshLogs, setAutoRefreshLogs] = useState(false);
  const [podLogsDrawer, setPodLogsDrawer] = useState({ visible: false, pod: null, container: '', content: '', loading: false });
  const [terminalDrawer, setTerminalDrawer] = useState({ visible: false, pod: null, container: '', shell: 'bash', output: '', status: 'idle' });
  const terminalSocketRef = useRef(null);
  const terminalInputRef = useRef(null);
  const terminalOutputRef = useRef(null);

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

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(filteredResources.length / pageSize));
    if (page > maxPage) {
      setPage(maxPage);
    }
  }, [filteredResources.length, page, pageSize]);

  const paginatedResources = useMemo(() => {
    const start = (page - 1) * pageSize;
    return filteredResources.slice(start, start + pageSize);
  }, [filteredResources, page, pageSize]);

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

  const fetchPodLogsContent = useCallback(
    async (pod, container) => {
      if (!clusterId || !pod || !container) {
        setPodLogsDrawer(prev => ({ ...prev, loading: false, content: '暂无容器可选' }));
        return;
      }
      try {
        const result = await getPodLogs(clusterId, {
          namespace: pod.namespace,
          pod: pod.name,
          container,
          tail: 500
        });
        setPodLogsDrawer(prev => ({ ...prev, loading: false, content: result?.content || '暂无日志' }));
      } catch (err) {
        setPodLogsDrawer(prev => ({ ...prev, loading: false }));
        message.error(err?.message || '获取 Pod 日志失败');
      }
    },
    [clusterId]
  );

  const openPodLogs = useCallback(pod => {
    if (!pod) return;
    const container = Array.isArray(pod.containers) && pod.containers.length ? pod.containers[0] : '';
    setPodLogsDrawer({
      visible: true,
      pod,
      container,
      content: '',
      loading: true
    });
  }, []);

  const closePodLogs = useCallback(() => {
    setPodLogsDrawer({ visible: false, pod: null, container: '', content: '', loading: false });
  }, []);

  const handlePodLogsContainerChange = value => {
    setPodLogsDrawer(prev => ({ ...prev, container: value }));
  };

  const refreshPodLogs = useCallback(() => {
    if (!podLogsDrawer.visible || !podLogsDrawer.pod || !podLogsDrawer.container) return;
    setPodLogsDrawer(prev => ({ ...prev, loading: true }));
    fetchPodLogsContent(podLogsDrawer.pod, podLogsDrawer.container);
  }, [fetchPodLogsContent, podLogsDrawer.container, podLogsDrawer.pod, podLogsDrawer.visible]);

  useEffect(() => {
    if (!podLogsDrawer.visible || !podLogsDrawer.pod || !podLogsDrawer.container) {
      if (podLogsDrawer.visible && (!podLogsDrawer.container || !podLogsDrawer.pod)) {
        setPodLogsDrawer(prev => ({ ...prev, loading: false, content: '该 Pod 无可用容器' }));
      }
      return;
    }
    setPodLogsDrawer(prev => ({ ...prev, loading: true }));
    fetchPodLogsContent(podLogsDrawer.pod, podLogsDrawer.container);
  }, [podLogsDrawer.visible, podLogsDrawer.pod, podLogsDrawer.container, fetchPodLogsContent]);

  const openPodTerminal = useCallback((pod, shellType = 'bash') => {
    if (!pod) return;
    const container = Array.isArray(pod.containers) && pod.containers.length ? pod.containers[0] : '';
    setTerminalDrawer({
      visible: true,
      pod,
      container,
      shell: shellType,
      output: container ? '正在连接终端...\n' : '该 Pod 无可用容器。\n',
      status: container ? 'connecting' : 'error'
    });
  }, []);

  const closeTerminalDrawer = useCallback(() => {
    if (terminalSocketRef.current) {
      try {
        terminalSocketRef.current.send(JSON.stringify({ op: 'close' }));
      } catch (err) {
        // ignore
      }
      terminalSocketRef.current.close();
      terminalSocketRef.current = null;
    }
    setTerminalDrawer({ visible: false, pod: null, container: '', shell: 'bash', output: '', status: 'idle' });
  }, []);

  const sendTerminalFrame = useCallback(data => {
    if (!data || !terminalSocketRef.current || terminalSocketRef.current.readyState !== WebSocket.OPEN) {
      return;
    }
    try {
      terminalSocketRef.current.send(JSON.stringify({ op: 'stdin', data }));
    } catch (err) {
      console.warn('send terminal data failed', err);
    }
  }, []);

  const handleTerminalKeyDown = useCallback(
    event => {
      if (!terminalDrawer.visible) return;
      let payload = '';
      const specialMap = {
        ArrowUp: '\u001b[A',
        ArrowDown: '\u001b[B',
        ArrowRight: '\u001b[C',
        ArrowLeft: '\u001b[D',
        Delete: '\u001b[3~',
        Home: '\u001bOH',
        End: '\u001bOF',
        PageUp: '\u001b[5~',
        PageDown: '\u001b[6~'
      };
      if (event.key === 'Enter') {
        payload = '\r';
      } else if (event.key === 'Backspace') {
        payload = '\u0008';
      } else if (event.key === 'Tab') {
        event.preventDefault();
        payload = '\t';
      } else if (event.key === 'c' && event.ctrlKey) {
        payload = '\u0003';
      } else if (specialMap[event.key]) {
        payload = specialMap[event.key];
      } else if (event.key.length === 1 && !event.metaKey && !event.altKey) {
        payload = event.key;
      }
      if (payload) {
        event.preventDefault();
        sendTerminalFrame(payload);
      }
    },
    [sendTerminalFrame, terminalDrawer.visible]
  );

  const handleTerminalContainerChange = value => {
    setTerminalDrawer(prev => ({
      ...prev,
      container: value,
      output: value ? '正在连接终端...\n' : '该容器不可用。\n',
      status: value ? 'connecting' : 'error'
    }));
  };

  const handleTerminalClear = () => {
    setTerminalDrawer(prev => ({ ...prev, output: '' }));
  };

  useEffect(() => {
    if (!terminalDrawer.visible || !terminalDrawer.pod || !terminalDrawer.container) {
      return;
    }
    const shellPath = terminalDrawer.shell === 'sh' ? '/bin/sh' : '/bin/bash';
    const wsUrl = buildWsUrl(
      `/admin/k8s/clusters/${clusterId}/pods/${terminalDrawer.pod.namespace}/${terminalDrawer.pod.name}/exec/stream`,
      {
        shell: shellPath,
        container: terminalDrawer.container
      }
    );
    setTerminalDrawer(prev => ({ ...prev, output: '正在连接终端...\n', status: 'connecting' }));
    const ws = new WebSocket(wsUrl);
    terminalSocketRef.current = ws;
    ws.onopen = () => setTerminalDrawer(prev => ({ ...prev, status: 'connected' }));
    ws.onerror = () => setTerminalDrawer(prev => ({ ...prev, status: 'error' }));
    ws.onclose = () => setTerminalDrawer(prev => ({ ...prev, status: 'closed' }));
    ws.onmessage = event => {
      try {
        const frame = JSON.parse(event.data);
        if (frame?.op === 'stdout' || frame?.op === 'stderr') {
          setTerminalDrawer(prev => ({
            ...prev,
            output: (prev.output || '') + cleanTerminalOutput(frame.data || '')
          }));
        } else if (frame?.op === 'error') {
          setTerminalDrawer(prev => ({
            ...prev,
            output: (prev.output || '') + `\n${cleanTerminalOutput(frame.data || '终端出错')}\n`,
            status: 'error'
          }));
        }
      } catch (err) {
        setTerminalDrawer(prev => ({
          ...prev,
          output: (prev.output || '') + cleanTerminalOutput(event.data)
        }));
      }
    };
    return () => {
      ws.close();
      terminalSocketRef.current = null;
    };
  }, [terminalDrawer.visible, terminalDrawer.pod, terminalDrawer.container, terminalDrawer.shell, clusterId]);

  useEffect(() => {
    if (terminalDrawer.visible && terminalInputRef.current) {
      terminalInputRef.current.focus();
    }
  }, [terminalDrawer.visible]);

  useEffect(() => {
    if (terminalOutputRef.current) {
      terminalOutputRef.current.scrollTop = terminalOutputRef.current.scrollHeight;
    }
  }, [terminalDrawer.output]);

  useEffect(
    () => () => {
      if (terminalSocketRef.current) {
        terminalSocketRef.current.close();
      }
    },
    []
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

  const handleDeletePod = useCallback(
    (pod, refreshFn) => {
      if (!pod) return;
      Modal.confirm({
        title: `删除 Pod ${pod.name}?`,
        content: '该操作会立即删除 Pod，谨慎执行。',
        okText: '删除',
        okButtonProps: { danger: true },
        onOk: async () => {
          try {
            await deleteResource(clusterId, {
              group: '',
              version: 'v1',
              resource: 'pods',
              namespace: pod.namespace,
              name: pod.name
            });
            message.success('Pod 删除成功');
            if (typeof refreshFn === 'function') {
              refreshFn();
            }
          } catch (err) {
            message.error(err?.message || '删除 Pod 失败');
          }
        }
      });
    },
    [clusterId]
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

  const openDetailDrawer = useCallback(record => {
    setDetailRecord(record);
    setDetailTab('overview');
    setWorkloadDetail(null);
    setHistory([]);
    setEvents([]);
    setEventsTotal(0);
    setEventsPage(1);
    setLogsContent('');
    setAutoRefreshLogs(false);
    setDetailDrawerVisible(true);
  }, []);

  const closeDetailDrawer = useCallback(() => {
    setDetailDrawerVisible(false);
    setDetailRecord(null);
    setWorkloadDetail(null);
    setHistory([]);
    setEvents([]);
    setEventsTotal(0);
    setLogsContent('');
    setAutoRefreshLogs(false);
    setDetailTab('overview');
  }, []);

  const fetchWorkloadDetail = useCallback(async () => {
    if (!clusterId || !detailRecord) return;
    setDetailLoading(true);
    try {
      const resp = await getWorkloadDetails(clusterId, {
        kind: (detailRecord.kind || '').toLowerCase(),
        namespace: detailRecord.namespace || '',
        name: detailRecord.name
      });
      setWorkloadDetail(resp || null);
    } catch (err) {
      message.error(err.message || '加载详情失败');
    } finally {
      setDetailLoading(false);
    }
  }, [clusterId, detailRecord]);

  const fetchHistoryEntries = useCallback(async () => {
    if (!clusterId || !detailRecord) return;
    if ((detailRecord.kind || '').toLowerCase() !== 'deployment') {
      setHistory([]);
      return;
    }
    setHistoryLoading(true);
    try {
      const resp = await getWorkloadHistory(clusterId, {
        kind: (detailRecord.kind || '').toLowerCase(),
        namespace: detailRecord.namespace || '',
        name: detailRecord.name
      });
      setHistory(Array.isArray(resp) ? resp : []);
    } catch (err) {
      message.error(err.message || '加载历史版本失败');
    } finally {
      setHistoryLoading(false);
    }
  }, [clusterId, detailRecord]);

  const fetchEventList = useCallback(
    async (targetPage = 1) => {
      if (!clusterId || !detailRecord) return;
      setEventsLoading(true);
      try {
        const resp = await listResourceEvents(clusterId, {
          namespace: detailRecord.namespace || '',
          kind: detailRecord.kind,
          name: detailRecord.name,
          page: targetPage,
          perPage: 10
        });
        const normalized = (resp?.items || []).map((item, index) => ({
          key: `${item.reason || 'event'}-${item.last_timestamp || index}-${index}`,
          ...item,
          firstTime: item.first_timestamp,
          lastTime: item.last_timestamp
        }));
        setEvents(normalized);
        setEventsTotal(resp?.total ?? normalized.length);
        setEventsPage(targetPage);
      } catch (err) {
        message.error(err.message || '加载事件失败');
      } finally {
        setEventsLoading(false);
      }
    },
    [clusterId, detailRecord]
  );

  const fetchLogs = useCallback(
    async tailOverride => {
      if (!clusterId || !detailRecord) return;
      setLogsLoading(true);
      try {
        const resp = await getWorkloadLogs(clusterId, {
          kind: (detailRecord.kind || '').toLowerCase(),
          namespace: detailRecord.namespace || '',
          name: detailRecord.name,
          allContainers: true,
          tail: tailOverride ?? logsTail
        });
        setLogsContent(resp?.content || '');
      } catch (err) {
        setLogsContent('');
        message.error(err.message || '加载日志失败');
      } finally {
        setLogsLoading(false);
      }
    },
    [clusterId, detailRecord, logsTail]
  );

  const handleRollback = useCallback(
    entry => {
      if (!detailRecord || !entry?.revision) return;
      Modal.confirm({
        title: `回滚到版本 ${entry.revision}`,
        content: '该操作会使用历史版本模板重新部署，请确认。',
        okText: '回滚',
        cancelText: '取消',
        okButtonProps: { danger: true },
        centered: true,
        onOk: async () => {
          try {
            await rollbackWorkload(clusterId, {
              kind: (detailRecord.kind || '').toLowerCase(),
              namespace: detailRecord.namespace || '',
              name: detailRecord.name,
              revision: entry.revision
            });
            message.success('已触发回滚');
            fetchWorkloadDetail();
            fetchHistoryEntries();
          } catch (err) {
            message.error(err.message || '回滚失败');
          }
        }
      });
    },
    [clusterId, detailRecord, fetchHistoryEntries, fetchWorkloadDetail]
  );

  const handleDetailTabChange = useCallback(
    key => {
      setDetailTab(key);
      if (key === 'logs') {
        fetchLogs();
      }
      if (key === 'events') {
        fetchEventList(1);
      }
    },
    [fetchEventList, fetchLogs]
  );

  const handleCopyLogs = useCallback(async () => {
    if (!logsContent) {
      message.info('暂无日志内容');
      return;
    }
    try {
      if (navigator?.clipboard?.writeText) {
        await navigator.clipboard.writeText(logsContent);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = logsContent;
        textarea.style.position = 'fixed';
        textarea.style.left = '-9999px';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
      }
      message.success('日志已复制');
    } catch (err) {
      message.error(err.message || '复制失败');
    }
  }, [logsContent]);

  const handleDownloadLogs = useCallback(() => {
    if (!logsContent) {
      message.info('暂无日志内容');
      return;
    }
    const blob = new Blob([logsContent], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${detailRecord?.name || 'workload'}-logs.txt`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }, [logsContent, detailRecord]);

  const handleLogsTailChange = useCallback(
    value => {
      const next = Number(value) || 200;
      setLogsTail(next);
      if (detailDrawerVisible && detailTab === 'logs') {
        fetchLogs(next);
      }
    },
    [detailDrawerVisible, detailTab, fetchLogs]
  );

  useEffect(() => {
    if (detailDrawerVisible && detailRecord) {
      fetchWorkloadDetail();
      fetchHistoryEntries();
      fetchEventList(1);
    }
  }, [detailDrawerVisible, detailRecord, fetchEventList, fetchHistoryEntries, fetchWorkloadDetail]);

  useEffect(() => {
    if (!autoRefreshLogs || detailTab !== 'logs' || !detailDrawerVisible) {
      return undefined;
    }
    const timer = setInterval(() => {
      fetchLogs();
    }, 5000);
    return () => clearInterval(timer);
  }, [autoRefreshLogs, detailDrawerVisible, detailTab, fetchLogs]);

  useEffect(() => {
    if (detailTab !== 'logs' && autoRefreshLogs) {
      setAutoRefreshLogs(false);
    }
  }, [detailTab, autoRefreshLogs]);

  useEffect(() => {
    if (!detailDrawerVisible && autoRefreshLogs) {
      setAutoRefreshLogs(false);
    }
  }, [detailDrawerVisible, autoRefreshLogs]);

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (_, record) => (
        <div className="workload-name">
          <button type="button" className="workload-name__title" onClick={() => openDetailDrawer(record)}>
            {record.name}
          </button>
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

  const detailPods = useMemo(() => {
    if (!workloadDetail?.pods) return [];
    return workloadDetail.pods.map(pod => ({
      ...pod,
      age: formatPodAge((pod.created_at || 0) * 1000)
    }));
  }, [workloadDetail]);

  const overview = workloadDetail?.overview || {};
  const workloadMeta = workloadDetail?.workload || detailRecord || {};
  const replica = overview.replica || {};
  const conditions = overview.conditions || [];
  const containers = overview.containers || [];
  const volumes = workloadDetail?.volumes || [];
  const configMaps = workloadDetail?.configmaps || [];
  const secrets = workloadDetail?.secrets || [];
  const pvcs = workloadDetail?.pvcs || [];
  const services = workloadDetail?.services || [];
  const ingresses = workloadDetail?.ingresses || [];
  const endpoints = workloadDetail?.endpoints || [];
  const configTableData = (configMaps || []).map((item, index) => ({
    key: `config-${item.name || index}`,
    name: item.name,
    namespace: item.namespace || workloadMeta.namespace || '—',
    type: item.kind || 'ConfigMap',
    labels: item.labels || {}
  }));
  const secretTableData = (secrets || []).map((item, index) => ({
    key: `secret-${item.name || index}`,
    name: item.name,
    namespace: item.namespace || workloadMeta.namespace || '—',
    type: item.type || item.kind || 'Secret',
    labels: item.labels || {}
  }));

  const renderMapTags = map => {
    if (!map || !Object.keys(map).length) {
      return '—';
    }
    return (
      <Space size={[6, 6]} wrap>
        {Object.entries(map).map(([key, value]) => (
          <Tag bordered={false} key={`${key}-${value}`} className="workload-detail__tag">
            {key}: {value}
          </Tag>
        ))}
      </Space>
    );
  };

  const renderKeyValueGrid = items => (
    <div className="workload-detail__grid">
      {items.map(item => (
        <div className="workload-detail__item" key={item.label}>
          <div className="workload-detail__item-label">{item.label}</div>
          <div className="workload-detail__item-value">{item.value ?? '—'}</div>
        </div>
      ))}
    </div>
  );

  const renderConditions = () =>
    conditions && conditions.length ? (
      <div className="workload-detail__conditions">
        {conditions.map(cond => (
          <div key={`${cond.type}-${cond.last_transition_time}`} className="workload-detail__condition">
            <div className="workload-detail__condition-header">
              <strong>{cond.type}</strong>
              <Tag color={cond.status === 'True' ? 'green' : cond.status === 'False' ? 'red' : 'blue'}>
                {cond.status}
              </Tag>
            </div>
            <div className="workload-detail__condition-meta">
              <span>{formatTime(cond.last_transition_time)}</span>
              {cond.reason ? <span>{cond.reason}</span> : null}
            </div>
            {cond.message ? <p>{cond.message}</p> : null}
          </div>
        ))}
      </div>
    ) : (
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无条件信息" />
    );

  const renderContainers = () =>
    containers && containers.length ? (
      <div className="workload-detail__containers">
        {containers.map(container => (
          <div className="workload-detail__container" key={`${container.name}-${container.init ? 'init' : 'normal'}`}>
            <div className="workload-detail__container-header">
              <div>
                <strong>{container.name}</strong>
                {container.init ? <Tag color="blue">Init</Tag> : null}
              </div>
              <span>{container.image}</span>
            </div>
            <div className="workload-detail__container-body">
              <div>
                <span className="workload-detail__item-label">镜像</span>
                <div className="workload-detail__item-value">{container.image || '—'}</div>
              </div>
              <div>
                <span className="workload-detail__item-label">端口</span>
                <div className="workload-detail__item-value">
                  {container.ports && container.ports.length ? container.ports.join(', ') : '—'}
                </div>
              </div>
              <div>
                <span className="workload-detail__item-label">命令</span>
                <div className="workload-detail__item-value">
                  {container.command && container.command.length ? container.command.join(' ') : '—'}
                </div>
              </div>
              <div>
                <span className="workload-detail__item-label">参数</span>
                <div className="workload-detail__item-value">
                  {container.args && container.args.length ? container.args.join(' ') : '—'}
                </div>
              </div>
              <div>
                <span className="workload-detail__item-label">变量</span>
                <div className="workload-detail__item-value">
                  {container.env && container.env.length ? container.env.join(', ') : '—'}
                </div>
              </div>
              <div className="workload-detail__container-flags">
                {container.liveness_probe ? <Tag color="purple">Liveness</Tag> : null}
                {container.readiness_probe ? <Tag color="green">Readiness</Tag> : null}
              </div>
            </div>
          </div>
        ))}
      </div>
    ) : (
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无容器信息" />
    );

  const volumeColumns = [
    { title: '卷', dataIndex: 'name', width: 200 },
    { title: '引用资源', dataIndex: 'source', render: value => value || '—' },
    { title: '类型', dataIndex: 'kind', width: 180 }
  ];

  const volumeData = (volumes || []).map(volume => ({
    key: `${volume.name}-${volume.source_name}`,
    name: volume.name,
    kind: volume.kind,
    source: volume.source_name
  }));

  const serviceColumns = [
    { title: '名称', dataIndex: 'name', width: 220 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '类型', dataIndex: 'type', width: 120 },
    { title: '标签', dataIndex: 'labels', render: value => renderMapTags(value) }
  ];

  const endpointColumns = [
    { title: '名称', dataIndex: 'name', width: 240 },
    { title: '类型', dataIndex: 'kind', width: 160 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 }
  ];

  const ingressColumns = [
    { title: '名称', dataIndex: 'name', width: 240 },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '标签', dataIndex: 'labels', render: value => renderMapTags(value) }
  ];

  const historyColumns = [
    { title: '版本', dataIndex: 'revision', width: 120 },
    { title: '镜像', dataIndex: 'images', render: value => (value && value.length ? value.join(', ') : '—') },
    { title: '创建时间', dataIndex: 'created_at', width: 200, render: value => formatTime(value) || '—' },
    { title: '来源', dataIndex: 'source', width: 220 },
    {
      title: '操作',
      width: 120,
      render: (_, record) => (
        <Button type="link" onClick={() => handleRollback(record)}>
          回滚
        </Button>
      )
    }
  ];

  const eventColumns = [
    { title: '类型', dataIndex: 'type', width: 120 },
    { title: '原因', dataIndex: 'reason', width: 140 },
    { title: '消息', dataIndex: 'message' },
    { title: '次数', dataIndex: 'count', width: 80 },
    { title: '首次发生', dataIndex: 'firstTime', width: 200, render: value => formatTime(value) || '—' },
    { title: '最近发生', dataIndex: 'lastTime', width: 200, render: value => formatTime(value) || '—' }
  ];

  const configColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      width: 240,
      render: value => <span className="workload-name__title workload-name__title--static">{value}</span>
    },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: '类型', dataIndex: 'type', width: 180 },
    { title: '标签', dataIndex: 'labels', render: value => renderMapTags(value) }
  ];

  const secretColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      width: 240,
      render: value => <span className="workload-name__title workload-name__title--static">{value}</span>
    },
    { title: '命名空间', dataIndex: 'namespace', width: 160 },
    { title: 'Secret 类型', dataIndex: 'type', width: 200 },
    { title: '标签', dataIndex: 'labels', render: value => renderMapTags(value) }
  ];

  const renderReferenceList = (title, list) => (
    <div className="workload-detail__reference" key={title}>
      <span className="workload-detail__reference-title">{title}</span>
      {list && list.length ? (
        <Space size={[6, 6]} wrap>
          {list.map(item => (
            <Tag bordered={false} key={`${title}-${item.name}`} className="workload-detail__tag">
              {item.name}
            </Tag>
          ))}
        </Space>
      ) : (
        <span className="workload-detail__muted">暂无</span>
      )}
    </div>
  );

  const strategy = overview.strategy || {};
  const strategyExtra = [
    strategy.max_surge ? `MaxSurge ${strategy.max_surge}` : '',
    strategy.max_unavailable ? `MaxUnavailable ${strategy.max_unavailable}` : '',
    typeof strategy.partition === 'number' ? `Partition ${strategy.partition}` : ''
  ]
    .filter(Boolean)
    .join(' / ');

  const overviewContent = (
    <Spin spinning={detailLoading}>
      {workloadDetail ? (
        <div className="workload-detail__scroll">
          <div className="workload-detail__section">
            <div className="workload-detail__section-title">基本信息</div>
            {renderKeyValueGrid([
              { label: '名称', value: overview.name || workloadMeta.name || '—' },
              { label: '命名空间', value: overview.namespace || workloadMeta.namespace || '—' },
              { label: '标签', value: renderMapTags(overview.labels || workloadMeta.labels) },
              { label: '注解', value: renderMapTags(overview.annotations) },
              { label: '选择器', value: renderMapTags(overview.selector) },
              { label: '创建时间', value: formatTime(overview.creation_timestamp) || '—' },
              { label: '最近更新', value: formatTime(overview.update_timestamp) || '—' }
            ])}
          </div>

          <div className="workload-detail__section">
            <div className="workload-detail__section-title">更多信息</div>
            {renderKeyValueGrid([
              { label: '副本', value: `${replica.ready ?? 0} / ${replica.desired ?? 0}` },
              { label: '可用副本', value: replica.available ?? '—' },
              { label: '已更新副本', value: replica.updated ?? '—' },
              {
                label: '更新策略',
                value: (
                  <span>
                    {strategy.type || '—'}
                    {strategyExtra ? <span className="workload-detail__muted">（{strategyExtra}）</span> : null}
                  </span>
                )
              }
            ])}
          </div>

          <div className="workload-detail__section">
            <div className="workload-detail__section-title">条件</div>
            {renderConditions()}
          </div>

          <div className="workload-detail__section">
            <div className="workload-detail__section-title">容器</div>
            {renderContainers()}
          </div>

          <div className="workload-detail__section">
            <div className="workload-detail__section-title">存储</div>
            <Table
              columns={volumeColumns}
              dataSource={volumeData}
              rowKey="key"
              size="small"
              pagination={false}
              locale={{ emptyText: '暂无存储引用' }}
            />
          </div>

          <div className="workload-detail__section workload-detail__reference-grid">
            <div className="workload-detail__section-title">引用配置</div>
            {renderReferenceList('ConfigMap', configMaps)}
            {renderReferenceList('Secret', secrets)}
            {renderReferenceList('PVC', pvcs)}
          </div>
        </div>
      ) : (
        <Empty description="暂无详情" />
      )}
    </Spin>
  );

  const podsContent =
    detailLoading || detailPods.length ? (
      <PodsTable
        pods={detailPods}
        loading={detailLoading}
        onTerminal={openPodTerminal}
        onLogs={openPodLogs}
        onDelete={pod => handleDeletePod(pod, fetchWorkloadDetail)}
      />
    ) : (
      <Empty description="暂无实例" />
    );

  const servicesData = (services || []).map((item, index) => ({ key: `${item.name}-${index}`, ...item }));
  const endpointsData = (endpoints || []).map((item, index) => ({ key: `${item.name}-${index}`, ...item }));
  const ingressData = (ingresses || []).map((item, index) => ({ key: `${item.name}-${index}`, ...item }));

  const serviceTab = (
    <div className="workload-detail__tab-body">
      <Space style={{ marginBottom: 12 }}>
        <Button type="primary" disabled>
          创建服务
        </Button>
      </Space>
      <Table
        columns={serviceColumns}
        dataSource={servicesData}
        rowKey="key"
        size="small"
        pagination={false}
        locale={{ emptyText: '暂无服务' }}
      />
      <div className="workload-detail__subsection-title">Endpoint</div>
      <Table
        columns={endpointColumns}
        dataSource={endpointsData}
        rowKey="key"
        size="small"
        pagination={false}
        locale={{ emptyText: '暂无 Endpoint' }}
      />
    </div>
  );

  const ingressTab = (
    <div className="workload-detail__tab-body">
      <Space style={{ marginBottom: 12 }}>
        <Button type="primary" disabled>
          创建路由
        </Button>
      </Space>
      <Table
        columns={ingressColumns}
        dataSource={ingressData}
        rowKey="key"
        size="small"
        pagination={false}
        locale={{ emptyText: '暂无路由' }}
      />
    </div>
  );

  const accessContent = (
    <Tabs
      size="small"
      items={[
        { key: 'service', label: '服务', children: serviceTab },
        { key: 'ingress', label: '路由规则', children: ingressTab }
      ]}
    />
  );

  const configsContent = (
    <div className="workload-detail__tab-body">
      <div className="workload-detail__subsection-title">ConfigMap</div>
      <Table
        columns={configColumns}
        dataSource={configTableData}
        rowKey="key"
        size="small"
        pagination={false}
        locale={{ emptyText: '暂无 ConfigMap' }}
      />
      <div className="workload-detail__subsection-title">Secret</div>
      <Table
        columns={secretColumns}
        dataSource={secretTableData}
        rowKey="key"
        size="small"
        pagination={false}
        locale={{ emptyText: '暂无 Secret' }}
      />
    </div>
  );

  const historyData = (history || []).map(entry => ({ key: entry.revision, ...entry }));
  const historyContent =
    (detailRecord?.kind || '').toLowerCase() === 'deployment' ? (
      <Table
        columns={historyColumns}
        dataSource={historyData}
        loading={historyLoading}
        rowKey="key"
        pagination={false}
        size="small"
        locale={{ emptyText: '暂无历史版本' }}
      />
    ) : (
      <Empty description="仅 Deployment 支持历史版本" />
    );

  const scalingTabs = [
    {
      key: 'metrics',
      label: '指标伸缩',
      children: (
        <div className="workload-detail__tab-body">
          <Space style={{ marginBottom: 12 }}>
            <Button type="primary" disabled>
              创建指标伸缩
            </Button>
          </Space>
          <Empty description="暂未配置指标伸缩" />
        </div>
      )
    },
    {
      key: 'schedule',
      label: '定时伸缩',
      children: (
        <div className="workload-detail__tab-body">
          <Space style={{ marginBottom: 12 }}>
            <Button type="primary" disabled>
              创建定时伸缩
            </Button>
          </Space>
          <Empty description="暂未配置定时伸缩" />
        </div>
      )
    }
  ];

  const eventsContent = (
    <Table
      columns={eventColumns}
      dataSource={events}
      loading={eventsLoading}
      rowKey="key"
      size="small"
      pagination={{
        current: eventsPage,
        pageSize: 10,
        total: eventsTotal,
        showSizeChanger: false,
        onChange: page => fetchEventList(page)
      }}
      locale={{ emptyText: '暂无事件' }}
    />
  );

  const logsTabContent = (
    <div className="workload-logs">
      <div className="workload-logs__toolbar">
        <Space size={[12, 12]} wrap>
          <Space>
            <span>尾行</span>
            <InputNumber min={50} max={2000} step={50} value={logsTail} onChange={handleLogsTailChange} />
          </Space>
          <Space>
            <span>自动换行</span>
            <Switch checked={logWrap} onChange={setLogWrap} />
          </Space>
          <Space>
            <span>自动更新</span>
            <Switch
              checked={autoRefreshLogs}
              onChange={checked => {
                setAutoRefreshLogs(checked);
                if (checked) {
                  fetchLogs();
                }
              }}
            />
          </Space>
          <Button onClick={() => fetchLogs()} loading={logsLoading}>
            刷新日志
          </Button>
          <Button onClick={handleCopyLogs} disabled={!logsContent}>
            复制日志
          </Button>
          <Button onClick={handleDownloadLogs} disabled={!logsContent}>
            下载日志
          </Button>
        </Space>
      </div>
      <div className={`workload-logs__viewer ${logWrap ? 'workload-logs__viewer--wrap' : ''}`}>
        {logsLoading ? <Spin /> : <pre>{logsContent || '暂无日志输出'}</pre>}
      </div>
    </div>
  );

  const detailTabItems = [
    { key: 'overview', label: '概览', children: overviewContent },
    { key: 'pods', label: '实例列表', children: podsContent },
    { key: 'access', label: '访问方式', children: accessContent },
    { key: 'config', label: '配置', children: configsContent },
    { key: 'history', label: '历史版本', children: historyContent },
    { key: 'scaling', label: '弹性伸缩', children: <Tabs size="small" items={scalingTabs} /> },
    { key: 'events', label: '事件', children: eventsContent },
    { key: 'logs', label: '日志', children: logsTabContent }
  ];

  return (
    <>
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
        dataSource={paginatedResources}
        size="middle"
        scroll={{ x: 'max-content' }}
        expandable={{
          expandedRowKeys,
          expandedRowRender: record => (
            <PodsTable
              pods={podsCache[record.key] || []}
              loading={!!podsLoading[record.key]}
              onTerminal={openPodTerminal}
              onLogs={openPodLogs}
              onDelete={pod => handleDeletePod(pod, () => loadPods(record, true))}
            />
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
        pagination={false}
      />
      <TablePagination
        page={page}
        pageSize={pageSize}
        total={filteredResources.length}
        onChange={(nextPage, nextSize) => {
          setPage(nextPage);
          setPageSize(nextSize);
        }}
        className="table-pagination--flush"
      />
      </Card>
      <Drawer
        className="workload-detail-drawer"
        width={1080}
        title={detailRecord ? `${detailRecord.kind} / ${detailRecord.name}` : '工作负载详情'}
        open={detailDrawerVisible}
        onClose={closeDetailDrawer}
        destroyOnClose={false}
        maskClosable
      >
        <Tabs
          activeKey={detailTab}
          onChange={handleDetailTabChange}
          items={detailTabItems}
          className="workload-detail__tabs"
        />
      </Drawer>
      <Drawer
        width={720}
        title={podLogsDrawer.pod ? `Pod 日志 · ${podLogsDrawer.pod.name}` : 'Pod 日志'}
        open={podLogsDrawer.visible}
        onClose={closePodLogs}
        destroyOnClose
        maskClosable
      >
        <Space style={{ marginBottom: 12 }} wrap>
          <Select
            placeholder="选择容器"
            style={{ minWidth: 200 }}
            value={podLogsDrawer.container || undefined}
            onChange={handlePodLogsContainerChange}
            options={(podLogsDrawer.pod?.containers || []).map(container => ({ value: container, label: container }))}
          />
          <Button onClick={refreshPodLogs} disabled={!podLogsDrawer.container} loading={podLogsDrawer.loading}>
            刷新
          </Button>
        </Space>
        <div className="pod-log-viewer">
          <Spin spinning={podLogsDrawer.loading}>
            <pre>{podLogsDrawer.content || '暂无日志'}</pre>
          </Spin>
        </div>
      </Drawer>
      <Drawer
        className="pod-terminal-drawer"
        width={980}
        title={terminalDrawer.pod ? `Pod 终端 · ${terminalDrawer.pod.name}` : 'Pod 终端'}
        open={terminalDrawer.visible}
        onClose={closeTerminalDrawer}
        destroyOnClose
        maskClosable
      >
        <div className="pod-terminal-toolbar">
          <Space size={[8, 8]} wrap>
            <Space>
              <span>容器</span>
              <Select
                style={{ minWidth: 200 }}
                value={terminalDrawer.container || undefined}
                onChange={handleTerminalContainerChange}
                options={(terminalDrawer.pod?.containers || []).map(container => ({ value: container, label: container }))}
                placeholder="选择容器"
              />
            </Space>
            <Tag color="blue">Shell: {terminalDrawer.shell === 'sh' ? 'sh' : 'bash'}</Tag>
            <Tag color={terminalDrawer.status === 'connected' ? 'green' : terminalDrawer.status === 'error' ? 'red' : 'default'}>
              {terminalDrawer.status === 'connected'
                ? '已连接'
                : terminalDrawer.status === 'error'
                ? '连接失败'
                : terminalDrawer.status === 'closed'
                ? '已断开'
                : '连接中'}
            </Tag>
            <Button onClick={handleTerminalClear}>清屏</Button>
          </Space>
        </div>
        <div className="pod-terminal" onClick={() => terminalInputRef.current?.focus()}>
          <pre ref={terminalOutputRef}>
            {terminalDrawer.output || '暂无终端输出'}
            {terminalDrawer.visible && terminalDrawer.status === 'connected' ? <span className="pod-terminal__cursor" /> : null}
          </pre>
          <textarea
            ref={terminalInputRef}
            className="pod-terminal__input"
            onKeyDown={handleTerminalKeyDown}
            spellCheck={false}
          />
        </div>
        <div className="pod-terminal__hint">按 Enter 发送命令，Ctrl+C 终止当前执行。</div>
      </Drawer>
    </>
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
