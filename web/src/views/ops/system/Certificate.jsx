import React, { useCallback, useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  message
} from 'antd';
import dayjs from 'dayjs';
import {
  listCertificates,
  createCertificate,
  updateCertificate,
  deleteCertificate,
  getCertificate
} from '../../../api/system/certificates';
import './certificate.less';

const TYPE_OPTIONS = [
  { value: 'git', label: 'Git' },
  { value: 'docker', label: 'Docker Registry' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'ldap', label: 'LDAP' },
  { value: 'kubernetes', label: 'Kubernetes' },
  { value: 'custom', label: '自定义' }
];

const DEFAULT_FORM_VALUES = {
  name: '',
  type: 'git',
  config: '{\n  "username": "",\n  "password": ""\n}'
};

const Certificate = () => {
  const [loading, setLoading] = useState(false);
  const [certificates, setCertificates] = useState([]);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');
  const [searchInput, setSearchInput] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [modalLoading, setModalLoading] = useState(false);
  const [editing, setEditing] = useState(null);
  const [detail, setDetail] = useState({ visible: false, record: null });
  const [form] = Form.useForm();
  const location = useLocation();

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params = {
        page,
        per_page: perPage
      };
      if (search.trim()) {
        params.name = search.trim();
      }
      if (typeFilter !== 'all') {
        params.type = typeFilter;
      }
      const resp = await listCertificates(params);
      const items = resp?.items || [];
      setCertificates(items);
      setTotal(resp?.total || items.length);
      setPage(resp?.page || page);
      setPerPage(resp?.per_page || perPage);
    } catch (err) {
      message.error(err.message || '加载凭证失败');
    } finally {
      setLoading(false);
    }
  }, [page, perPage, search, typeFilter]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const nameParam = params.get('name');
    if (nameParam) {
      setSearch(nameParam);
      setSearchInput(nameParam);
    }
  }, [location.search]);

  useEffect(() => {
    setPage(1);
  }, [search, typeFilter]);

  const openCreateModal = () => {
    setEditing(null);
    form.setFieldsValue(DEFAULT_FORM_VALUES);
    setModalVisible(true);
  };

  const openEditModal = async record => {
    setEditing(record);
    try {
      const detailResp = await getCertificate(record.id, { reveal: true });
      const configString = JSON.stringify(detailResp?.config || {}, null, 2);
      form.setFieldsValue({
        name: detailResp?.name || record.name,
        type: detailResp?.type || record.type,
        config: configString
      });
    } catch (err) {
      const configString = JSON.stringify(record.config || {}, null, 2);
      form.setFieldsValue({
        name: record.name,
        type: record.type,
        config: configString
      });
      message.warning('获取凭证详情失败，已加载当前记录信息');
    }
    setModalVisible(true);
  };

  const closeModal = () => {
    setModalVisible(false);
    setEditing(null);
    form.resetFields();
  };

  const handleModalSubmit = async () => {
    try {
      const values = await form.validateFields();
      let parsedConfig = {};
      if (values.config && values.config.trim()) {
        try {
          parsedConfig = JSON.parse(values.config);
        } catch (err) {
          message.error('Config 字段必须是合法的 JSON');
          return;
        }
      }
      const payload = {
        name: values.name.trim(),
        type: values.type,
        config: parsedConfig
      };
      setModalLoading(true);
      if (editing) {
        await updateCertificate(editing.id, payload);
        message.success('凭证已更新');
      } else {
        await createCertificate(payload);
        message.success('凭证已创建');
      }
      closeModal();
      fetchData();
    } catch (err) {
      if (err?.errorFields) {
        return;
      }
      message.error(err.message || '保存失败');
    } finally {
      setModalLoading(false);
    }
  };

  const handleDelete = record => {
    Modal.confirm({
      title: '删除凭证',
      content: `确定删除凭证 “${record.name}” 吗？该操作不可恢复。`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteCertificate(record.id);
          message.success('凭证已删除');
          fetchData();
        } catch (err) {
          message.error(err.message || '删除失败');
        }
      }
    });
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (text, record) => (
        <Button type="link" onClick={() => setDetail({ visible: true, record })}>
          {text}
        </Button>
      )
    },
    {
      title: '类型',
      dataIndex: 'type',
      render: value => <Tag color="processing">{value}</Tag>
    },
    {
      title: '最近更新',
      dataIndex: 'updated',
      width: 200,
      render: value => (value ? dayjs(value * 1000).format('YYYY-MM-DD HH:mm') : '-')
    },
    {
      title: '操作',
      width: 180,
      render: (_, record) => (
        <Space>
          <Button type="link" onClick={() => openEditModal(record)}>
            编辑
          </Button>
          <Button type="link" danger onClick={() => handleDelete(record)}>
            删除
          </Button>
        </Space>
      )
    }
  ];

  const paginationConfig = {
    current: page,
    pageSize: perPage,
    total,
    showSizeChanger: true,
    onChange: (p, size) => {
      setPage(p);
      setPerPage(size);
    }
  };

  return (
    <div className="ops-certificate">
      <Card
        title="凭证管理"
        extra={
          <Space>
            <Input.Search
              allowClear
              placeholder="搜索名称"
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              onSearch={value => {
                setSearch(value.trim());
                setSearchInput(value.trim());
              }}
              style={{ width: 220 }}
            />
            <Select
              value={typeFilter}
              onChange={value => setTypeFilter(value)}
              style={{ width: 150 }}
              options={[{ value: 'all', label: '全部类型' }, ...TYPE_OPTIONS]}
            />
            <Button type="primary" onClick={openCreateModal}>
              新建凭证
            </Button>
          </Space>
        }
        bodyStyle={{ paddingTop: 16 }}
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={certificates}
          pagination={paginationConfig}
        />
      </Card>

      <Drawer
        title={editing ? '编辑凭证' : '新建凭证'}
        open={modalVisible}
        onClose={closeModal}
        width={520}
        extra={
          <Space>
            <Button onClick={closeModal}>取消</Button>
            <Button type="primary" loading={modalLoading} onClick={handleModalSubmit}>
              保存
            </Button>
          </Space>
        }
      >
        <Form layout="vertical" form={form} initialValues={DEFAULT_FORM_VALUES}>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入凭证名称' }]}
          >
            <Input placeholder="例如：gitlab-token" />
          </Form.Item>
          <Form.Item
            name="type"
            label="类型"
            rules={[{ required: true, message: '请选择凭证类型' }]}
          >
            <Select options={TYPE_OPTIONS} />
          </Form.Item>
          <Form.Item
            name="config"
            label={
              <Space>
                <span>配置 (JSON / 文本)</span>
                <Tooltip title="使用标准 JSON 键值表示凭证内容，例如包含 username/password、token 等">
                  <Tag color="default">JSON</Tag>
                </Tooltip>
              </Space>
            }
            rules={[{ required: true, message: '请输入配置' }]}
          >
            <Input.TextArea
              rows={10}
              placeholder={`{
  "username": "",
  "password": ""
}`}
            />
          </Form.Item>
        </Form>
      </Drawer>

      <Drawer
        title={detail.record ? `凭证详情 · ${detail.record.name}` : '凭证详情'}
        open={detail.visible}
        onClose={() => setDetail({ visible: false, record: null })}
        width={520}
      >
        {detail.record && (
          <div className="ops-certificate__detail">
            <p>
              <strong>类型：</strong>
              <Tag color="processing">{detail.record.type}</Tag>
            </p>
            <p>
              <strong>最近更新时间：</strong>
              {detail.record.updated ? dayjs(detail.record.updated * 1000).format('YYYY-MM-DD HH:mm:ss') : '—'}
            </p>
            <p>
              <strong>配置：</strong>
            </p>
            <pre className="ops-certificate__json">
              {JSON.stringify(detail.record.config || {}, null, 2)}
            </pre>
            {detail.record.masked_fields?.length ? (
              <p>
                <strong>已隐藏字段：</strong>
                {detail.record.masked_fields.join(', ')}
              </p>
            ) : null}
          </div>
        )}
      </Drawer>
    </div>
  );
};

export default Certificate;
