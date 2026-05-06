import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { listAuditLogs } from '../../api/system/audit';

const { Text } = Typography;
const { RangePicker } = DatePicker;

const METHOD_COLOR = {
  POST: 'green',
  PUT: 'orange',
  PATCH: 'orange',
  DELETE: 'red'
};

// statusColor 把 HTTP 状态码映射到 antd Tag 颜色, 让审计页快速看出失败请求.
const statusColor = status => {
  if (!status) return 'default';
  if (status >= 500) return 'red';
  if (status >= 400) return 'orange';
  if (status >= 300) return 'gold';
  return 'green';
};

const SystemAudit = () => {
  const [data, setData] = useState<{ items: unknown[]; total: number; page: number; per_page: number }>({
    items: [],
    total: 0,
    page: 1,
    per_page: 20
  });
  const [loading, setLoading] = useState(false);
  const [params, setParams] = useState<Record<string, unknown>>({ page: 1, per_page: 20 });
  const [form] = Form.useForm();

  const load = useCallback(async (next = params) => {
    setLoading(true);
    try {
      const res = await listAuditLogs(next);
      const r = (res || {}) as Record<string, unknown>;
      setData({
        items: Array.isArray(r.items) ? r.items : [],
        total: Number(r.total) || 0,
        page: Number(r.page) || 1,
        per_page: Number(r.per_page) || 20
      });
      setParams(next);
    } finally {
      setLoading(false);
    }
  }, [params]);

  useEffect(() => {
    load({ page: 1, per_page: 20 });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onSearch = async () => {
    const v = await form.validateFields();
    const range = v.range || [];
    load({
      page: 1,
      per_page: params.per_page,
      login: (v.login || '').trim() || undefined,
      method: v.method || undefined,
      path: (v.path || '').trim() || undefined,
      start: range[0] ? range[0].unix() : undefined,
      end: range[1] ? range[1].unix() : undefined
    });
  };

  const onReset = () => {
    form.resetFields();
    load({ page: 1, per_page: params.per_page });
  };

  const columns = useMemo(() => [
    {
      title: '时间',
      dataIndex: 'created',
      key: 'created',
      width: 170,
      render: ts => (ts ? dayjs.unix(ts).format('YYYY-MM-DD HH:mm:ss') : '-')
    },
    {
      title: '用户',
      key: 'user',
      width: 160,
      render: row => (
        <Space direction="vertical" size={0}>
          <Text strong>{row.login || '-'}</Text>
          <Text type="secondary">#{row.user_id}</Text>
        </Space>
      )
    },
    {
      title: 'Method',
      dataIndex: 'method',
      key: 'method',
      width: 90,
      render: m => <Tag color={METHOD_COLOR[m] || 'blue'}>{m}</Tag>
    },
    {
      title: '路径',
      dataIndex: 'path',
      key: 'path',
      ellipsis: true,
      render: p => <Text code>{p}</Text>
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: s => <Tag color={statusColor(s)}>{s || '-'}</Tag>
    },
    {
      title: '耗时',
      dataIndex: 'duration',
      key: 'duration',
      width: 90,
      render: d => `${d || 0} ms`
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
      width: 140
    },
    {
      title: '参数',
      dataIndex: 'summary',
      key: 'summary',
      ellipsis: true,
      render: s => s ? (
        <Tooltip title={<pre style={{ margin: 0 }}>{s}</pre>}>
          <Text type="secondary">{s}</Text>
        </Tooltip>
      ) : '-'
    }
  ], []);

  return (
    <Card
      title="操作审计"
      bordered={false}
      extra={
        <Button icon={<ReloadOutlined />} onClick={() => load(params)} loading={loading}>
          刷新
        </Button>
      }
    >
      <Form form={form} layout="inline" style={{ marginBottom: 16 }}>
        <Form.Item name="login" label="用户">
          <Input allowClear placeholder="用户名" style={{ width: 140 }} />
        </Form.Item>
        <Form.Item name="method" label="方法">
          <Select
            allowClear
            placeholder="选择方法"
            style={{ width: 110 }}
            options={['POST', 'PUT', 'PATCH', 'DELETE'].map(m => ({ label: m, value: m }))}
          />
        </Form.Item>
        <Form.Item name="path" label="路径">
          <Input allowClear placeholder="支持模糊匹配" style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="range" label="时间">
          <RangePicker showTime allowClear />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" onClick={onSearch}>查询</Button>
            <Button onClick={onReset}>重置</Button>
          </Space>
        </Form.Item>
      </Form>
      <Table
        rowKey="id"
        loading={loading}
        dataSource={data.items}
        columns={columns}
        pagination={{
          current: data.page,
          pageSize: data.per_page,
          total: data.total,
          showSizeChanger: true,
          pageSizeOptions: [20, 50, 100],
          onChange: (page, pageSize) => load({ ...params, page, per_page: pageSize })
        }}
      />
    </Card>
  );
};

export default SystemAudit;
