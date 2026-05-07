import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Badge,
  Button,
  Empty,
  List,
  message as antdMessage,
  Popconfirm,
  Space,
  Tabs,
  Tag,
  Tooltip,
  Typography
} from 'antd';
import { CheckOutlined, ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { listMessages, markAllRead, markRead } from '../../api/system/messages';
import OpsPageCard, { opsPageTitle } from '../../components/OpsPageCard';

const { Text, Paragraph } = Typography;

const TYPE_COLOR = {
  info: 'blue',
  warn: 'gold',
  error: 'red'
};

const SOURCE_LABEL = {
  system: '系统',
  pipeline: '流水线',
  alert: '告警',
  rbac: '权限'
};

const PAGE_SIZE = 20;

const MessageNotification = () => {
  const [tab, setTab] = useState('unread');
  const [data, setData] = useState({ items: [], total: 0, unread: 0, page: 1, per_page: PAGE_SIZE });
  const [loading, setLoading] = useState(false);

  const load = useCallback(async (nextTab = tab, page = 1) => {
    setLoading(true);
    try {
      const res = await listMessages({
        page,
        per_page: PAGE_SIZE,
        unread: nextTab === 'unread' ? true : undefined
      });
      const r = (res || {}) as Record<string, unknown>;
      setData({
        items: Array.isArray(r.items) ? r.items : [],
        total: Number(r.total) || 0,
        unread: Number(r.unread) || 0,
        page: Number(r.page) || 1,
        per_page: Number(r.per_page) || PAGE_SIZE
      });
    } finally {
      setLoading(false);
    }
  }, [tab]);

  useEffect(() => {
    load(tab, 1);
  }, [tab, load]);

  const handleRead = async ids => {
    try {
      await markRead(ids);
      antdMessage.success('已标记已读');
      load(tab, data.page);
    } catch (err) {
      // request 工具已弹错误
    }
  };

  const handleReadAll = async () => {
    try {
      await markAllRead();
      antdMessage.success('全部已读');
      load(tab, 1);
    } catch (err) {
      // ignored
    }
  };

  const onTabChange = key => {
    setTab(key);
  };

  const tabItems = useMemo(() => [
    {
      key: 'unread',
      label: <Space><span>未读</span><Badge count={data.unread} /></Space>
    },
    {
      key: 'all',
      label: '全部'
    }
  ], [data.unread]);

  return (
    <OpsPageCard
      title={opsPageTitle('消息通知', '消息通知')}
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => load(tab, data.page)} loading={loading}>
            刷新
          </Button>
          <Popconfirm
            title="把所有未读消息标记为已读?"
            onConfirm={handleReadAll}
            disabled={data.unread === 0}
          >
            <Button type="primary" disabled={data.unread === 0}>全部已读</Button>
          </Popconfirm>
        </Space>
      }
    >
      <Tabs activeKey={tab} onChange={onTabChange} items={tabItems} />
      {data.items.length === 0 ? (
        <Empty description={tab === 'unread' ? '没有未读消息' : '没有消息'} />
      ) : (
        <List
          loading={loading}
          dataSource={data.items}
          rowKey="id"
          pagination={{
            current: data.page,
            pageSize: data.per_page,
            total: data.total,
            onChange: page => load(tab, page)
          }}
          renderItem={item => (
            <List.Item
              actions={item.read_at === 0 ? [
                <Tooltip key="read" title="标为已读">
                  <Button
                    type="link"
                    icon={<CheckOutlined />}
                    onClick={() => handleRead([item.id])}
                  >
                    标读
                  </Button>
                </Tooltip>
              ] : []}
            >
              <List.Item.Meta
                title={
                  <Space>
                    <Text strong>{item.title}</Text>
                    {item.read_at === 0 && <Badge status="processing" />}
                    <Tag color={TYPE_COLOR[item.type] || 'default'}>
                      {item.type}
                    </Tag>
                    <Tag>{SOURCE_LABEL[item.source] || item.source}</Tag>
                  </Space>
                }
                description={
                  <Space direction="vertical" size={2} style={{ width: '100%' }}>
                    <Paragraph style={{ marginBottom: 0 }}>{item.content || ''}</Paragraph>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {item.created ? dayjs.unix(item.created).format('YYYY-MM-DD HH:mm:ss') : ''}
                      {item.read_at > 0 && (
                        <> · 已读于 {dayjs.unix(item.read_at).format('YYYY-MM-DD HH:mm:ss')}</>
                      )}
                    </Text>
                  </Space>
                }
              />
            </List.Item>
          )}
        />
      )}
    </OpsPageCard>
  );
};

export default MessageNotification;
