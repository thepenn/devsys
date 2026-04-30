import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Form, Input, Modal, Select, Space, Spin, Switch, Table, Tabs, Tag, Tooltip, message } from 'antd';
import { ArrowLeftOutlined, PlayCircleOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { hasAnyLabel } from '../../utils/permission';
import {
  cancelJobRun,
  getJob,
  listJobRuns,
  triggerJob,
  updateJob
} from '../../api/system/pipelineJobs';
import { listCertificates } from '../../api/system/certificates';
import CodeEditor from '../../components/CodeEditor';
import BuildStepInserter from '../../components/BuildStepInserter';
import { formatDuration, formatTime } from '../../utils/time';
import { formatPipelineStatus, getPipelineStatusClass } from '../../constants/pipeline';
import '../pipelineTemplates/pipeline-templates.less';
import './pipeline-jobs.less';

const emptyVar = () => ({ key: '', value: '' });

const variablesToRows = vars => {
  if (!vars || typeof vars !== 'object') return [emptyVar()];
  const entries = Object.entries(vars);
  if (entries.length === 0) return [emptyVar()];
  return entries.map(([k, v]) => ({ key: k, value: v ?? '' }));
};

const rowsToVariables = rows =>
  (rows || [])
    .map(row => ({ key: (row.key || '').trim(), value: row.value ?? '' }))
    .filter(row => row.key !== '')
    .reduce((acc, row) => {
      acc[row.key] = row.value;
      return acc;
    }, {});

const PipelineJobEditor = () => {
  const { id } = useParams();
  const numericId = Number(id);
  const navigate = useNavigate();
  const { user } = useAuth();
  const canWrite = useMemo(() => hasAnyLabel(user, ['pipeline_job:write']), [user]);
  const canTrigger = useMemo(() => hasAnyLabel(user, ['pipeline_job:trigger']), [user]);

  const [loading, setLoading] = useState(false);
  const [job, setJob] = useState(null);
  const [meta, setMeta] = useState({ display_name: '', description: '' });
  const [content, setContent] = useState('');
  const [git, setGit] = useState({ enabled: false, clone_url: '', branch: '', credential_id: null });
  const [varRows, setVarRows] = useState([emptyVar()]);
  const [cronRows, setCronRows] = useState([]);

  const [activeTab, setActiveTab] = useState('yaml');
  const [saving, setSaving] = useState(false);
  const [triggerOpen, setTriggerOpen] = useState(false);
  const [triggerForm, setTriggerForm] = useState({ branch: '', variables: [emptyVar()] });
  const [triggerSubmitting, setTriggerSubmitting] = useState(false);

  const [certs, setCerts] = useState([]);
  const [runs, setRuns] = useState([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsPage, setRunsPage] = useState(1);
  const [runsPerPage] = useState(10);
  const [runsTotal, setRunsTotal] = useState(0);

  const fetchJob = useCallback(async () => {
    if (!Number.isFinite(numericId) || numericId <= 0) {
      message.error('Job id 非法');
      return;
    }
    setLoading(true);
    try {
      const data = await getJob(numericId);
      setJob(data);
      setMeta({ display_name: data?.display_name || '', description: data?.description || '' });
      setContent(data?.content || '');
      setGit({
        enabled: Boolean(data?.git_enabled),
        clone_url: data?.git_clone_url || '',
        branch: data?.git_branch || '',
        credential_id: data?.git_credential_id ?? null
      });
      setVarRows(variablesToRows(data?.variables));
      setCronRows(Array.isArray(data?.cron_schedules) ? data.cron_schedules.filter(Boolean) : []);
    } catch (_err) {
      // 错误提示由 request.js 拦截器统一弹出, 避免与本地 message.error
      // 因 toast key 替换而互相覆盖, 让用户看不到真实后端错误.
    } finally {
      setLoading(false);
    }
  }, [numericId]);

  useEffect(() => {
    fetchJob();
  }, [fetchJob]);

  // Git 凭证下拉数据 (只在启用 git 时拉, 减少无谓请求)
  useEffect(() => {
    if (!git.enabled) return;
    listCertificates({ per_page: 200 })
      .then(data => setCerts(Array.isArray(data?.items) ? data.items : Array.isArray(data) ? data : []))
      .catch(err => {
        // 凭证拉不到不影响其它功能
        console.warn('failed to load certificates', err);
      });
  }, [git.enabled]);

  const fetchRuns = useCallback(
    async (targetPage = 1) => {
      if (!Number.isFinite(numericId) || numericId <= 0) return;
      setRunsLoading(true);
      try {
        const data = await listJobRuns(numericId, { page: targetPage, per_page: runsPerPage });
        setRuns(Array.isArray(data?.items) ? data.items : []);
        setRunsTotal(Number(data?.total || 0));
        setRunsPage(Number(data?.page || targetPage));
      } catch (_err) {
        // 由 request.js 拦截器统一弹错.
      } finally {
        setRunsLoading(false);
      }
    },
    [numericId, runsPerPage]
  );

  // 当用户切到运行历史 Tab 时再拉, 节省加载.
  useEffect(() => {
    if (activeTab === 'runs') fetchRuns(1);
  }, [activeTab, fetchRuns]);

  const handleSave = async () => {
    if (!canWrite || !job) return;
    setSaving(true);
    try {
      const payload = {
        display_name: meta.display_name,
        description: meta.description,
        content,
        git_enabled: git.enabled,
        git_clone_url: git.clone_url,
        git_branch: git.branch,
        git_credential_id: git.credential_id ?? null,
        clear_credential: git.credential_id == null,
        variables: rowsToVariables(varRows),
        cron_schedules: cronRows.map(s => (s || '').trim()).filter(Boolean)
      };
      const updated = await updateJob(job.id, payload);
      setJob(updated);
      message.success('已保存');
    } catch (_err) {
      // 由 request.js 拦截器统一弹错.
    } finally {
      setSaving(false);
    }
  };

  const submitTrigger = async () => {
    if (!job) return;
    setTriggerSubmitting(true);
    try {
      const run = await triggerJob(job.id, {
        branch: triggerForm.branch.trim() || undefined,
        variables: rowsToVariables(triggerForm.variables)
      });
      message.success('已触发');
      setTriggerOpen(false);
      navigate(`/ops/pipeline-jobs/${job.id}/runs/${run.id}`);
    } catch (_err) {
      // 由 request.js 拦截器统一弹错.
    } finally {
      setTriggerSubmitting(false);
    }
  };

  const cancelRun = async runId => {
    if (!job) return;
    try {
      await cancelJobRun(job.id, runId);
      message.success('已发送取消');
      fetchRuns(runsPage);
    } catch (_err) {
      // 由 request.js 拦截器统一弹错.
    }
  };

  const certOptions = useMemo(
    () =>
      (certs || [])
        .filter(c => !c.type || String(c.type).toLowerCase() === 'git')
        .map(c => ({ value: c.id, label: `${c.name}${c.type ? ` (${c.type})` : ''}` })),
    [certs]
  );

  const runColumns = [
    {
      title: '#',
      dataIndex: 'number',
      width: 100,
      render: (value, record) => (
        <button type="button" className="pipeline-template__link" onClick={() => navigate(`/ops/pipeline-jobs/${job.id}/runs/${record.id}`)}>
          <strong>#{value}</strong>
        </button>
      )
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 140,
      render: value => (
        <Tag className={['project-status', `project-status--${getPipelineStatusClass(value)}`].join(' ')}>
          {formatPipelineStatus(value)}
        </Tag>
      )
    },
    { title: '触发人', dataIndex: 'author', width: 160, render: v => v || '—' },
    { title: '分支', dataIndex: 'branch', width: 160, render: v => v || '—' },
    { title: '开始时间', dataIndex: 'started', width: 200, render: (_, r) => formatTime((r.started || r.created) * 1000) },
    {
      title: '耗时',
      dataIndex: 'duration',
      width: 140,
      render: (_, r) => formatDuration((r.started || r.created) * 1000, (r.finished || 0) * 1000)
    },
    {
      title: '操作',
      dataIndex: 'actions',
      width: 160,
      render: (_, r) => (
        <Space>
          <Button size="small" onClick={() => navigate(`/ops/pipeline-jobs/${job.id}/runs/${r.id}`)}>
            查看
          </Button>
          {canTrigger && (
            <Button size="small" danger onClick={() => cancelRun(r.id)}>
              取消
            </Button>
          )}
        </Space>
      )
    }
  ];

  if (loading && !job) {
    return (
      <Card>
        <Spin />
      </Card>
    );
  }

  return (
    <Card
      className="pipeline-template-editor"
      title={
        <Space>
          <Button type="link" icon={<ArrowLeftOutlined />} onClick={() => navigate('/ops/pipeline-jobs')}>
            返回列表
          </Button>
          <strong>{job ? job.display_name || job.name : 'Job'}</strong>
          {job?.git_enabled && <Tag color="blue">Git 启用</Tag>}
        </Space>
      }
      extra={
        <Space>
          <Tooltip title="重新加载">
            <Button icon={<ReloadOutlined />} onClick={fetchJob} />
          </Tooltip>
          {canWrite && (
            <Button icon={<SaveOutlined />} onClick={handleSave} loading={saving}>
              保存
            </Button>
          )}
          {canTrigger && (
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              onClick={() => {
                setTriggerForm({ branch: git.branch || '', variables: variablesToRows(rowsToVariables(varRows)) });
                setTriggerOpen(true);
              }}
              disabled={!job}
            >
              立即运行
            </Button>
          )}
        </Space>
      }
    >
      {!job ? (
        <Alert type="error" message="Job 不存在或已被删除" showIcon />
      ) : (
        <>
          <div className="pipeline-template-meta">
            <Form layout="vertical">
              <div className="pipeline-template-meta__row">
                <Form.Item label="Job 标识 (name)">
                  <Input value={job.name} disabled />
                </Form.Item>
                <Form.Item label="显示名称">
                  <Input
                    value={meta.display_name}
                    disabled={!canWrite}
                    onChange={e => setMeta(prev => ({ ...prev, display_name: e.target.value }))}
                  />
                </Form.Item>
              </div>
              <Form.Item label="描述">
                <Input.TextArea
                  rows={2}
                  value={meta.description}
                  disabled={!canWrite}
                  onChange={e => setMeta(prev => ({ ...prev, description: e.target.value }))}
                />
              </Form.Item>
            </Form>
          </div>

          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'yaml',
                label: 'YAML',
                children: (
                  <>
                    <Space style={{ marginBottom: 8 }} wrap>
                      <BuildStepInserter value={content} onChange={setContent} disabled={!canWrite} />
                    </Space>
                    <CodeEditor
                      language="yaml"
                      value={content}
                      onChange={setContent}
                      readOnly={!canWrite}
                      placeholder={'name: example\nworkspace: /workspace\nsteps:\n  hello:\n    image: alpine\n    commands:\n      - echo "hello from job"'}
                    />
                  </>
                )
              },
              {
                key: 'git',
                label: 'Git 配置',
                children: (
                  <div className="pipeline-job-section">
                    <Space align="center">
                      <span>启用 Git</span>
                      <Switch checked={git.enabled} disabled={!canWrite} onChange={v => setGit(prev => ({ ...prev, enabled: v }))} />
                      <span className="pipeline-template-vars__hint">
                        启用后会把 clone URL / 凭证作为环境变量 (JOB_GIT_CLONE_URL / JOB_GIT_USERNAME / JOB_GIT_PASSWORD)
                        提供给 commands, commands 内自行 git clone.
                      </span>
                    </Space>
                    {git.enabled && (
                      <Form layout="vertical" style={{ marginTop: 16 }}>
                        <Form.Item label="Clone URL">
                          <Input
                            value={git.clone_url}
                            disabled={!canWrite}
                            placeholder="https://example.com/group/repo.git"
                            onChange={e => setGit(prev => ({ ...prev, clone_url: e.target.value }))}
                          />
                        </Form.Item>
                        <Form.Item label="默认分支">
                          <Input
                            value={git.branch}
                            disabled={!canWrite}
                            placeholder="main"
                            onChange={e => setGit(prev => ({ ...prev, branch: e.target.value }))}
                          />
                        </Form.Item>
                        <Form.Item label="凭证 (Certificate)">
                          <Select
                            allowClear
                            placeholder="选择 git 类型凭证"
                            value={git.credential_id ?? undefined}
                            options={certOptions}
                            disabled={!canWrite}
                            onChange={v => setGit(prev => ({ ...prev, credential_id: v ?? null }))}
                          />
                        </Form.Item>
                      </Form>
                    )}
                  </div>
                )
              },
              {
                key: 'vars',
                label: '默认变量',
                children: (
                  <div className="pipeline-template-vars">
                    <p className="pipeline-template-vars__hint">
                      {/* eslint-disable-next-line no-template-curly-in-string */}
                      用于 YAML 中的 <code>{'${VAR}'}</code> 占位符替换. 触发时可以在「立即运行」弹窗中再次覆盖.
                    </p>
                    {varRows.map((row, idx) => (
                      <div key={`var-${idx}`} className="pipeline-template-vars__row">
                        <Input
                          value={row.key}
                          placeholder="变量名"
                          disabled={!canWrite}
                          onChange={e => setVarRows(prev => prev.map((r, i) => (i === idx ? { ...r, key: e.target.value } : r)))}
                        />
                        <Input
                          value={row.value}
                          placeholder="变量值"
                          disabled={!canWrite}
                          onChange={e => setVarRows(prev => prev.map((r, i) => (i === idx ? { ...r, value: e.target.value } : r)))}
                        />
                        <Button
                          type="link"
                          disabled={!canWrite}
                          onClick={() => setVarRows(prev => {
                            const next = prev.filter((_, i) => i !== idx);
                            return next.length ? next : [emptyVar()];
                          })}
                        >
                          删除
                        </Button>
                      </div>
                    ))}
                    <Button type="dashed" disabled={!canWrite} onClick={() => setVarRows(prev => [...prev, emptyVar()])}>
                      + 添加变量
                    </Button>
                  </div>
                )
              },
              {
                key: 'cron',
                label: `调度 (Cron)${cronRows.length ? ` · ${cronRows.length}` : ''}`,
                children: (
                  <div className="pipeline-job-section">
                    <p className="pipeline-template-vars__hint">
                      标准 5 字段格式 <code>分 时 日 月 周</code>。例如 <code>0 2 * * *</code> 表示每天凌晨 2:00；<code>*/15 * * * *</code> 每 15 分钟；<code>0 9 * * 1-5</code> 工作日 9:00。多条表达式可并存，全部清空即关闭调度。保存后立即生效，无需重启。
                    </p>
                    <Space direction="vertical" style={{ width: '100%' }}>
                      {cronRows.length === 0 && (
                        <span className="pipeline-job-list__meta">暂无调度（手动触发模式）</span>
                      )}
                      {cronRows.map((expr, idx) => (
                        <Space key={`cron-${idx}`} align="baseline" style={{ width: '100%' }}>
                          <Input
                            value={expr}
                            disabled={!canWrite}
                            placeholder="例如 0 2 * * *"
                            style={{ width: 360 }}
                            onChange={e => setCronRows(prev => prev.map((v, i) => (i === idx ? e.target.value : v)))}
                          />
                          <Button
                            type="link"
                            disabled={!canWrite}
                            onClick={() => setCronRows(prev => prev.filter((_, i) => i !== idx))}
                          >
                            移除
                          </Button>
                        </Space>
                      ))}
                      <Button type="dashed" disabled={!canWrite} onClick={() => setCronRows(prev => [...prev, ''])}>
                        + 添加 Cron 表达式
                      </Button>
                    </Space>
                  </div>
                )
              },
              {
                key: 'runs',
                label: '运行历史',
                children: (
                  <div>
                    <Space style={{ marginBottom: 12 }}>
                      <Button icon={<ReloadOutlined />} onClick={() => fetchRuns(runsPage)} loading={runsLoading}>
                        刷新
                      </Button>
                      <span className="pipeline-job-list__meta">总数 {runsTotal}</span>
                    </Space>
                    <Table
                      rowKey="id"
                      size="small"
                      columns={runColumns}
                      loading={runsLoading}
                      dataSource={runs}
                      pagination={{
                        current: runsPage,
                        pageSize: runsPerPage,
                        total: runsTotal,
                        showSizeChanger: false,
                        onChange: target => fetchRuns(target)
                      }}
                    />
                  </div>
                )
              }
            ]}
          />
        </>
      )}

      <Modal
        open={triggerOpen}
        title={`立即运行 · ${job?.display_name || job?.name || ''}`}
        onCancel={() => setTriggerOpen(false)}
        onOk={submitTrigger}
        confirmLoading={triggerSubmitting}
        destroyOnClose
        width={560}
      >
        <Form layout="vertical">
          <Form.Item label="分支 (覆盖默认)">
            <Input
              value={triggerForm.branch}
              placeholder={git.branch || 'main'}
              onChange={e => setTriggerForm(prev => ({ ...prev, branch: e.target.value }))}
            />
          </Form.Item>
          <Form.Item label="覆盖变量">
            {triggerForm.variables.map((row, idx) => (
              <div key={`tv-${idx}`} className="pipeline-template-vars__row">
                <Input
                  value={row.key}
                  placeholder="变量名"
                  onChange={e => setTriggerForm(prev => {
                    const next = [...prev.variables];
                    next[idx] = { ...next[idx], key: e.target.value };
                    return { ...prev, variables: next };
                  })}
                />
                <Input
                  value={row.value}
                  placeholder="变量值"
                  onChange={e => setTriggerForm(prev => {
                    const next = [...prev.variables];
                    next[idx] = { ...next[idx], value: e.target.value };
                    return { ...prev, variables: next };
                  })}
                />
                <Button
                  type="link"
                  onClick={() => setTriggerForm(prev => {
                    const next = prev.variables.filter((_, i) => i !== idx);
                    return { ...prev, variables: next.length ? next : [emptyVar()] };
                  })}
                >
                  删除
                </Button>
              </div>
            ))}
            <Button
              type="dashed"
              onClick={() => setTriggerForm(prev => ({ ...prev, variables: [...prev.variables, emptyVar()] }))}
            >
              + 添加变量
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default PipelineJobEditor;
