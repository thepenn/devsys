import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, Form, Input, Modal, Popconfirm, Space, Table, Tag, Tooltip, message } from 'antd';
import { PlayCircleOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { hasAnyLabel } from '../../utils/permission';
import { createJob, deleteJob, listJobs, triggerJob } from '../../api/system/pipelineJobs';
import { formatTime } from '../../utils/time';
import { formatPipelineStatus, getPipelineStatusClass } from '../../constants/pipeline';
import TablePagination from '../../components/TablePagination';
import '../pipelineTemplates/pipeline-templates.less';
import './pipeline-jobs.less';

const JOB_NAME_PATTERN = /^[A-Za-z][A-Za-z0-9_-]{0,127}$/;

const PipelineJobList = () => {
  const navigate = useNavigate();
  const { user } = useAuth();
  const canWrite = useMemo(() => hasAnyLabel(user, ['pipeline_job:write']), [user]);
  const canTrigger = useMemo(() => hasAnyLabel(user, ['pipeline_job:trigger']), [user]);

  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [triggering, setTriggering] = useState({});

  const [createOpen, setCreateOpen] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [createForm] = Form.useForm();

  const fetchList = useCallback(
    async (targetPage = page, overrides = {}) => {
      const nextPerPage = overrides.perPage || perPage;
      const nextKeyword = overrides.keyword !== undefined ? overrides.keyword : keyword;
      setLoading(true);
      try {
        const data = await listJobs({
          page: targetPage,
          per_page: nextPerPage,
          keyword: nextKeyword || undefined
        });
        setItems(Array.isArray(data?.items) ? data.items : []);
        setTotal(Number(data?.total || 0));
        setPage(Number(data?.page || targetPage));
        if (data?.per_page) {
          setPerPage(Number(data.per_page));
        } else if (overrides.perPage) {
          setPerPage(overrides.perPage);
        }
      } catch (err) {
        message.error(err?.message || '加载 Job 列表失败');
      } finally {
        setLoading(false);
      }
    },
    [page, perPage, keyword]
  );

  useEffect(() => {
    fetchList(1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleEdit = useCallback(
    record => navigate(`/ops/pipeline-jobs/${record.id}`),
    [navigate]
  );

  const handleTrigger = useCallback(
    async record => {
      if (!record?.id) return;
      setTriggering(prev => ({ ...prev, [record.id]: true }));
      try {
        const run = await triggerJob(record.id, {});
        message.success('已触发运行');
        navigate(`/ops/pipeline-jobs/${record.id}/runs/${run.id}`);
      } catch (err) {
        message.error(err?.message || '触发失败');
      } finally {
        setTriggering(prev => ({ ...prev, [record.id]: false }));
      }
    },
    [navigate]
  );

  const handleDelete = useCallback(
    async record => {
      try {
        await deleteJob(record.id);
        message.success('已删除');
        fetchList(page);
      } catch (err) {
        message.error(err?.message || '删除失败');
      }
    },
    [fetchList, page]
  );

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreateSubmitting(true);
      const created = await createJob({
        name: values.name.trim(),
        display_name: (values.display_name || values.name).trim(),
        description: (values.description || '').trim()
      });
      message.success('Job 已创建');
      setCreateOpen(false);
      createForm.resetFields();
      navigate(`/ops/pipeline-jobs/${created.id}`);
    } catch (err) {
      if (err?.errorFields) return;
      const text = err?.message || '创建失败';
      if (text.includes('exists') || text.includes('conflict') || text.includes('重复')) {
        message.error('该 Job 名已存在');
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
        title: '描述',
        dataIndex: 'description',
        render: value => (value ? <span className="pipeline-template__desc">{value}</span> : '—')
      },
      {
        title: 'Git',
        dataIndex: 'git_enabled',
        width: 100,
        render: value => (value ? <Tag color="blue">启用</Tag> : <Tag>未启用</Tag>)
      },
      {
        title: '最近运行',
        dataIndex: 'last_run_status',
        width: 240,
        render: (_, record) =>
          record.last_run_status ? (
            <Space direction="vertical" size={0}>
              <Tag className={['project-status', `project-status--${getPipelineStatusClass(record.last_run_status)}`].join(' ')}>
                {formatPipelineStatus(record.last_run_status)}
              </Tag>
              <span className="pipeline-job-list__meta">
                #{record.last_run_number} · {formatTime(record.last_run_time)}
              </span>
            </Space>
          ) : (
            <span className="pipeline-template__desc">尚未运行</span>
          )
      },
      {
        title: '总运行数',
        dataIndex: 'total_runs',
        width: 100,
        render: value => value || 0
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
        width: 240,
        fixed: 'right',
        render: (_, record) => (
          <Space>
            {canTrigger && (
              <Button
                size="small"
                type="primary"
                icon={<PlayCircleOutlined />}
                loading={!!triggering[record.id]}
                onClick={() => handleTrigger(record)}
              >
                运行
              </Button>
            )}
            <Button size="small" onClick={() => handleEdit(record)}>编辑</Button>
            {canWrite && (
              <Popconfirm
                title="确认删除该 Job?"
                description="删除后历史运行记录仍可在数据库查到, 但 Job 元数据丢失."
                okText="删除"
                okButtonProps={{ danger: true }}
                onConfirm={() => handleDelete(record)}
              >
                <Button size="small" danger>删除</Button>
              </Popconfirm>
            )}
          </Space>
        )
      }
    ],
    [canTrigger, canWrite, handleDelete, handleEdit, handleTrigger, triggering]
  );

  return (
    <Card
      className="pipeline-template-card"
      title="项目管理 · 独立 Pipeline Job"
      extra={
        <Space size={12}>
          <Input.Search
            placeholder="搜索名称 / 显示名"
            allowClear
            onSearch={value => {
              setKeyword(value);
              fetchList(1, { keyword: value });
            }}
            style={{ width: 260 }}
            enterButton="搜索"
          />
          <Tooltip title="刷新">
            <Button icon={<ReloadOutlined />} onClick={() => fetchList(page)} />
          </Tooltip>
          {canWrite && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              新建 Job
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
        title="新建独立 Job"
        onCancel={() => {
          setCreateOpen(false);
          createForm.resetFields();
        }}
        onOk={handleCreate}
        confirmLoading={createSubmitting}
        destroyOnClose
        width={560}
      >
        <Form form={createForm} layout="vertical" preserve={false}>
          <Form.Item
            label="Job 标识 (name)"
            name="name"
            rules={[
              { required: true, message: '请填写 Job 标识' },
              { pattern: JOB_NAME_PATTERN, message: '以字母开头, 仅含字母 / 数字 / -/_, 不超过 128 字符' }
            ]}
            extra="创建后不可修改, 用于 RBAC / 日志关联"
          >
            <Input placeholder="例如 nightly-cleanup" />
          </Form.Item>
          <Form.Item label="显示名称" name="display_name">
            <Input placeholder="缺省与 Job 标识相同" />
          </Form.Item>
          <Form.Item label="描述" name="description">
            <Input.TextArea rows={3} placeholder="此 Job 用途说明" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default PipelineJobList;
