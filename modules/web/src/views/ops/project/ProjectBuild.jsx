import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Card, Checkbox, Drawer, Empty, Input, Modal, Select, Space, Spin, Table, Tabs, Tag, Tooltip, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { listRepositories } from '../../../api/project/repos';
import { getPipelineConfig, getPipelineSettings, listPipelineRuns, triggerPipelineRun, updatePipelineConfig, updatePipelineSettings } from '../../../api/project/pipeline';
import { formatDuration, formatTime } from '../../../utils/time';
import { formatPipelineStatus, getPipelineStatusClass } from '../../../constants/pipeline';
import { emptyVariableRow, normalizeVariableRows, serializeVariableRows } from '../../../utils/pipelineRun';
import './project.less';

const DEFAULT_PIPELINE_SETTINGS = {
  cleanup_enabled: false,
  retention_days: 7,
  max_records: 10,
  dockerfile: '',
  disallow_parallel: false,
  cron_schedules: []
};

const ProjectBuild = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [repos, setRepos] = useState([]);
  const [selectedRepo, setSelectedRepo] = useState(null);
  const [runs, setRuns] = useState([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [loadingRuns, setLoadingRuns] = useState(false);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);
  const [total, setTotal] = useState(0);
  const [runModalVisible, setRunModalVisible] = useState(false);
  const [runForm, setRunForm] = useState({ branch: '', commit: '', variables: [emptyVariableRow()] });
  const [runSubmitting, setRunSubmitting] = useState(false);
  const [runFormError, setRunFormError] = useState('');
  const [configDrawerVisible, setConfigDrawerVisible] = useState(false);
  const [configRepo, setConfigRepo] = useState(null);
  const [configDrawerTab, setConfigDrawerTab] = useState('yaml');
  const [yamlContent, setYamlContent] = useState('');
  const [yamlSaving, setYamlSaving] = useState(false);
  const [configLoading, setConfigLoading] = useState(false);
  const [settingsForm, setSettingsForm] = useState({ ...DEFAULT_PIPELINE_SETTINGS });
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [cronRows, setCronRows] = useState([]);

  const repoOptions = useMemo(
    () =>
      (repos || []).map(repo => ({
        label: repo.full_name || repo.name,
        value: repo.id
      })),
    [repos]
  );

  const fetchRepos = useCallback(
    async (targetRepoId = null) => {
      setLoadingRepos(true);
      try {
        const data = await listRepositories({ page: 1, per_page: 100, synced: 'true' });
        const items = Array.isArray(data?.items) ? data.items : [];
        setRepos(items);
        if (items.length === 0) {
          setSelectedRepo(null);
          setRuns([]);
          setTotal(0);
          return;
        }
        const repoIdFromQuery = targetRepoId ?? Number(searchParams.get('repo'));
        const nextRepo = items.find(item => item.id === repoIdFromQuery) || items[0];
        setSelectedRepo(nextRepo);
        if (nextRepo) {
          const currentRepo = searchParams.get('repo');
          const currentName = searchParams.get('name') || '';
          const nextName = nextRepo.full_name || nextRepo.name || '';
          if (String(nextRepo.id) !== currentRepo || currentName !== nextName) {
            setSearchParams(prev => {
              const next = new URLSearchParams(prev);
              next.set('repo', nextRepo.id);
              next.set('name', nextName);
              return next;
            });
          }
        }
      } catch (err) {
        message.error(err.message || '加载项目列表失败');
      } finally {
        setLoadingRepos(false);
      }
    },
    [searchParams, setSearchParams]
  );

  useEffect(() => {
    fetchRepos();
  }, [fetchRepos]);

  const fetchRuns = useCallback(
    async (targetPage = 1) => {
      if (!selectedRepo?.id) {
        setRuns([]);
        setTotal(0);
        return;
      }
      setLoadingRuns(true);
      try {
        const data = await listPipelineRuns(selectedRepo.id, { page: targetPage, per_page: perPage });
        setRuns(Array.isArray(data?.items) ? data.items : []);
        setTotal(data?.total || 0);
        setPage(data?.page || targetPage);
        if (data?.per_page) {
          setPerPage(data.per_page);
        }
      } catch (err) {
        message.error(err.message || '加载构建记录失败');
      } finally {
        setLoadingRuns(false);
      }
    },
    [perPage, selectedRepo]
  );

  useEffect(() => {
    if (selectedRepo?.id) {
      fetchRuns(1);
    }
  }, [fetchRuns, selectedRepo]);

  const handleRepoChange = repoId => {
    const repo = repos.find(item => item.id === repoId);
    if (!repo) return;
    setSelectedRepo(repo);
    setSearchParams(prev => {
      const next = new URLSearchParams(prev);
      next.set('repo', repo.id);
      next.set('name', repo.full_name || repo.name || '');
      return next;
    });
  };

  const viewRunDetail = run => {
    if (!selectedRepo?.owner || !selectedRepo?.name || !run?.id) return;
    const query = new URLSearchParams();
    if (selectedRepo?.full_name || selectedRepo?.name) {
      query.set('name', selectedRepo.full_name || selectedRepo.name);
    }
    const suffix = query.toString();
    navigate(`/ops/projects/build/${selectedRepo.id}/${run.id}${suffix ? `?${suffix}` : ''}`);
  };

  const formatEvent = record => {
    const value = String(record?.event || '').toLowerCase();
    if (record?.title && record.title.includes('手动触发')) {
      return '手动触发';
    }
    const mapping = {
      push: '代码推送',
      manual: '手动触发',
      cron: '定时任务',
      tag: '标签触发',
      release: '发布',
      deploy: '部署',
      rollback: '回滚'
    };
    if (!value) return '手动触发';
    return mapping[value] || value;
  };

  const formatRemark = record => {
    const errors = Array.isArray(record?.errors) ? record.errors.map(err => err?.message).filter(Boolean) : [];
    const reasons = Array.isArray(record?.event_reason)
      ? record.event_reason.filter(Boolean)
      : Array.isArray(record?.eventReason)
        ? record.eventReason.filter(Boolean)
        : [];
    if (record?.message) {
      errors.unshift(record.message);
    }
    const all = errors.length ? errors : reasons;
    return all.length ? all.join('；') : '—';
  };

  const formatCommitBlock = record => {
    const hash = record?.commit || '';
    const desc = record?.message || record?.title || `${formatEvent(record)}（${record?.author || '系统'}）`;
    return {
      hash,
      desc
    };
  };

  const renderCommitCell = (hash, tooltip) => {
    const shortHash = (hash || '').slice(0, 8);
    if (!shortHash) {
      return <span className="pipeline-commit__hash">—</span>;
    }
    if (tooltip) {
      return (
        <Tooltip title={tooltip}>
          <span className="pipeline-commit__hash">{shortHash}</span>
        </Tooltip>
      );
    }
    return <span className="pipeline-commit__hash">{shortHash}</span>;
  };

  const columns = [
    {
      title: '状态',
      dataIndex: 'status',
      width: 150,
      fixed: 'left',
      render: (_, record) => (
        <button type="button" className="project-build__link" onClick={() => viewRunDetail(record)}>
          <Tag className={['project-status', `project-status--${getPipelineStatusClass(record.status)}`].join(' ')}>
            {formatPipelineStatus(record.status)}
          </Tag>
        </button>
      )
    },
    {
      title: '构建 #',
      dataIndex: 'number',
      width: 160,
      render: (_, record) => (
        <button type="button" className="project-build__link project-build__number" onClick={() => viewRunDetail(record)}>
          <span>#{record.number || record.id}</span>
          <span className="project-build__builder">{record.author || '—'}</span>
        </button>
      )
    },
    {
      title: '分支',
      dataIndex: 'branch',
      width: 160,
      render: value => value || '—'
    },
    {
      title: '提交',
      dataIndex: 'commit',
      width: 200,
      render: (_, record) => {
        const commit = formatCommitBlock(record);
        return renderCommitCell(commit.hash, commit.desc);
      }
    },
    {
      title: '上次提交',
      dataIndex: 'prev_commit',
      width: 200,
      render: (_, record) => renderCommitCell(record.prev_commit, record.prev_commit ? `上一次构建提交：${record.prev_commit}` : '')
    },
    {
      title: '开始时间',
      dataIndex: 'started',
      width: 200,
      render: (_, record) => formatTime(record.started || record.created)
    },
    {
      title: '耗时',
      dataIndex: 'duration',
      width: 140,
      render: (_, record) => formatDuration(record.started || record.created, record.finished || record.updated)
    },
    {
      title: '备注',
      dataIndex: 'message',
      render: (_, record) => {
        const remark = formatRemark(record);
        return (
          <span className="pipeline-remark" title={remark}>
            {remark}
          </span>
        );
      }
    }
  ];

  const selectedRepoLabel = selectedRepo?.full_name || selectedRepo?.name;
  const openRunModal = () => {
    if (!selectedRepo) return;
    setRunForm({ branch: selectedRepo.branch || 'main', commit: '', variables: [emptyVariableRow()] });
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

  const openConfigDrawer = useCallback(() => {
    if (!selectedRepo?.id) {
      message.warning('请先选择项目');
      return;
    }
    setConfigRepo(selectedRepo);
    setConfigDrawerVisible(true);
    setConfigDrawerTab('yaml');
    setConfigLoading(true);
    Promise.all([getPipelineConfig(selectedRepo.id), getPipelineSettings(selectedRepo.id)])
      .then(([config, settings]) => {
        setYamlContent(config?.content || '');
        const normalized = normalizePipelineSettings(settings);
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
  }, [selectedRepo]);

  const closeConfigDrawer = useCallback(() => {
    setConfigDrawerVisible(false);
    setConfigRepo(null);
    setYamlContent('');
    setSettingsForm({ ...DEFAULT_PIPELINE_SETTINGS });
    setCronRows([]);
    setConfigDrawerTab('yaml');
  }, []);

  const buildSettingsPayload = useCallback(
    overrides => ({
      cleanup_enabled: settingsForm.cleanup_enabled,
      retention_days: settingsForm.retention_days,
      max_records: settingsForm.max_records,
      disallow_parallel: settingsForm.disallow_parallel,
      dockerfile: settingsForm.dockerfile,
      cron_schedules: (overrides?.cron_schedules ?? cronRows).map(item => item.trim()).filter(Boolean),
      ...overrides
    }),
    [cronRows, settingsForm]
  );

  const saveYaml = useCallback(async () => {
    if (!configRepo?.id) return;
    setYamlSaving(true);
    try {
      await updatePipelineConfig(configRepo.id, { content: yamlContent });
      message.success('流水线 YAML 已保存');
    } catch (err) {
      message.error(err?.message || '保存 YAML 失败');
    } finally {
      setYamlSaving(false);
    }
  }, [configRepo, yamlContent]);

  const saveSettings = useCallback(
    async payload => {
      if (!configRepo?.id) return;
      setSettingsSaving(true);
      try {
        const body = payload || buildSettingsPayload();
        await updatePipelineSettings(configRepo.id, body);
        setSettingsForm(prev => ({ ...prev, ...body }));
        setCronRows(body.cron_schedules || []);
        message.success('流水线设置已保存');
      } catch (err) {
        message.error(err?.message || '保存设置失败');
      } finally {
        setSettingsSaving(false);
      }
    },
    [buildSettingsPayload, configRepo]
  );

  const handleConfigSave = useCallback(() => {
    if (configDrawerTab === 'yaml') {
      return saveYaml();
    }
    if (configDrawerTab === 'docker') {
      return saveSettings(buildSettingsPayload({ dockerfile: settingsForm.dockerfile }));
    }
    return saveSettings();
  }, [buildSettingsPayload, configDrawerTab, saveSettings, saveYaml, settingsForm.dockerfile]);

  const triggerRun = async () => {
    if (!selectedRepo?.id) return;
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
      await triggerPipelineRun(selectedRepo.id, payload);
      message.success('已触发流水线');
      setRunModalVisible(false);
      fetchRuns(page);
    } catch (err) {
      message.error(err?.message || '触发构建失败');
    } finally {
      setRunSubmitting(false);
    }
  };

  return (
    <Card
      className="ops-project-card"
      title="项目管理 · 项目构建"
      extra={
        <Space className="ops-project-toolbar">
          <Select
            loading={loadingRepos}
            options={repoOptions}
            value={selectedRepo?.id}
            placeholder="选择项目"
            style={{ minWidth: 260 }}
            onChange={handleRepoChange}
            showSearch
            optionFilterProp="label"
          />
          <Button type="primary" disabled={!selectedRepo} onClick={openRunModal}>
            构建
          </Button>
          <Button disabled={!selectedRepo} onClick={openConfigDrawer}>
            配置流水线
          </Button>
          <Tooltip title="刷新构建记录">
            <Button icon={<ReloadOutlined />} onClick={() => fetchRuns(page)} disabled={!selectedRepo}>
              刷新
            </Button>
          </Tooltip>
        </Space>
      }
    >
      {!selectedRepo ? (
        <Empty description="暂无同步项目，无法展示构建记录" />
      ) : (
        <>
          <div className="ops-project-selected">
            当前项目：<strong>{selectedRepoLabel}</strong>
          </div>
          <Table
            rowKey="id"
            columns={columns}
            loading={loadingRuns}
            dataSource={runs}
            className="ops-build-table"
            pagination={{
              current: page,
              pageSize: perPage,
              total,
              showSizeChanger: false,
              showTotal: value => `共 ${value} 条构建记录`,
              onChange: target => fetchRuns(target)
            }}
          />
        </>
      )}

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
                  <CodeEditor
                    language="yaml"
                    value={yamlContent}
                    onChange={setYamlContent}
                    placeholder="粘贴或编辑流水线 YAML 内容"
                  />
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
        title="运行流水线"
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
    </Card>
  );
};

export default ProjectBuild;

const normalizePipelineSettings = payload => {
  if (!payload) return { ...DEFAULT_PIPELINE_SETTINGS };
  const schedules = Array.isArray(payload.cron_schedules)
    ? payload.cron_schedules.filter(item => typeof item === 'string' && item.trim()).map(item => item.trim())
    : [];
  return {
    cleanup_enabled: Boolean(payload.cleanup_enabled),
    retention_days: Number.isFinite(payload.retention_days) ? payload.retention_days : DEFAULT_PIPELINE_SETTINGS.retention_days,
    max_records:
      Number.isFinite(payload.max_records) && payload.max_records > 0 ? payload.max_records : DEFAULT_PIPELINE_SETTINGS.max_records,
    dockerfile: payload.dockerfile || '',
    disallow_parallel: Boolean(payload.disallow_parallel),
    cron_schedules: schedules
  };
};

const escapeHtml = (value = '') =>
  value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

const highlightYaml = value => {
  let html = escapeHtml(value);
  html = html.replace(/(#.*?$)/gm, '<span class="code-comment">$1</span>');
  html = html.replace(/^(\s*-?\s*)([A-Za-z0-9_.-]+)(:)/gm, '$1<span class="code-key">$2</span>$3');
  html = html.replace(/(:\s*)([A-Za-z0-9_\-./{}$][^\n]*)/g, (_, prefix, val) => {
    if (/^(&lt;|&amp;)/.test(val)) return `${prefix}${val}`;
    if (/^<span/.test(val)) return `${prefix}${val}`;
    return `${prefix}<span class="code-value">${val}</span>`;
  });
  return html;
};

const highlightDockerfile = value => {
  let html = escapeHtml(value);
  html = html.replace(
    /^(\s*)(FROM|RUN|CMD|COPY|ADD|ENV|ARG|WORKDIR|ENTRYPOINT|EXPOSE|VOLUME|USER|LABEL|ONBUILD|STOPSIGNAL|HEALTHCHECK)(\b)/gim,
    '$1<span class="code-keyword">$2</span>$3'
  );
  html = html.replace(/(#.*?$)/gm, '<span class="code-comment">$1</span>');
  return html;
};

const CodeEditor = ({ value = '', onChange, language = 'yaml', placeholder = '开始编辑...' }) => {
  const textRef = useRef(null);
  const highlightRef = useRef(null);
  const highlighted = useMemo(
    () => (language === 'dockerfile' ? highlightDockerfile(value) : highlightYaml(value)),
    [language, value]
  );

  const syncScroll = event => {
    if (!highlightRef.current) return;
    highlightRef.current.scrollTop = event.target.scrollTop;
    highlightRef.current.scrollLeft = event.target.scrollLeft;
  };

  return (
    <div className="code-editor">
      <pre className="code-editor__highlight" ref={highlightRef} aria-hidden="true">
        <code dangerouslySetInnerHTML={{ __html: highlighted || escapeHtml(placeholder) }} />
      </pre>
      <textarea
        ref={textRef}
        className="code-editor__textarea"
        value={value}
        onChange={e => onChange?.(e.target.value)}
        onScroll={syncScroll}
        spellCheck={false}
        placeholder={placeholder}
      />
    </div>
  );
};
