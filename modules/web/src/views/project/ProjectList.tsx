import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Checkbox, Drawer, Input, Modal, Segmented, Space, Spin, Table, Tabs, Tag, Tooltip, message } from 'antd';
import { ReloadOutlined, SyncOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { listRepositories, syncRepositories, syncRepository } from '../../api/project/repos';
import { getPipelineConfig, updatePipelineConfig, getPipelineSettings, updatePipelineSettings, triggerPipelineRun } from '../../api/project/pipeline';
import { formatTime } from '../../utils/time';
import { emptyVariableRow, normalizeVariableRows, serializeVariableRows } from '../../utils/pipelineRun';
import { itemsFromListResponse } from '../../utils/listResponse';
import TablePagination from '../../components/TablePagination';
import CodeEditor from '../../components/CodeEditor';
import PipelineSourceEditor from '../../components/PipelineSourceEditor';
import './project.less';

const SYNC_OPTIONS = [
  { label: '已同步', value: 'synced' },
  { label: '未同步', value: 'unsynced' }
];

const DEFAULT_PIPELINE_SETTINGS = {
  cleanup_enabled: false,
  retention_days: 7,
  max_records: 10,
  dockerfile: '',
  disallow_parallel: false,
  cron_schedules: []
};

const ProjectList = () => {
  const navigate = useNavigate();
  const [repos, setRepos] = useState([]);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [syncedFilter, setSyncedFilter] = useState('synced');
  const [syncingAll, setSyncingAll] = useState(false);
  const [repoSyncing, setRepoSyncing] = useState({});
  const [configDrawerVisible, setConfigDrawerVisible] = useState(false);
  const [configDrawerTab, setConfigDrawerTab] = useState('yaml');
  const [configRepo, setConfigRepo] = useState(null);
  const [pipelineSource, setPipelineSource] = useState({
    source: 'inline',
    content: '',
    template_id: null,
    template_variables: {},
    compose_steps: []
  });
  const [yamlSaving, setYamlSaving] = useState(false);
  const [configLoading, setConfigLoading] = useState(false);
  const [settingsForm, setSettingsForm] = useState(DEFAULT_PIPELINE_SETTINGS);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [cronRows, setCronRows] = useState([]);
  const [runModalVisible, setRunModalVisible] = useState(false);
  const [runRepo, setRunRepo] = useState(null);
  const [runForm, setRunForm] = useState({ branch: '', commit: '', variables: [emptyVariableRow()] });
  const [runSubmitting, setRunSubmitting] = useState(false);
  const [runFormError, setRunFormError] = useState('');

  const fetchRepos = useCallback(
    async (targetPage = 1, overrides: Record<string, unknown> = {}) => {
      const nextSearch = String(overrides.search !== undefined ? overrides.search : search);
      const nextFilter = String(overrides.synced !== undefined ? overrides.synced : syncedFilter);
      const params: Record<string, string | number> = {
        page: targetPage,
        per_page: Number(overrides.perPage) || perPage,
        synced: nextFilter === 'synced' ? 'true' : 'false'
      };
      if (nextSearch.trim()) {
        params.search = nextSearch.trim();
      }
      setLoading(true);
      try {
        const data = (await listRepositories(params)) as {
          items?: unknown[];
          total?: number;
          page?: number;
          per_page?: number;
        };
        setRepos(itemsFromListResponse(data));
        setTotal(data?.total || 0);
        setPage(data?.page || targetPage);
        if (data?.per_page) {
          setPerPage(data.per_page);
        } else if (overrides.perPage != null) {
          setPerPage(Number(overrides.perPage));
        }
      } catch (err) {
        message.error(err.message || '加载项目失败');
      } finally {
        setLoading(false);
      }
    },
    [perPage, search, syncedFilter]
  );

  useEffect(() => {
    fetchRepos(1);
  }, [fetchRepos]);

  const handleSearch = value => {
    setSearch(value);
    fetchRepos(1, { search: value });
  };

  const handleSyncToggle = value => {
    if (value === syncedFilter) return;
    setSyncedFilter(value);
    fetchRepos(1, { synced: value });
  };

  const handleSyncAll = async () => {
    setSyncingAll(true);
    try {
      await syncRepositories();
      message.success('已触发仓库同步');
      fetchRepos(page);
    } catch (err) {
      message.error(err.message || '同步失败');
    } finally {
      setSyncingAll(false);
    }
  };

  const handleSyncRepo = useCallback(
    async repo => {
      if (!repo?.forge_remote_id) return;
      setRepoSyncing(prev => ({ ...prev, [repo.forge_remote_id]: true }));
      try {
        await syncRepository(repo.forge_remote_id);
        message.success(`${repo.full_name || repo.name} 同步任务已触发`);
        fetchRepos(page);
      } catch (err) {
        message.error(err.message || '同步失败');
      } finally {
        setRepoSyncing(prev => ({ ...prev, [repo.forge_remote_id]: false }));
      }
    },
    [fetchRepos, page]
  );

  const handleViewPipeline = useCallback(
    repo => {
      if (!repo?.id) return;
      navigate(`/ops/projects/pipeline?repo=${repo.id}&name=${encodeURIComponent(repo.full_name || repo.name)}`);
    },
    [navigate]
  );

  const handleConfigPipeline = useCallback(
    repo => {
      if (!repo?.id) {
        message.warning('缺少项目标识，无法加载配置');
        return;
      }
      setConfigRepo(repo);
      setConfigDrawerVisible(true);
      setConfigDrawerTab('yaml');
      setConfigLoading(true);
      Promise.all([getPipelineConfig(repo.id), getPipelineSettings(repo.id)])
        .then(([config, settings]) => {
          const c = config as Record<string, any>;
          const src = c?.source === 'template' || c?.source === 'compose' ? c.source : 'inline';
          setPipelineSource({
            source: src,
            content: c?.content || '',
            template_id: c?.template_id ?? null,
            template_variables: c?.template_variables || {},
            compose_steps: Array.isArray(c?.compose_steps) ? c.compose_steps : []
          });
          const normalized = normalizeSettings(settings);
          setSettingsForm(normalized);
          setCronRows(normalized.cron_schedules || []);
        })
        .catch(err => {
          message.error(err?.message || '加载流水线配置失败');
          setConfigDrawerVisible(false);
        })
        .finally(() => {
          setConfigLoading(false);
        });
    },
    []
  );

  const closeConfigDrawer = useCallback(() => {
    setConfigDrawerVisible(false);
    setConfigRepo(null);
    setPipelineSource({ source: 'inline', content: '', template_id: null, template_variables: {}, compose_steps: [] });
    setSettingsForm(DEFAULT_PIPELINE_SETTINGS);
    setCronRows([]);
    setConfigDrawerTab('yaml');
  }, []);

  const normalizeSettings = payload => {
    if (!payload) return { ...DEFAULT_PIPELINE_SETTINGS };
    const schedules = Array.isArray(payload.cron_schedules)
      ? payload.cron_schedules.filter(item => typeof item === 'string' && item.trim()).map(item => item.trim())
      : [];
    return {
      cleanup_enabled: Boolean(payload.cleanup_enabled),
      retention_days: Number.isFinite(payload.retention_days) ? payload.retention_days : DEFAULT_PIPELINE_SETTINGS.retention_days,
      max_records: Number.isFinite(payload.max_records) && payload.max_records > 0 ? payload.max_records : DEFAULT_PIPELINE_SETTINGS.max_records,
      dockerfile: payload.dockerfile || '',
      disallow_parallel: Boolean(payload.disallow_parallel),
      cron_schedules: schedules
    };
  };

  const buildSettingsPayload = (overrides: Record<string, unknown> = {}) => ({
    cleanup_enabled: settingsForm.cleanup_enabled,
    retention_days: settingsForm.retention_days,
    max_records: settingsForm.max_records,
    disallow_parallel: settingsForm.disallow_parallel,
    dockerfile: settingsForm.dockerfile,
    cron_schedules: (Array.isArray(overrides.cron_schedules) ? (overrides.cron_schedules as string[]) : cronRows)
      .map(item => String(item).trim())
      .filter(Boolean),
    ...overrides
  });

  const saveYaml = async () => {
    if (!configRepo?.id) return;
    if (pipelineSource.source === 'template' && !pipelineSource.template_id) {
      message.error('请先选择通用 pipeline 模板');
      return;
    }
    if (pipelineSource.source === 'compose') {
      const refs = (pipelineSource.compose_steps || []).filter(s => s && s.step_template_id);
      if (refs.length === 0) {
        message.error('请至少添加一个步骤模板');
        return;
      }
    }
    setYamlSaving(true);
    try {
      let payload;
      if (pipelineSource.source === 'template') {
        payload = {
          source: 'template',
          template_id: pipelineSource.template_id,
          template_variables: pipelineSource.template_variables || {}
        };
      } else if (pipelineSource.source === 'compose') {
        payload = {
          source: 'compose',
          compose_steps: (pipelineSource.compose_steps || [])
            .filter(s => s && s.step_template_id)
            .map(s => ({
              step_template_id: Number(s.step_template_id),
              alias: s.alias || '',
              variables: s.variables || {}
            }))
        };
      } else {
        payload = { source: 'inline', content: pipelineSource.content || '' };
      }
      await updatePipelineConfig(configRepo.id, payload);
      message.success('流水线配置已保存');
    } catch (err) {
      message.error(err?.message || '保存流水线配置失败');
    } finally {
      setYamlSaving(false);
    }
  };

  const saveSettings = async (payload?: unknown) => {
    if (!configRepo?.id) return;
    setSettingsSaving(true);
    try {
      const body = (payload as Record<string, unknown>) || buildSettingsPayload();
      await updatePipelineSettings(configRepo.id, body);
      setSettingsForm(prev => ({ ...prev, ...body }));
      setCronRows(Array.isArray(body.cron_schedules) ? (body.cron_schedules as string[]) : []);
      message.success('流水线设置已保存');
    } catch (err) {
      message.error(err?.message || '保存设置失败');
    } finally {
      setSettingsSaving(false);
    }
  };

  const handleConfigSave = () => {
    if (configDrawerTab === 'yaml') {
      return saveYaml();
    }
    if (configDrawerTab === 'docker') {
      return saveSettings(buildSettingsPayload({ dockerfile: settingsForm.dockerfile }));
    }
    return saveSettings();
  };

  const openRunModal = repo => {
    setRunRepo(repo);
    setRunForm({ branch: repo?.branch || 'main', commit: '', variables: [emptyVariableRow()] });
    setRunFormError('');
    setRunModalVisible(true);
  };

  const updateVariable = (index, key, value) => {
    setRunForm(prev => {
      const rows = normalizeVariableRows(prev.variables);
      rows[index][key] = value;
      return { ...prev, variables: rows };
    });
  };

  const removeVariable = index => {
    setRunForm(prev => {
      const rows = normalizeVariableRows(prev.variables).filter((_, idx) => idx !== index);
      return { ...prev, variables: rows.length ? rows : [emptyVariableRow()] };
    });
  };

  const addVariable = () => {
    setRunForm(prev => ({ ...prev, variables: [...prev.variables, emptyVariableRow()] }));
  };

  const triggerRun = async () => {
    if (!runRepo?.id) return;
    if (!runForm.branch.trim()) {
      setRunFormError('请填写构建分支');
      return;
    }
    setRunSubmitting(true);
    setRunFormError('');
    try {
      const payload = {
        branch: runForm.branch.trim(),
        commit: runForm.commit.trim() || undefined,
        variables: serializeVariableRows(runForm.variables)
      };
      await triggerPipelineRun(runRepo.id, payload);
      message.success('已触发构建');
      setRunModalVisible(false);
      navigate(`/ops/projects/pipeline?repo=${runRepo.id}&name=${encodeURIComponent(runRepo.full_name || runRepo.name)}`);
    } catch (err) {
      message.error(err?.message || '触发构建失败');
    } finally {
      setRunSubmitting(false);
    }
  };

  const columns = useMemo(
    () => [
      {
        title: '项目',
        dataIndex: 'full_name',
        render: (_, record) => (
          <div
            className="project-info"
            role="button"
            tabIndex={0}
            onClick={() => handleViewPipeline(record)}
            onKeyDown={event => {
              if (event.key === 'Enter') handleViewPipeline(record);
            }}
          >
            <div className="project-info__name">
              <span className="project-info__label">{record.full_name || record.name}</span>
            </div>
            <div className="project-info__meta">
              <span>{record.owner || '—'}</span>
              <span> / </span>
              <span>{record.branch || 'main'}</span>
            </div>
          </div>
        )
      },
      {
        title: '默认分支',
        dataIndex: 'branch',
        render: value => value || 'main'
      },
      {
        title: '可见性',
        dataIndex: 'visibility',
        render: value => {
          const normalized = String(value || '').toLowerCase();
          const color = normalized === 'private' ? 'magenta' : normalized === 'internal' ? 'gold' : 'green';
          return <Tag color={color}>{normalized || 'public'}</Tag>;
        }
      },
      {
        title: '同步状态',
        dataIndex: 'active',
        render: value =>
          value ? (
            <Tag color="green">已同步</Tag>
          ) : (
            <Tag color="red" bordered={false}>
              未同步
            </Tag>
          )
      },
      {
        title: '最后更新',
        dataIndex: 'updated',
        render: (_, record) => formatTime(record.updated || record.synced_at)
      },
      {
        title: '操作',
        dataIndex: 'actions',
        width: 320,
        fixed: 'right' as const,
        render: (_, record) => (
          <Space>
            {record.active ? (
              <>
                <Button type="primary" onClick={() => openRunModal(record)}>
                  构建
                </Button>
                <Button onClick={() => handleConfigPipeline(record)}>配置流水线</Button>
              </>
            ) : (
              <Button
                icon={<SyncOutlined spin={!!repoSyncing[record.forge_remote_id]} />}
                onClick={() => handleSyncRepo(record)}
                loading={!!repoSyncing[record.forge_remote_id]}
              >
                同步
              </Button>
            )}
          </Space>
        )
      }
    ],
    [handleConfigPipeline, handleSyncRepo, handleViewPipeline, repoSyncing]
  );

  return (
    <>
      <Card
      className="ops-project-card"
      title="项目管理 · 项目列表"
      extra={
        <Space size={12} className="ops-project-toolbar">
          <Segmented options={SYNC_OPTIONS} value={syncedFilter} onChange={handleSyncToggle} />
          <Input.Search
            placeholder="搜索项目名称"
            allowClear
            onSearch={handleSearch}
            style={{ width: 240 }}
            enterButton="搜索"
          />
          <Tooltip title="刷新列表">
            <Button icon={<ReloadOutlined />} onClick={() => fetchRepos(page)} />
          </Tooltip>
          <Button type="primary" loading={syncingAll} onClick={handleSyncAll} icon={<SyncOutlined />}>
            同步所有
          </Button>
        </Space>
      }
    >
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={repos}
        onRow={record => ({
          onClick: event => {
            if ((event.target as HTMLElement).closest('.ant-btn')) return;
            handleViewPipeline(record);
          }
        })}
        pagination={false}
      />
      <TablePagination
        page={page}
        pageSize={perPage}
        total={total}
        onChange={(nextPage, nextSize) => {
          fetchRepos(nextPage, { perPage: nextSize });
        }}
        className="table-pagination--flush"
      />
    </Card>
      <Drawer
        title={configRepo ? `配置流水线 · ${configRepo.full_name || configRepo.name}` : '配置流水线'}
        open={configDrawerVisible}
        width={760}
        onClose={closeConfigDrawer}
        extra={
          <Space>
            <Button onClick={closeConfigDrawer}>取消</Button>
            <Button type="primary" loading={configDrawerTab === 'yaml' ? yamlSaving : settingsSaving} onClick={handleConfigSave}>
              保存
            </Button>
          </Space>
        }
      >
        {configLoading ? (
          <Spin />
        ) : (
          <Tabs
            activeKey={configDrawerTab}
            onChange={setConfigDrawerTab}
            items={[
              {
                key: 'yaml',
                label: '流水线 YAML',
                children: (
                  <PipelineSourceEditor value={pipelineSource} onChange={setPipelineSource} repoId={configRepo?.id} />
                )
              },
              {
                key: 'settings',
                label: '基础设置',
                children: (
                  <div className="pipeline-settings">
                    <Checkbox
                      checked={settingsForm.cleanup_enabled}
                      onChange={e => setSettingsForm(prev => ({ ...prev, cleanup_enabled: e.target.checked }))}
                    >
                      删除过期构建记录
                    </Checkbox>
                    <div className="form-row">
                      <label>构建记录保留天数</label>
                      <Input
                        type="number"
                        min={0}
                        value={settingsForm.retention_days}
                        onChange={e => setSettingsForm(prev => ({ ...prev, retention_days: Number(e.target.value) }))}
                      />
                    </div>
                    <div className="form-row">
                      <label>构建记录最大数量</label>
                      <Input
                        type="number"
                        min={1}
                        value={settingsForm.max_records}
                        onChange={e => setSettingsForm(prev => ({ ...prev, max_records: Number(e.target.value) }))}
                      />
                    </div>
                    <Checkbox
                      checked={settingsForm.disallow_parallel}
                      onChange={e => setSettingsForm(prev => ({ ...prev, disallow_parallel: e.target.checked }))}
                    >
                      不允许并发构建
                    </Checkbox>
                    <div className="form-row cron-section">
                      <label>Cron 触发器</label>
                      <Space direction="vertical" style={{ width: '100%' }}>
                        {!cronRows.length && <span className="settings-empty">暂无 Cron 表达式</span>}
                        {cronRows.map((cron, index) => (
                          <Space key={`cron-${index}`} align="baseline" style={{ width: '100%' }}>
                            <Input
                              value={cron}
                              onChange={e => {
                                const rows = [...cronRows];
                                rows[index] = e.target.value;
                                setCronRows(rows);
                              }}
                              placeholder="例如：0 0 * * *"
                            />
                            <Button type="link" onClick={() => setCronRows(rows => rows.filter((_, idx) => idx !== index))}>
                              移除
                            </Button>
                          </Space>
                        ))}
                        <Button type="dashed" onClick={() => setCronRows(rows => [...rows, ''])}>
                          添加 Cron 表达式
                        </Button>
                      </Space>
                    </div>
                  </div>
                )
              },
              {
                key: 'docker',
                label: 'Dockerfile',
                children: (
                  <CodeEditor
                    language="dockerfile"
                    value={settingsForm.dockerfile}
                    onChange={value => setSettingsForm(prev => ({ ...prev, dockerfile: value }))}
                    placeholder="粘贴或编辑 Dockerfile 模板"
                  />
                )
              }
            ]}
          />
        )}
      </Drawer>
      <Modal
        open={runModalVisible}
        title={`运行流水线${runRepo ? ` · ${runRepo.full_name || runRepo.name}` : ''}`}
        onCancel={() => setRunModalVisible(false)}
        onOk={triggerRun}
        confirmLoading={runSubmitting}
        destroyOnClose
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
            <p className="modal-hint">这些键值会同步到流水线，可用于自定义参数。</p>
            {runForm.variables.map((row, idx) => (
              <Space key={`var-${idx}`} className="run-variable-row" align="baseline">
                <Input
                  value={row.key}
                  placeholder="变量名"
                  onChange={e => updateVariable(idx, 'key', e.target.value)}
                />
                <Input
                  value={row.value}
                  placeholder="变量值"
                  onChange={e => updateVariable(idx, 'value', e.target.value)}
                />
                <Button type="link" onClick={() => removeVariable(idx)}>
                  删除
                </Button>
              </Space>
            ))}
            <Button type="dashed" onClick={addVariable}>
              + 添加变量
            </Button>
          </div>
          {runFormError && <Alert type="error" message={runFormError} showIcon />}
        </Space>
      </Modal>
    </>
  );
};

export default ProjectList;
