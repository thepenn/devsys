import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, Form, Input, Modal, Popconfirm, Radio, Segmented, Space, Table, Tag, Tooltip, message } from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { hasAnyLabel } from '../../utils/permission';
import { createTemplate, deleteTemplate, listTemplates } from '../../api/system/pipelineTemplates';
import { formatTime } from '../../utils/time';
import { itemsFromListResponse } from '../../utils/listResponse';
import TablePagination from '../../components/TablePagination';
import './pipeline-templates.less';

const TEMPLATE_NAME_PATTERN = /^[A-Za-z][A-Za-z0-9_-]{0,127}$/;

const KIND_OPTIONS = [
  { label: '全部', value: '' },
  { label: '完整 Pipeline', value: 'pipeline' },
  { label: '步骤模板', value: 'step' }
];

const renderKindTag = kind =>
  kind === 'step' ? <Tag color="purple">步骤模板</Tag> : <Tag color="geekblue">完整 Pipeline</Tag>;

const PipelineTemplateList = () => {
  const navigate = useNavigate();
  const { user } = useAuth();
  // pipeline_template:write 是管理员角色, 与默认 RBAC 配置对齐.
  const canWrite = useMemo(() => hasAnyLabel(user, ['pipeline_template:write']), [user]);

  const [items, setItems] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [kindFilter, setKindFilter] = useState('');
  const [loading, setLoading] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [createForm] = Form.useForm();

  const fetchList = useCallback(
    async (targetPage = page, overrides: Record<string, unknown> = {}) => {
      const nextPerPage = overrides.perPage || perPage;
      const nextKeyword = overrides.keyword !== undefined ? overrides.keyword : keyword;
      const nextKind = overrides.kind !== undefined ? overrides.kind : kindFilter;
      setLoading(true);
      try {
        const data = (await listTemplates({
          page: targetPage,
          per_page: nextPerPage,
          keyword: nextKeyword || undefined,
          kind: nextKind || undefined
        })) as { items?: unknown[]; total?: number; page?: number; per_page?: number };
        setItems(itemsFromListResponse(data));
        setTotal(Number(data?.total || 0));
        setPage(Number(data?.page || targetPage));
        if (data?.per_page) {
          setPerPage(Number(data.per_page));
        } else if (overrides.perPage != null) {
          setPerPage(Number(overrides.perPage));
        }
      } catch (err) {
        message.error(err?.message || '加载模板列表失败');
      } finally {
        setLoading(false);
      }
    },
    [page, perPage, keyword, kindFilter]
  );

  useEffect(() => {
    fetchList(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleSearch = value => {
    setKeyword(value);
    fetchList(1, { keyword: value });
  };

  const handleEdit = useCallback(
    record => {
      navigate(`/ops/pipeline-templates/${record.id}`);
    },
    [navigate]
  );

  const handleDelete = useCallback(
    async record => {
      try {
        await deleteTemplate(record.id);
        message.success('模板已删除');
        fetchList(page);
      } catch (err) {
        // 后端 409 时 err.message 含 "still in use", 给出更友好的提示.
        const text = err?.message || '删除失败';
        if (text.includes('in use') || text.includes('引用')) {
          message.error('该模板仍被项目引用, 请先在项目中切换为其它模板或自定义 YAML');
        } else {
          message.error(text);
        }
      }
    },
    [fetchList, page]
  );

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreateSubmitting(true);
      const created = (await createTemplate({
        name: values.name.trim(),
        display_name: (values.display_name || values.name).trim(),
        description: (values.description || '').trim(),
        kind: values.kind || 'pipeline',
        draft_content: values.draft_content || ''
      })) as { id: number };
      message.success('模板已创建');
      setCreateOpen(false);
      createForm.resetFields();
      navigate(`/ops/pipeline-templates/${created.id}`);
    } catch (err) {
      if (err?.errorFields) return;
      const text = err?.message || '创建失败';
      if (text.includes('exists') || text.includes('conflict') || text.includes('重复')) {
        message.error('该模板名已存在, 请换一个');
      } else {
        message.error(text);
      }
    } finally {
      setCreateSubmitting(false);
    }
  };

  const columns = useMemo(
    () => [
      {
        title: '名称',
        dataIndex: 'name',
        render: (_, record) => (
          <button type="button" className="pipeline-template__link" onClick={() => handleEdit(record)}>
            <strong>{record.display_name || record.name}</strong>
            <span className="pipeline-template__id">{record.name}</span>
          </button>
        )
      },
      {
        title: '类型',
        dataIndex: 'kind',
        width: 130,
        render: value => renderKindTag(value)
      },
      {
        title: '描述',
        dataIndex: 'description',
        render: value => (value ? <span className="pipeline-template__desc">{value}</span> : '—')
      },
      {
        title: '发布状态',
        dataIndex: 'is_published',
        width: 200,
        render: (_, record) =>
          record.is_published ? (
            <Tooltip title={`发布于 ${formatTime(record.published_at)}${record.published_by ? ' · ' + record.published_by : ''}`}>
              <Tag color="green">已发布</Tag>
            </Tooltip>
          ) : (
            <Tag color="orange">仅草稿</Tag>
          )
      },
      {
        title: '引用项目数',
        dataIndex: 'referenced_by',
        width: 120,
        render: value => <Tag color={value > 0 ? 'blue' : 'default'}>{value || 0}</Tag>
      },
      {
        title: '最后更新',
        dataIndex: 'updated',
        width: 200,
        render: value => formatTime(value)
      },
      {
        title: '操作',
        dataIndex: 'actions',
        width: 200,
        fixed: 'right' as const,
        render: (_, record) => (
          <Space>
            <Button size="small" onClick={() => handleEdit(record)}>
              编辑
            </Button>
            {canWrite && (
              <Popconfirm
                title="确认删除该模板?"
                description={record.referenced_by > 0 ? '当前仍有项目引用, 删除将被拒绝' : '删除后无法恢复'}
                okText="删除"
                okButtonProps={{ danger: true }}
                onConfirm={() => handleDelete(record)}
                disabled={record.referenced_by > 0}
              >
                <Button size="small" danger disabled={record.referenced_by > 0}>
                  删除
                </Button>
              </Popconfirm>
            )}
          </Space>
        )
      }
    ],
    [canWrite, handleDelete, handleEdit]
  );

  return (
    <Card
      className="pipeline-template-card"
      title="项目管理 · 通用 Pipeline 模板"
      extra={
        <Space size={12}>
          <Segmented
            options={KIND_OPTIONS}
            value={kindFilter}
            onChange={value => {
              const v = String(value);
              setKindFilter(v);
              fetchList(1, { kind: v });
            }}
          />
          <Input.Search
            placeholder="搜索模板名 / 显示名"
            allowClear
            onSearch={handleSearch}
            style={{ width: 260 }}
            enterButton="搜索"
          />
          <Tooltip title="刷新列表">
            <Button icon={<ReloadOutlined />} onClick={() => fetchList(page)} />
          </Tooltip>
          {canWrite && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              新建模板
            </Button>
          )}
        </Space>
      }
    >
      <Table
        rowKey="id"
        columns={columns}
        loading={loading}
        dataSource={items}
        pagination={false}
      />
      <TablePagination
        page={page}
        pageSize={perPage}
        total={total}
        onChange={(nextPage, nextSize) => fetchList(nextPage, { perPage: nextSize })}
        className="table-pagination--flush"
      />

      <Modal
        open={createOpen}
        title="新建通用 Pipeline 模板"
        onCancel={() => {
          setCreateOpen(false);
          createForm.resetFields();
        }}
        onOk={handleCreate}
        confirmLoading={createSubmitting}
        destroyOnClose
        width={560}
      >
        <Form form={createForm} layout="vertical" preserve={false} initialValues={{ kind: 'pipeline' }}>
          <Form.Item
            label="模板类型"
            name="kind"
            rules={[{ required: true }]}
            extra="完整 Pipeline = 项目整体引用; 步骤模板 = 项目用 [组装] 模式拼装"
          >
            <Radio.Group>
              <Radio.Button value="pipeline">完整 Pipeline</Radio.Button>
              <Radio.Button value="step">步骤模板</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item
            label="模板标识 (name)"
            name="name"
            rules={[
              { required: true, message: '请填写模板标识' },
              {
                pattern: TEMPLATE_NAME_PATTERN,
                message: '以字母开头, 仅含字母 / 数字 / -/_, 不超过 128 字符'
              }
            ]}
            extra="项目侧引用模板用. 创建后不可修改"
          >
            <Input placeholder="例如 go-service-default 或 step-clone-code" />
          </Form.Item>
          <Form.Item label="显示名称 (display_name)" name="display_name">
            <Input placeholder="缺省与模板标识相同" />
          </Form.Item>
          <Form.Item label="描述" name="description">
            <Input.TextArea rows={3} placeholder="给团队成员看的简短说明" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default PipelineTemplateList;
