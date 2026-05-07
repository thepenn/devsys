import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Drawer, Dropdown, Input, Select, Space, Table, Tag, message } from 'antd';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';
import { listNamespaces, listResources } from '../../api/admin/k8s';
import { formatPodAge, formatTime } from '../../utils/time';
import TablePagination from '../../components/TablePagination';
import OpsPageCard from '../../components/OpsPageCard';
import K8sClusterGuard from './K8sClusterGuard';
import { API_BASE_URL } from '../../utils/request';
import { getToken } from '../../utils/auth';
import './resource-tables.less';
import './workloads.less';

const ALL_NAMESPACE = '__all__';

const PodsContent = ({ clusterId }) => {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState(ALL_NAMESPACE);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [pods, setPods] = useState([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [terminalDrawer, setTerminalDrawer] = useState({
    visible: false,
    pod: null,
    container: '',
    shell: 'bash',
    status: 'idle'
  });
  const terminalSocketRef = useRef(null);
  const terminalRef = useRef(null);
  const terminalContainerRef = useRef(null);
  const terminalFitAddonRef = useRef(null);
  const terminalOpenedRef = useRef(false);
  const terminalDecoderRef = useRef(null);

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

  const openPodTerminal = useCallback((pod, shellType = 'bash') => {
    if (!pod) return;
    const container = Array.isArray(pod.containers) && pod.containers.length ? pod.containers[0] : '';
    setTerminalDrawer({
      visible: true,
      pod,
      container,
      shell: shellType,
      status: container ? 'connecting' : 'error'
    });
  }, []);

  const closeTerminalSocket = useCallback(() => {
    if (terminalSocketRef.current) {
      terminalSocketRef.current.close();
      terminalSocketRef.current = null;
    }
  }, []);

  const closeTerminalDrawer = useCallback(() => {
    closeTerminalSocket();
    setTerminalDrawer({ visible: false, pod: null, container: '', shell: 'bash', status: 'idle' });
  }, [closeTerminalSocket]);

  const sendTerminalFrame = useCallback(data => {
    if (!data || !terminalSocketRef.current || terminalSocketRef.current.readyState !== WebSocket.OPEN) {
      return;
    }
    try {
      terminalSocketRef.current.send(data);
    } catch (err) {
      // ignore
    }
  }, []);

  const sendTerminalResize = useCallback(() => {
    const ws = terminalSocketRef.current;
    const term = terminalRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN || !term) {
      return;
    }
    try {
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
    } catch (err) {
      // ignore
    }
  }, []);

  const ensureTerminal = useCallback(() => {
    if (terminalRef.current) {
      return;
    }
    const term = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontSize: 14,
      fontFamily: "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
      theme: {
        background: '#0f172a',
        foreground: '#e2e8f0'
      }
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.onData(chunk => sendTerminalFrame(chunk));
    term.onResize(() => sendTerminalResize());
    terminalRef.current = term;
    terminalFitAddonRef.current = fitAddon;
  }, [sendTerminalFrame, sendTerminalResize]);

  const openTerminal = useCallback(() => {
    ensureTerminal();
    if (!terminalRef.current || !terminalContainerRef.current || !terminalFitAddonRef.current) {
      return;
    }
    if (!terminalOpenedRef.current) {
      terminalRef.current.open(terminalContainerRef.current);
      terminalOpenedRef.current = true;
    }
    terminalFitAddonRef.current.fit();
    terminalRef.current.focus();
  }, [ensureTerminal]);

  const writeTerminal = useCallback(data => {
    if (data === undefined || data === null || !terminalRef.current) {
      return;
    }
    terminalRef.current.write(String(data));
  }, []);

  const handleTerminalContainerChange = value => {
    setTerminalDrawer(prev => ({
      ...prev,
      container: value,
      status: value ? 'connecting' : 'error'
    }));
  };

  const handleTerminalClear = () => {
    terminalRef.current?.clear();
    terminalRef.current?.focus();
  };

  useEffect(() => {
    if (!terminalDrawer.visible) {
      return;
    }
    openTerminal();
    terminalRef.current?.reset();
    if (!terminalDrawer.pod || !terminalDrawer.container) {
      writeTerminal('该 Pod 无可用容器。\r\n');
      setTerminalDrawer(prev => ({ ...prev, status: 'error' }));
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
    setTerminalDrawer(prev => ({ ...prev, status: 'connecting' }));
    writeTerminal(`正在连接终端 ${terminalDrawer.pod.name}/${terminalDrawer.container} ...\r\n`);
    const ws = new WebSocket(wsUrl);
    terminalSocketRef.current = ws;

    ws.onopen = () => {
      if (terminalSocketRef.current !== ws) return;
      setTerminalDrawer(prev => ({ ...prev, status: 'connected' }));
      sendTerminalResize();
      terminalRef.current?.focus();
    };

    ws.onerror = () => {
      if (terminalSocketRef.current !== ws) return;
      setTerminalDrawer(prev => ({ ...prev, status: 'error' }));
      writeTerminal('\r\n终端连接失败\r\n');
    };

    ws.onclose = () => {
      if (terminalSocketRef.current !== ws) return;
      setTerminalDrawer(prev => ({ ...prev, status: 'closed' }));
      writeTerminal('\r\n终端已断开\r\n');
    };

    ws.onmessage = async event => {
      if (terminalSocketRef.current !== ws) return;
      if (typeof event.data === 'string') {
        writeTerminal(event.data);
        return;
      }
      if (event.data instanceof ArrayBuffer) {
        const decoder = terminalDecoderRef.current || new TextDecoder('utf-8');
        terminalDecoderRef.current = decoder;
        const text = decoder.decode(new Uint8Array(event.data));
        writeTerminal(text);
        return;
      }
      if (event.data instanceof Blob) {
        const text = await event.data.text();
        if (terminalSocketRef.current !== ws) return;
        writeTerminal(text);
      }
    };

    return () => {
      if (terminalSocketRef.current === ws) {
        closeTerminalSocket();
      }
    };
  }, [
    closeTerminalSocket,
    clusterId,
    openTerminal,
    sendTerminalResize,
    terminalDrawer.visible,
    terminalDrawer.pod,
    terminalDrawer.container,
    terminalDrawer.shell,
    writeTerminal
  ]);

  useEffect(() => {
    if (!terminalDrawer.visible) {
      return;
    }
    const fit = () => {
      terminalFitAddonRef.current?.fit();
      sendTerminalResize();
    };
    const timer = window.setTimeout(fit, 100);
    window.addEventListener('resize', fit);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener('resize', fit);
    };
  }, [sendTerminalResize, terminalDrawer.visible]);

  useEffect(
    () => () => {
      closeTerminalSocket();
      terminalDecoderRef.current = null;
      if (terminalRef.current) {
        terminalRef.current.dispose();
        terminalRef.current = null;
      }
      terminalFitAddonRef.current = null;
      terminalOpenedRef.current = false;
    },
    [closeTerminalSocket]
  );

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
    { title: '运行时长', dataIndex: 'age', width: 160 },
    {
      title: '操作',
      width: 110,
      render: (_, record) => (
        <Dropdown
          trigger={['click']}
          menu={{
            items: [
              { key: 'bash', label: 'Bash 终端' },
              { key: 'sh', label: 'Sh 终端' }
            ],
            onClick: ({ key }) => openPodTerminal(record, key)
          }}
        >
          <Button type="link" size="small">
            终端
          </Button>
        </Dropdown>
      )
    }
  ];

  return (
    <>
      <OpsPageCard
        bodyVariant="tableFlush"
        title="K8s 管理 · Pods"
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
      </OpsPageCard>

      <Drawer
        className="pod-terminal-drawer"
        title={terminalDrawer.pod ? `终端 · ${terminalDrawer.pod.name}` : '终端'}
        open={terminalDrawer.visible}
        onClose={closeTerminalDrawer}
        width={900}
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
        <div className="pod-terminal pod-terminal--xterm" onClick={() => terminalRef.current?.focus()}>
          <div ref={terminalContainerRef} className="pod-terminal__xterm" />
        </div>
        <div className="pod-terminal__hint">已切换为 xterm 交互终端，支持标准键盘输入（Backspace、方向键、Tab、Ctrl+C）。</div>
      </Drawer>
    </>
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
    const spec = item.spec || {};
    const containerStatuses = status.containerStatuses || [];
    const containers = spec.containers || [];
    const readyCount = containerStatuses.filter(cs => cs.ready).length;
    const total = containerStatuses.length || containers.length || 0;
    const nodeName = spec.nodeName || '-';
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
      containers: containers.map(container => container.name).filter(Boolean)
    };
  });
}

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

const buildWsUrl = (path: string, params: Record<string, string | number | boolean | undefined> = {}) => {
  if (typeof window === 'undefined') return '';
  const base = ensureAbsoluteBase(API_BASE_URL).replace(/\/+$/, '');
  const suffix = path.startsWith('/') ? path : `/${path}`;
  const url = new URL(`${base}${suffix}`);
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      url.searchParams.set(key, String(value));
    }
  });
  const token = getToken();
  if (token) {
    url.searchParams.set('token', token);
  }
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
};
