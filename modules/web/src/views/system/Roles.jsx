import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Avatar,
  Button,
  Card,
  Checkbox,
  Drawer,
  Form,
  Input,
  message,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography
} from 'antd';
import {
  assignUserRoles,
  createRole,
  deleteRole,
  listEndpoints,
  listLabels,
  listRoles,
  listUserRoles,
  updateRole
} from '../../api/system/rbac';

const { Title, Text } = Typography;

// 同后端 internal/label/label.go 中的内置 role 名
const BUILTIN_ROLE_TITLES = {
  superadmin: '超级管理员',
  admin: '管理员',
  ops: '运维',
  developer: '开发者',
  guest: '访客'
};

const RolesPanel = ({ roles, labels, onChanged }) => {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form] = Form.useForm();

  const labelsByModule = useMemo(() => {
    const map = new Map();
    labels.forEach(label => {
      if (!map.has(label.module)) {
        map.set(label.module, []);
      }
      map.get(label.module).push(label);
    });
    return Array.from(map.entries()).map(([module, items]) => ({ module: module || '其他', items }));
  }, [labels]);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ name: '', title: '', parents: [], labels: [] });
    setDrawerOpen(true);
  };

  const openEdit = role => {
    setEditing(role);
    form.setFieldsValue({
      name: role.name,
      title: role.title,
      parents: role.parents || [],
      labels: role.labels || []
    });
    setDrawerOpen(true);
  };

  const handleSubmit = async () => {
    const values = await form.validateFields();
    try {
      if (editing) {
        await updateRole(editing.id, values);
        message.success('角色已更新');
      } else {
        await createRole(values);
        message.success('角色已创建');
      }
      setDrawerOpen(false);
      onChanged();
    } catch (err) {
      // request util already pops error message
    }
  };

  const handleDelete = async role => {
    try {
      await deleteRole(role.id);
      message.success('角色已删除');
      onChanged();
    } catch (err) {
      // ignored
    }
  };

  const columns = [
    {
      title: '角色',
      dataIndex: 'name',
      key: 'name',
      render: (value, row) => (
        <Space direction="vertical" size={0}>
          <Text strong>{value}</Text>
          <Text type="secondary">{row.title || BUILTIN_ROLE_TITLES[value] || ''}</Text>
        </Space>
      )
    },
    {
      title: '继承',
      dataIndex: 'parents',
      key: 'parents',
      render: parents => (
        <Space wrap size={4}>
          {(parents || []).map(p => (
            <Tag key={p}>{p}</Tag>
          ))}
        </Space>
      )
    },
    {
      title: 'Label',
      dataIndex: 'labels',
      key: 'labels',
      render: labelsField => (
        <Space wrap size={4}>
          {(labelsField || []).map(l => (
            <Tag key={l} color={l === '*' ? 'magenta' : 'blue'}>{l}</Tag>
          ))}
        </Space>
      )
    },
    {
      title: '类型',
      dataIndex: 'builtin',
      key: 'builtin',
      width: 100,
      render: builtin => (builtin ? <Tag color="gold">内置</Tag> : <Tag>自定义</Tag>)
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_, row) => (
        <Space>
          <Button type="link" onClick={() => openEdit(row)}>编辑</Button>
          {!row.builtin && (
            <Popconfirm title={`确认删除角色 ${row.name}?`} onConfirm={() => handleDelete(row)}>
              <Button type="link" danger>删除</Button>
            </Popconfirm>
          )}
        </Space>
      )
    }
  ];

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" onClick={openCreate}>新建角色</Button>
      </Space>
      <Table rowKey="id" dataSource={roles} columns={columns} pagination={false} />

      <Drawer
        title={editing ? `编辑角色: ${editing.name}` : '新建角色'}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={560}
        extra={
          <Space>
            <Button onClick={() => setDrawerOpen(false)}>取消</Button>
            <Button type="primary" onClick={handleSubmit}>保存</Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="角色名 (英文标识)"
            rules={[{ required: true, message: '请填写角色名' }]}
          >
            <Input placeholder="例如 db-readonly" disabled={!!editing && editing.builtin} />
          </Form.Item>
          <Form.Item name="title" label="显示名称">
            <Input placeholder="例如 数据库只读" />
          </Form.Item>
          <Form.Item name="parents" label="继承自">
            <Select
              mode="multiple"
              allowClear
              placeholder="选择父角色, 子角色将继承父角色的所有 label"
              options={roles
                .filter(r => !editing || r.id !== editing.id)
                .map(r => ({ label: `${r.name} (${r.title || BUILTIN_ROLE_TITLES[r.name] || ''})`, value: r.name }))}
            />
          </Form.Item>
          <Form.Item name="labels" label="Label (按模块分组)">
            <LabelGroupPicker groups={labelsByModule} />
          </Form.Item>
        </Form>
      </Drawer>
    </>
  );
};

const LabelGroupPicker = ({ value, onChange, groups }) => {
  const selected = value || [];
  const toggle = (name, checked) => {
    const next = checked
      ? Array.from(new Set([...selected, name]))
      : selected.filter(n => n !== name);
    onChange?.(next);
  };
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {groups.map(group => (
        <Card key={group.module} size="small" title={group.module}>
          <Space wrap>
            {group.items.map(label => (
              <Checkbox
                key={label.id}
                checked={selected.includes(label.name)}
                onChange={e => toggle(label.name, e.target.checked)}
              >
                <Space size={4}>
                  <Text code>{label.name}</Text>
                  <Text type="secondary">{label.title}</Text>
                </Space>
              </Checkbox>
            ))}
          </Space>
        </Card>
      ))}
    </div>
  );
};

const UsersPanel = ({ roles, users, onChanged }) => {
  const [editing, setEditing] = useState(null);
  const [picked, setPicked] = useState([]);

  const beginEdit = user => {
    setEditing(user);
    setPicked(user.roles || []);
  };

  const submit = async () => {
    try {
      await assignUserRoles(editing.user_id, picked);
      message.success('用户角色已更新');
      setEditing(null);
      onChanged();
    } catch (err) {
      // ignored
    }
  };

  const columns = [
    {
      title: '用户',
      key: 'login',
      render: row => (
        <Space>
          <Avatar src={row.avatar_url} size="small">
            {(row.login || '?').slice(0, 1).toUpperCase()}
          </Avatar>
          <Space direction="vertical" size={0}>
            <Text strong>{row.login}</Text>
            <Text type="secondary">{row.email}</Text>
          </Space>
        </Space>
      )
    },
    {
      title: 'OAuth Admin',
      dataIndex: 'admin',
      key: 'admin',
      width: 140,
      render: admin => (admin ? <Tag color="red">是</Tag> : <Tag>否</Tag>)
    },
    {
      title: '角色',
      dataIndex: 'roles',
      key: 'roles',
      render: assigned => (
        <Space wrap>
          {(assigned || []).map(r => (
            <Tag key={r} color="blue">{r}</Tag>
          ))}
        </Space>
      )
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: row => (
        <Button type="link" onClick={() => beginEdit(row)}>分配角色</Button>
      )
    }
  ];

  return (
    <>
      <Table rowKey="user_id" dataSource={users} columns={columns} pagination={{ pageSize: 20 }} />
      <Modal
        title={editing ? `分配角色: ${editing.login}` : ''}
        open={!!editing}
        onCancel={() => setEditing(null)}
        onOk={submit}
        okText="保存"
        cancelText="取消"
      >
        <Select
          mode="multiple"
          allowClear
          style={{ width: '100%' }}
          placeholder="选择角色"
          value={picked}
          onChange={setPicked}
          options={roles.map(r => ({
            label: `${r.name} (${r.title || BUILTIN_ROLE_TITLES[r.name] || ''})`,
            value: r.name
          }))}
        />
      </Modal>
    </>
  );
};

const EndpointsPanel = ({ endpoints }) => {
  const columns = [
    { title: 'Method', dataIndex: 'method', key: 'method', width: 100, render: m => <Tag color="purple">{m}</Tag> },
    { title: '路径', dataIndex: 'path', key: 'path' },
    { title: '模块', dataIndex: 'module', key: 'module', width: 160 },
    { title: '说明', dataIndex: 'remark', key: 'remark' },
    {
      title: 'Label',
      dataIndex: 'labels',
      key: 'labels',
      render: labels => (
        <Space wrap size={4}>
          {(labels || []).map(l => (
            <Tag key={l} color="blue">{l}</Tag>
          ))}
        </Space>
      )
    }
  ];
  return <Table rowKey="id" dataSource={endpoints} columns={columns} pagination={{ pageSize: 30 }} />;
};

const SystemRoles = () => {
  const [loading, setLoading] = useState(true);
  const [roles, setRoles] = useState([]);
  const [labels, setLabels] = useState([]);
  const [endpoints, setEndpoints] = useState([]);
  const [users, setUsers] = useState([]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [r, l, e, u] = await Promise.all([
        listRoles(),
        listLabels(),
        listEndpoints(),
        listUserRoles()
      ]);
      setRoles(Array.isArray(r) ? r : []);
      setLabels(Array.isArray(l) ? l : []);
      setEndpoints(Array.isArray(e) ? e : []);
      setUsers(Array.isArray(u) ? u : []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <Card loading={loading} bordered={false}>
      <Title level={4} style={{ marginTop: 0 }}>角色与权限</Title>
      <Text type="secondary">
        权限链路: OAuth 登录 → 用户 → 角色 → label → 接口 (METHOD + Path).
        admin / superadmin 角色不可删除; superadmin 拥有 <Text code>*</Text> 通配 label.
      </Text>
      <Tabs
        defaultActiveKey="roles"
        style={{ marginTop: 16 }}
        items={[
          {
            key: 'roles',
            label: '角色管理',
            children: <RolesPanel roles={roles} labels={labels} onChanged={refresh} />
          },
          {
            key: 'users',
            label: '用户授权',
            children: <UsersPanel roles={roles} users={users} onChanged={refresh} />
          },
          {
            key: 'endpoints',
            label: '接口目录',
            children: <EndpointsPanel endpoints={endpoints} />
          }
        ]}
      />
    </Card>
  );
};

export default SystemRoles;
