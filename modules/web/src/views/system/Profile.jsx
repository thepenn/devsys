import React from 'react';
import { Card, Descriptions, Avatar, Space, Tag, Spin } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import { useAuth } from 'context/AuthContext';

const ROLE_COLOR = {
  superadmin: 'red',
  admin: 'orange',
  ops: 'geekblue',
  developer: 'blue',
  guest: 'default'
};

const SystemProfile = () => {
  const { user, loading, roles, labels } = useAuth();

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div style={{ padding: 24 }}>
      <Card title="个人信息">
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Avatar size={80} src={user?.avatar_url} icon={<UserOutlined />} />
          <h3 style={{ marginTop: 12, marginBottom: 0 }}>{user?.login || '-'}</h3>
        </div>
        <Descriptions column={1} bordered>
          <Descriptions.Item label="用户名">{user?.login || '-'}</Descriptions.Item>
          <Descriptions.Item label="邮箱">{user?.email || '-'}</Descriptions.Item>
          <Descriptions.Item label="认证来源">{user?.provider || '-'}</Descriptions.Item>
          <Descriptions.Item label="角色">
            {roles && roles.length > 0 ? (
              <Space wrap size={4}>
                {roles.map(r => (
                  <Tag key={r} color={ROLE_COLOR[r] || 'blue'}>{r}</Tag>
                ))}
              </Space>
            ) : (
              <Tag>未分配</Tag>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="生效权限">
            {labels && labels.length > 0 ? (
              <Space wrap size={4}>
                {labels.map(l => (
                  <Tag key={l} color={l === '*' ? 'magenta' : 'blue'}>{l}</Tag>
                ))}
              </Space>
            ) : (
              <Tag>无</Tag>
            )}
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
};

export default SystemProfile;
