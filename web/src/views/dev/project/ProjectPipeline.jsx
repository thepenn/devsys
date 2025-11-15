import React, { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Empty,
  Form,
  Input,
  Modal,
  Space,
  Spin,
  Table,
  Tag,
  message
} from 'antd';
import { ReloadOutlined, CodeOutlined, SettingOutlined, FileTextOutlined, PlayCircleOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import clsx from 'clsx';
import { useProjectContext } from './ProjectContext';
import {
  getPipelineConfig,
  updatePipelineConfig,
  listPipelineRuns,
  triggerPipelineRun,
  getPipelineSettings,
  updatePipelineSettings
} from 'api/project/pipeline';
import { formatPipelineStatus, getPipelineStatusClass } from 'constants/pipeline';
import { formatDuration, formatTime } from 'utils/time';
import { normalizeError } from 'utils/error';
import { emptyVariableRow, normalizeVariableRows, serializeVariableRows } from 'utils/pipelineRun';
import './ProjectPipeline.less';

const DEFAULT_SETTINGS = {
  cleanup_enabled: false,
  retention_days: 7,
  max_records: 10,
  dockerfile: '',
  disallow_parallel: false,
  cron_schedules: []
};

const ProjectPipeline = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { repo, owner, name, isAdmin } = useProjectContext();
  const repoId = repo?.id;

  const [runs, setRuns] = useState([]);
  const [totalRuns, setTotalRuns] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loadingRuns, setLoadingRuns] = useState(false);
  const [error, setError] = useState('');
  const [selectedRunId, setSelectedRunId] = useState(null);

  const [yamlModal, setYamlModal] = useState({ visible: false, content: '', loading: false, saving: false, updatedAt: '' });
  const [dockerfileModal, setDockerfileModal] = useState({ visible: false, content: '', saving: false });
  const [runModalVisible, setRunModalVisible] = useState(false);
  const [runForm, setRunForm] = useState({ branch: '', commit: '', variables: [emptyVariableRow()] });
  const [runSubmitting, setRunSubmitting] = useState(false);
  const [runFormError, setRunFormError] = useState('');

  const [settingsVisible, setSettingsVisible] = useState(false);
  const [settingsForm, setSettingsForm] = useState(DEFAULT_SETTINGS);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [cronRows, setCronRows] = useState([]);

  useEffect(() => {
    const query = new URLSearchParams(location.search);
    const highlight = Number(query.get('highlight'));
    if (!Number.isNaN(highlight) && highlight > 0) {
      setSelectedRunId(highlight);
    }
  }, [location.search]);

  useEffect(() => {
    if (!repoId) return;
    loadRuns(page, pageSize);
    loadSettings();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoId, page, pageSize]);

  useEffect(() => {
    if (!repo) return;
    setRunForm(prev => {
      if (prev.branch && prev.branch.trim()) {
        return prev;
      }
      const defaultBranch =
        (repo.branch && repo.branch.trim()) ||
        (repo.default_branch && repo.default_branch.trim()) ||
        'main';
      return { ...prev, branch: defaultBranch };
    });
  }, [repo]);

  const loadRuns = async (targetPage = page, targetPageSize = pageSize) => {
    if (!repoId) return;
    setLoadingRuns(true);
    setError('');
    try {
      const data = await listPipelineRuns(repoId, { page: targetPage, per_page: targetPageSize });
      const items = data?.items || [];
      setRuns(items);
      const total = Number(data?.total) || items.length;
      setTotalRuns(total);
      if (data?.per_page) {
        const parsed = Number(data.per_page);
        if (!Number.isNaN(parsed) && parsed > 0) {
          setPageSize(parsed);
        }
      }
      if (data?.page) {
        const parsed = Number(data.page);
        if (!Number.isNaN(parsed) && parsed > 0) {
          setPage(parsed);
        }
      }
      if (!selectedRunId && items.length) {
        setSelectedRunId(items[0].id);
      }
    } catch (err) {
      const normalized = normalizeError(err, '加载构建记录失败');
      setError(normalized.message);
      setRuns([]);
      setTotalRuns(0);
    } finally {
      setLoadingRuns(false);
    }
  };

  const loadYaml = async () => {
    if (!repoId) return;
    setYamlModal(prev => ({ ...prev, visible: true, loading: true, saving: false }));
    try {
      const data = await getPipelineConfig(repoId);
      setYamlModal({
        visible: true,
        loading: false,
        saving: false,
        content: data?.content || '',
        updatedAt: data?.updated_at || ''
      });
    } catch (err) {
      setYamlModal({ visible: false, loading: false, saving: false, content: '', updatedAt: '' });
      message.error(normalizeError(err, '加载 YAML 失败').message);
    }
  };

  const saveYaml = async () => {
    if (!repoId) return;
    setYamlModal(prev => ({ ...prev, saving: true }));
    try {
      await updatePipelineConfig(repoId, { content: yamlModal.content });
      message.success('流水线配置已更新');
      setYamlModal(prev => ({ ...prev, saving: false, visible: false }));
    } catch (err) {
      message.error(normalizeError(err, '保存 YAML 失败').message);
      setYamlModal(prev => ({ ...prev, saving: false }));
    }
  };

  const loadSettings = async () => {
    if (!repoId || !isAdmin) return;
    setSettingsLoading(true);
    try {
      const payload = await getPipelineSettings(repoId);
      const normalized = normalizeSettingsResponse(payload);
      setSettingsForm(normalized);
      setCronRows(normalized.cron_schedules || []);
    } catch (err) {
      message.error(normalizeError(err, '加载流水线设置失败').message);
    } finally {
      setSettingsLoading(false);
    }
  };

  const openDockerfileModal = async () => {
    if (!isAdmin) return;
    setDockerfileModal({ visible: true, content: settingsForm.dockerfile || '', saving: false });
    if (!settingsForm.dockerfile && !settingsLoading) {
      await loadSettings();
      setDockerfileModal({ visible: true, content: (settingsForm.dockerfile || ''), saving: false });
    }
  };

  const saveDockerfile = async () => {
    if (!repoId) return;
    setDockerfileModal(prev => ({ ...prev, saving: true }));
    try {
      const payload = buildSettingsPayload({ dockerfile: dockerfileModal.content });
      await updatePipelineSettings(repoId, payload);
      message.success('Dockerfile 已保存');
      setSettingsForm(payload);
      setDockerfileModal({ visible: false, content: dockerfileModal.content, saving: false });
    } catch (err) {
      message.error(normalizeError(err, '保存 Dockerfile 失败').message);
      setDockerfileModal(prev => ({ ...prev, saving: false }));
    }
  };

  const runPipeline = async () => {
    if (!repoId) return;
    if (!runForm.branch.trim()) {
      setRunFormError('请填写分支');
      return;
    }
    setRunFormError('');
    setRunSubmitting(true);
    try {
      const payload = {
        branch: runForm.branch.trim(),
        commit: runForm.commit.trim() || undefined,
        variables: serializeVariableRows(runForm.variables)
      };
      await triggerPipelineRun(repoId, payload);
      message.success('已触发流水线');
      setRunModalVisible(false);
      setRunForm({ branch: '', commit: '', variables: [emptyVariableRow()] });
      loadRuns();
    } catch (err) {
      message.error(normalizeError(err, '运行流水线失败').message);
    } finally {
      setRunSubmitting(false);
    }
  };

  const buildSettingsPayload = overrides => ({
    cleanup_enabled: settingsForm.cleanup_enabled,
    retention_days: settingsForm.retention_days,
    max_records: settingsForm.max_records,
    dockerfile: settingsForm.dockerfile,
    disallow_parallel: settingsForm.disallow_parallel,
    cron_schedules: cleanCronRows(),
    ...overrides
  });

  const cleanCronRows = () => {
    const seen = new Set();
    const result = [];
    cronRows.forEach(row => {
      const trimmed = (row || '').trim();
      if (!trimmed || seen.has(trimmed)) return;
      seen.add(trimmed);
      result.push(trimmed);
    });
    return result;
  };

  const saveSettings = async () => {
    if (!repoId) return;
    setSettingsSaving(true);
    try {
      const payload = buildSettingsPayload();
      await updatePipelineSettings(repoId, payload);
      message.success('设置已保存');
      setSettingsForm(payload);
      setSettingsVisible(false);
    } catch (err) {
      message.error(normalizeError(err, '保存设置失败').message);
    } finally {
      setSettingsSaving(false);
    }
  };

  const settingsEnabled = Boolean(repoId && isAdmin);

  const runColumns = useMemo(() => {
    const formatter = value => (value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '—');
    return [
      {
        title: '状态',
        dataIndex: 'status',
        render: status => (
          <Tag className={clsx('pipeline-status', `pipeline-status--${getPipelineStatusClass(status)}`)}>
            {formatPipelineStatus(status)}
          </Tag>
        )
      },
      {
        title: '运行编号',
        dataIndex: 'number',
        render: (num, record) => `#${num || record.id}`
      },
      {
        title: '分支',
        dataIndex: 'branch',
        ellipsis: true
      },
      {
        title: 'Commit',
        dataIndex: 'commit',
        render: commit => commit ? commit.slice(0, 8) : '—'
      },
      {
        title: '发起人',
        dataIndex: 'author',
        render: author => author || '系统'
      },
      {
        title: '耗时',
        render: (_, record) => formatDuration(record.created, record.finished)
      },
      {
        title: '触发时间',
        dataIndex: 'created',
        render: value => formatter(value)
      },
      {
        title: '备注',
        dataIndex: 'message',
        ellipsis: true,
        render: value => value || '—'
      }
    ];
  }, []);

  const selectedRun = useMemo(() => runs.find(item => item.id === selectedRunId), [runs, selectedRunId]);

  const paginationConfig = useMemo(() => ({
    current: page,
    pageSize,
    total: totalRuns,
    showSizeChanger: true,
    onChange: (nextPage, nextSize) => {
      setPage(nextPage);
      setPageSize(nextSize);
    }
  }), [page, pageSize, totalRuns]);

  const normalizeSettingsResponse = payload => {
    if (!payload) return { ...DEFAULT_SETTINGS };
    const schedules = Array.isArray(payload.cron_schedules)
      ? payload.cron_schedules.filter(item => typeof item === 'string' && item.trim()).map(item => item.trim())
      : [];
    return {
      cleanup_enabled: Boolean(payload.cleanup_enabled),
      retention_days: Number.isFinite(payload.retention_days) ? payload.retention_days : DEFAULT_SETTINGS.retention_days,
      max_records: Number.isFinite(payload.max_records) && payload.max_records > 0 ? payload.max_records : DEFAULT_SETTINGS.max_records,
      dockerfile: payload.dockerfile || '',
      disallow_parallel: Boolean(payload.disallow_parallel),
      cron_schedules: schedules
    };
  };

  return (
    <div className="pipeline-page">
      {error && <Alert type="error" message={error} showIcon className="pipeline-alert" />}

      <Card className="pipeline-history" title="构建记录" extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => loadRuns(page, pageSize)} loading={loadingRuns}>
            刷新
          </Button>
          <Button type="primary" icon={<PlayCircleOutlined />} onClick={() => setRunModalVisible(true)}>
            运行流水线
          </Button>
          <Button icon={<CodeOutlined />} onClick={loadYaml}>
            编辑 YAML
          </Button>
          <Button icon={<FileTextOutlined />} disabled={!isAdmin} onClick={openDockerfileModal}>
            编辑 Dockerfile
          </Button>
          <Button icon={<SettingOutlined />} disabled={!settingsEnabled} onClick={() => setSettingsVisible(true)}>
            流水线设置
          </Button>
        </Space>
      }>
        {runs.length ? (
          <Table
            rowKey="id"
            dataSource={runs}
            columns={runColumns}
            rowClassName={record =>
              clsx('pipeline-row', { 'pipeline-row--active': record.id === selectedRunId })
            }
            loading={loadingRuns}
            pagination={paginationConfig}
            onRow={record => ({
              onClick: () => navigate(`/dev/projects/${owner}/${name}/pipeline/${record.id}`)
            })}
          />
        ) : loadingRuns ? (
          <div className="pipeline-placeholder">
            <Spin />
          </div>
        ) : (
          <Empty description="暂无构建记录" />
        )}
      </Card>

      {selectedRun && (
        <Card className="pipeline-selected" title={`构建 #${selectedRun.number || selectedRun.id}`}>
          <Space size="large" wrap>
            <div>
              <span className="pipeline-label">状态</span>
              <Tag className={clsx('pipeline-status', `pipeline-status--${getPipelineStatusClass(selectedRun.status)}`)}>
                {formatPipelineStatus(selectedRun.status)}
              </Tag>
            </div>
            <div>
              <span className="pipeline-label">分支</span>
              <span>{selectedRun.branch || '—'}</span>
            </div>
            <div>
              <span className="pipeline-label">耗时</span>
              <span>{formatDuration(selectedRun.created, selectedRun.finished)}</span>
            </div>
            <div>
              <span className="pipeline-label">触发时间</span>
              <span>{formatTime(selectedRun.created) || '—'}</span>
            </div>
          </Space>
        </Card>
      )}

      <Modal
        open={yamlModal.visible}
        title={`编辑流水线 YAML ${yamlModal.updatedAt ? `（更新于 ${formatTime(yamlModal.updatedAt)}）` : ''}`}
        onCancel={() => setYamlModal(prev => ({ ...prev, visible: false }))}
        onOk={saveYaml}
        confirmLoading={yamlModal.saving}
        width={720}
      >
        {yamlModal.loading ? (
          <Spin />
        ) : (
          <Input.TextArea
            rows={18}
            value={yamlModal.content}
            onChange={e => setYamlModal(prev => ({ ...prev, content: e.target.value }))}
          />
        )}
      </Modal>

      <Modal
        open={dockerfileModal.visible}
        title="编辑 Dockerfile"
        onCancel={() => setDockerfileModal(prev => ({ ...prev, visible: false }))}
        onOk={saveDockerfile}
        confirmLoading={dockerfileModal.saving}
        width={720}
      >
        <Input.TextArea
          rows={18}
          value={dockerfileModal.content}
          onChange={e => setDockerfileModal(prev => ({ ...prev, content: e.target.value }))}
        />
      </Modal>

      <Modal
        open={runModalVisible}
        title="运行流水线"
        onCancel={() => setRunModalVisible(false)}
        onOk={runPipeline}
        confirmLoading={runSubmitting}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <label className="modal-field">
            <span>构建分支 *</span>
            <Input
              value={runForm.branch}
              onChange={e => setRunForm(prev => ({ ...prev, branch: e.target.value }))}
              placeholder="例如 main"
            />
          </label>
          <label className="modal-field">
            <span>Commit ID (可选)</span>
            <Input
              value={runForm.commit}
              onChange={e => setRunForm(prev => ({ ...prev, commit: e.target.value }))}
              placeholder="指定 commit 时优先使用"
            />
          </label>
          <div className="modal-field">
            <span>运行变量（可选）</span>
            <p className="modal-hint">以环境变量形式传入流水线，仅填写需要覆盖的键。</p>
            {runForm.variables.map((variable, idx) => (
              <Space key={`var-${idx}`} className="run-variable-row" align="baseline">
                <Input
                  value={variable.key}
                  placeholder="变量名"
                  onChange={e => {
                    const rows = normalizeVariableRows(runForm.variables);
                    rows[idx].key = e.target.value;
                    setRunForm(prev => ({ ...prev, variables: rows }));
                  }}
                />
                <Input
                  value={variable.value}
                  placeholder="变量值"
                  onChange={e => {
                    const rows = normalizeVariableRows(runForm.variables);
                    rows[idx].value = e.target.value;
                    setRunForm(prev => ({ ...prev, variables: rows }));
                  }}
                />
                <Button type="link" onClick={() => {
                  const rows = normalizeVariableRows(runForm.variables).filter((_, index) => index !== idx);
                  setRunForm(prev => ({ ...prev, variables: rows.length ? rows : [emptyVariableRow()] }));
                }}>
                  删除
                </Button>
              </Space>
            ))}
            <Button type="dashed" onClick={() => setRunForm(prev => ({ ...prev, variables: [...prev.variables, emptyVariableRow()] }))}>
              + 添加变量
            </Button>
          </div>
          {runFormError && <Alert type="error" message={runFormError} showIcon />}
        </Space>
      </Modal>

      <Modal
        open={settingsVisible}
        title="流水线设置"
        onCancel={() => setSettingsVisible(false)}
        onOk={saveSettings}
        confirmLoading={settingsSaving}
        width={640}
      >
        {settingsLoading ? (
          <Spin />
        ) : (
          <Form layout="vertical">
            <Form.Item>
              <Checkbox
                checked={settingsForm.cleanup_enabled}
                onChange={e => setSettingsForm(prev => ({ ...prev, cleanup_enabled: e.target.checked }))}
              >
                删除过期构建记录
              </Checkbox>
            </Form.Item>
            <Form.Item label="构建记录保留期限 (天)">
              <Input
                type="number"
                min={0}
                value={settingsForm.retention_days}
                onChange={e => setSettingsForm(prev => ({ ...prev, retention_days: Number(e.target.value) }))}
              />
            </Form.Item>
            <Form.Item label="构建记录最大数量">
              <Input
                type="number"
                min={1}
                value={settingsForm.max_records}
                onChange={e => setSettingsForm(prev => ({ ...prev, max_records: Number(e.target.value) }))}
              />
            </Form.Item>
            <Form.Item>
              <Checkbox
                checked={settingsForm.disallow_parallel}
                onChange={e => setSettingsForm(prev => ({ ...prev, disallow_parallel: e.target.checked }))}
              >
                不允许并发构建
              </Checkbox>
            </Form.Item>
            <Form.Item label="预设 Dockerfile">
              <Input.TextArea
                rows={6}
                value={settingsForm.dockerfile}
                onChange={e => setSettingsForm(prev => ({ ...prev, dockerfile: e.target.value }))}
              />
            </Form.Item>
            <Form.Item label="构建触发器 (Cron)">
              <Space direction="vertical" style={{ width: '100%' }}>
                {!cronRows.length && <span className="settings-empty">暂无定时任务，点击下方添加。</span>}
                {cronRows.map((cron, idx) => (
                  <Space key={`cron-${idx}`} align="baseline" className="cron-row">
                    <Input
                      value={cron}
                      onChange={e => {
                        const rows = [...cronRows];
                        rows[idx] = e.target.value;
                        setCronRows(rows);
                      }}
                      placeholder="例如：0 0 * * *"
                    />
                    <Button type="link" onClick={() => setCronRows(rows => rows.filter((_, index) => index !== idx))}>
                      移除
                    </Button>
                  </Space>
                ))}
                <Button type="dashed" onClick={() => setCronRows(rows => [...rows, ''])}>
                  添加 Cron 表达式
                </Button>
              </Space>
            </Form.Item>
          </Form>
        )}
      </Modal>
    </div>
  );
};

export default ProjectPipeline;
