import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Collapse, Empty, Input, Radio, Select, Space, Tag, message } from 'antd';
import { ArrowDownOutlined, ArrowUpOutlined, ReloadOutlined } from '@ant-design/icons';
import CodeEditor from './CodeEditor';
import BuildStepInserter from './BuildStepInserter';
import { listTemplates, renderTemplate } from '../api/system/pipelineTemplates';
import '../views/pipelineTemplates/pipeline-templates.less';

// PipelineSourceEditor 把项目流水线 YAML 的三种来源 (inline / template /
// compose) 收敛到一个组件, 供 ProjectList / ProjectBuild 的 Drawer 复用.
// 父组件通过 value/onChange 受控:
//
//   value = {
//     source: 'inline' | 'template' | 'compose',
//     content: string,                           // inline 编辑器内容
//     template_id: number | null,                // template 模式选中的模板
//     template_variables: { [k]: v },            // template 模式覆盖变量
//     compose_steps: [{ step_template_id, alias?, variables? }, ...]
//   }
//
// 组件只负责编辑 + 预览, 不发请求保存; 父组件在抽屉的 [保存] 里把 value
// 一次性 PUT 给 /repos/:id/pipeline/config.
//
// repoId (可选) 让模板预览接口注入项目 repo 上下文 (CI_REPO_FULL_NAME /
// REPO_CLONE_URL_AUTH 等), 让 Drawer 预览结果与真实触发完全一致.
const VARIABLE_PATTERN = /\$\{([A-Za-z_][A-Za-z0-9_-]*)/g;

const emptyVarRow = () => ({ key: '', value: '' });

const variablesToRows = vars => {
  if (!vars || typeof vars !== 'object') return [emptyVarRow()];
  const entries = Object.entries(vars);
  if (entries.length === 0) return [emptyVarRow()];
  return entries.map(([key, value]) => ({ key, value: value ?? '' }));
};

const rowsToVariables = rows =>
  (rows || [])
    .map(row => ({ key: (row.key || '').trim(), value: row.value ?? '' }))
    .filter(row => row.key !== '')
    .reduce((acc, row) => {
      acc[row.key] = row.value;
      return acc;
    }, {});

const PipelineSourceEditor = ({ value, onChange, disabled = false, repoId }) => {
  const source = value?.source === 'template' || value?.source === 'compose' ? value.source : 'inline';
  const content = value?.content || '';
  const templateId = value?.template_id ?? null;
  const variables = useMemo(() => value?.template_variables || {}, [value?.template_variables]);
  const composeSteps = useMemo(() => Array.isArray(value?.compose_steps) ? value.compose_steps : [], [value?.compose_steps]);

  const [allTemplates, setAllTemplates] = useState([]);
  const [stepTemplates, setStepTemplates] = useState([]);
  const [loadingTemplates, setLoadingTemplates] = useState(false);
  const [varRows, setVarRows] = useState(() => variablesToRows(variables));
  const [preview, setPreview] = useState({ content: '', missing: [], loading: false, error: '' });

  // 父组件 value.template_id / source 变化时同步本地 rows (例如重开 Drawer).
  useEffect(() => {
    setVarRows(variablesToRows(value?.template_variables));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value?.template_id, value?.source]);

  const fetchTemplates = useCallback(async () => {
    setLoadingTemplates(true);
    try {
      const all = await listTemplates({ published: 'true', per_page: 200 });
      const items = Array.isArray(all?.items) ? all.items : [];
      setAllTemplates(items);
      setStepTemplates(items.filter(t => t.kind === 'step'));
    } catch (err) {
      message.error(err?.message || '加载模板列表失败');
    } finally {
      setLoadingTemplates(false);
    }
  }, []);

  // template / compose 模式下都需要拉模板列表.
  useEffect(() => {
    if (source === 'template' || source === 'compose') {
      fetchTemplates();
    }
  }, [fetchTemplates, source]);

  const pipelineKindTemplates = useMemo(
    () => allTemplates.filter(t => !t.kind || t.kind === 'pipeline'),
    [allTemplates]
  );

  const selectedTemplate = useMemo(
    () => pipelineKindTemplates.find(item => item.id === templateId) || null,
    [pipelineKindTemplates, templateId]
  );

  const detectedVars = useMemo(() => {
    if (!selectedTemplate || !preview.content) return [];
    const matches = preview.content.matchAll(VARIABLE_PATTERN);
    const set = new Set();
    for (const m of matches) {
      set.add(m[1]);
    }
    return Array.from(set);
  }, [preview.content, selectedTemplate]);

  const refreshPreview = useCallback(
    async (overrideVars) => {
      if (!templateId) {
        setPreview({ content: '', missing: [], loading: false, error: '' });
        return;
      }
      setPreview(prev => ({ ...prev, loading: true, error: '' }));
      try {
        const data = await renderTemplate(templateId, overrideVars ?? variables, repoId);
        setPreview({
          content: data?.content || '',
          missing: Array.isArray(data?.missing) ? data.missing : [],
          loading: false,
          error: ''
        });
      } catch (err) {
        setPreview({
          content: '',
          missing: [],
          loading: false,
          error: err?.message || '加载预览失败'
        });
      }
    },
    [templateId, variables, repoId]
  );

  useEffect(() => {
    if (source === 'template' && templateId) {
      refreshPreview();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, templateId]);

  const emit = next => {
    onChange?.({
      source,
      content,
      template_id: templateId,
      template_variables: variables,
      compose_steps: composeSteps,
      ...next
    });
  };

  const handleSourceChange = e => {
    const next = e.target.value;
    if (next === source) return;
    emit({ source: next });
  };

  const handleTemplateChange = nextId => {
    emit({ source: 'template', template_id: nextId });
  };

  const updateVarRow = (idx, field, fieldValue) => {
    setVarRows(prev => {
      const next = prev.map((row, i) => (i === idx ? { ...row, [field]: fieldValue } : row));
      const merged = rowsToVariables(next);
      emit({ template_variables: merged });
      return next;
    });
  };

  const addVarRow = () => setVarRows(prev => [...prev, emptyVarRow()]);
  const removeVarRow = idx =>
    setVarRows(prev => {
      const next = prev.filter((_, i) => i !== idx);
      const merged = rowsToVariables(next.length ? next : [emptyVarRow()]);
      emit({ template_variables: merged });
      return next.length ? next : [emptyVarRow()];
    });

  const fillFromDetected = () => {
    if (!detectedVars.length) return;
    setVarRows(prev => {
      const existing = new Map(prev.map(row => [row.key.trim(), row.value]));
      const next = detectedVars.map(name => ({ key: name, value: existing.get(name) ?? '' }));
      const merged = rowsToVariables(next);
      emit({ template_variables: merged });
      return next.length ? next : [emptyVarRow()];
    });
  };

  // ---------- compose 模式辅助 ----------

  const updateComposeSteps = nextList => {
    emit({ compose_steps: nextList });
  };
  const addComposeStep = () => {
    updateComposeSteps([...composeSteps, { step_template_id: undefined, alias: '', variables: {} }]);
  };
  const removeComposeStep = idx => {
    updateComposeSteps(composeSteps.filter((_, i) => i !== idx));
  };
  const moveComposeStep = (idx, delta) => {
    const target = idx + delta;
    if (target < 0 || target >= composeSteps.length) return;
    const next = [...composeSteps];
    const tmp = next[idx];
    next[idx] = next[target];
    next[target] = tmp;
    updateComposeSteps(next);
  };
  const updateComposeStep = (idx, patch) => {
    const next = composeSteps.map((step, i) => (i === idx ? { ...step, ...patch } : step));
    updateComposeSteps(next);
  };

  return (
    <div className="pipeline-template-source">
      <Radio.Group value={source} onChange={handleSourceChange} disabled={disabled}>
        <Radio.Button value="inline">自定义 YAML</Radio.Button>
        <Radio.Button value="template">引用通用模板</Radio.Button>
        <Radio.Button value="compose">组装步骤模板</Radio.Button>
      </Radio.Group>

      {source === 'inline' && (
        <>
          <Space style={{ marginTop: 8, marginBottom: 8 }} wrap>
            <BuildStepInserter
              value={content}
              onChange={next => emit({ content: next })}
              disabled={disabled}
            />
          </Space>
          <CodeEditor
            language="yaml"
            value={content}
            onChange={next => emit({ content: next })}
            placeholder="粘贴或编辑流水线 YAML 内容"
            readOnly={disabled}
          />
        </>
      )}

      {source === 'template' && (
        <>
          <Space className="pipeline-template-source__row" wrap>
            <span>选择模板</span>
            <Select
              showSearch
              optionFilterProp="label"
              loading={loadingTemplates}
              value={templateId || undefined}
              style={{ minWidth: 280 }}
              placeholder="请选择已发布的完整 Pipeline 模板"
              onChange={handleTemplateChange}
              options={pipelineKindTemplates.map(item => ({
                value: item.id,
                label: `${item.display_name || item.name} (${item.name})`
              }))}
              disabled={disabled}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchTemplates} loading={loadingTemplates}>
              刷新
            </Button>
            {selectedTemplate?.is_published === false && (
              <Tag color="orange">该模板未发布, 无法触发流水线</Tag>
            )}
          </Space>

          {selectedTemplate?.description && (
            <Alert type="info" showIcon message={selectedTemplate.description} />
          )}

          {!templateId ? (
            <Empty description="请先选择模板" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          ) : (
            <>
              <div className="pipeline-template-vars">
                <p className="pipeline-template-vars__hint">
                  {/* eslint-disable-next-line no-template-curly-in-string */}
                  覆盖变量 (用于模板中的 <code>{'${VAR}'}</code> 占位符替换). 缺失的变量在触发时会被替换为空串
                  {/* eslint-disable-next-line no-template-curly-in-string */}
                  或模板的 <code>{'${VAR:-default}'}</code> 默认值; 命中凭证库名时按凭证回填.
                </p>
                {detectedVars.length > 0 && (
                  <Button size="small" onClick={fillFromDetected}>
                    根据预览自动填充变量名 ({detectedVars.length})
                  </Button>
                )}
                {varRows.map((row, idx) => (
                  <div className="pipeline-template-vars__row" key={`var-${idx}`}>
                    <Input
                      value={row.key}
                      placeholder="变量名 (例如 IMAGE_TAG)"
                      onChange={e => updateVarRow(idx, 'key', e.target.value)}
                      disabled={disabled}
                    />
                    <Input
                      value={row.value}
                      placeholder="变量值"
                      onChange={e => updateVarRow(idx, 'value', e.target.value)}
                      disabled={disabled}
                    />
                    <Button type="link" onClick={() => removeVarRow(idx)} disabled={disabled}>
                      删除
                    </Button>
                  </div>
                ))}
                <Button type="dashed" onClick={addVarRow} disabled={disabled}>
                  + 添加变量
                </Button>
              </div>

              <div className="pipeline-template-source__preview">
                <Space style={{ marginBottom: 8 }}>
                  <strong>最终生效 YAML 预览</strong>
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => refreshPreview()} loading={preview.loading}>
                    重新预览
                  </Button>
                  {preview.missing && preview.missing.length > 0 && (
                    <Tag color="orange">缺失变量: {preview.missing.join(', ')}</Tag>
                  )}
                </Space>
                {preview.error ? (
                  <Alert type="error" showIcon message={preview.error} />
                ) : (
                  <CodeEditor language="yaml" value={preview.content} readOnly placeholder="" className="code-editor--compact" />
                )}
              </div>
            </>
          )}
        </>
      )}

      {source === 'compose' && (
        <div className="pipeline-template-source__compose">
          <Alert
            type="info"
            showIcon
            message={
              <span>
                按顺序选择已发布的「步骤模板」, 每个模板可单独传变量. 触发时按顺序拼成完整 pipeline,
                项目级变量 (CI_REPO_*/REPO_CLONE_URL_AUTH 等) 自动注入到每个片段。
              </span>
            }
          />
          <Space style={{ marginTop: 8, marginBottom: 8 }}>
            <Button icon={<ReloadOutlined />} onClick={fetchTemplates} loading={loadingTemplates} size="small">
              刷新可选模板
            </Button>
            <Tag>步骤模板数量 {stepTemplates.length}</Tag>
          </Space>
          {composeSteps.length === 0 && (
            <Empty description="暂未选择任何步骤" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          )}
          <Collapse
            accordion={false}
            items={composeSteps.map((step, idx) => {
              const tplInfo = stepTemplates.find(t => t.id === step.step_template_id);
              const stepRows = variablesToRows(step.variables);
              return {
                key: `compose-${idx}`,
                label: (
                  <Space>
                    <Tag>#{idx + 1}</Tag>
                    <Select
                      size="small"
                      style={{ minWidth: 280 }}
                      showSearch
                      optionFilterProp="label"
                      placeholder="选择步骤模板"
                      value={step.step_template_id || undefined}
                      onClick={e => e.stopPropagation()}
                      onChange={v => updateComposeStep(idx, { step_template_id: v })}
                      options={stepTemplates.map(t => ({
                        value: t.id,
                        label: `${t.display_name || t.name} (${t.name})`
                      }))}
                      disabled={disabled}
                    />
                    {tplInfo?.is_published === false && <Tag color="orange">未发布</Tag>}
                  </Space>
                ),
                extra: (
                  <Space onClick={e => e.stopPropagation()}>
                    <Button
                      size="small"
                      icon={<ArrowUpOutlined />}
                      disabled={disabled || idx === 0}
                      onClick={() => moveComposeStep(idx, -1)}
                    />
                    <Button
                      size="small"
                      icon={<ArrowDownOutlined />}
                      disabled={disabled || idx === composeSteps.length - 1}
                      onClick={() => moveComposeStep(idx, 1)}
                    />
                    <Button size="small" danger disabled={disabled} onClick={() => removeComposeStep(idx)}>
                      移除
                    </Button>
                  </Space>
                ),
                children: (
                  <div className="pipeline-template-vars">
                    <Space style={{ marginBottom: 8 }} wrap>
                      <span>别名 (可选)</span>
                      <Input
                        size="small"
                        value={step.alias || ''}
                        placeholder="覆盖片段内 step.name; 多 step 自动加 -N"
                        style={{ width: 280 }}
                        disabled={disabled}
                        onChange={e => updateComposeStep(idx, { alias: e.target.value })}
                      />
                    </Space>
                    <p className="pipeline-template-vars__hint">
                      仅作用于本片段的覆盖变量 (优先级高于项目级变量).
                    </p>
                    {stepRows.map((row, rIdx) => (
                      <div className="pipeline-template-vars__row" key={`step-${idx}-var-${rIdx}`}>
                        <Input
                          value={row.key}
                          placeholder="变量名"
                          disabled={disabled}
                          onChange={e => {
                            const next = stepRows.map((r, i) => (i === rIdx ? { ...r, key: e.target.value } : r));
                            updateComposeStep(idx, { variables: rowsToVariables(next) });
                          }}
                        />
                        <Input
                          value={row.value}
                          placeholder="变量值"
                          disabled={disabled}
                          onChange={e => {
                            const next = stepRows.map((r, i) => (i === rIdx ? { ...r, value: e.target.value } : r));
                            updateComposeStep(idx, { variables: rowsToVariables(next) });
                          }}
                        />
                        <Button
                          type="link"
                          disabled={disabled}
                          onClick={() => {
                            const next = stepRows.filter((_, i) => i !== rIdx);
                            updateComposeStep(idx, { variables: rowsToVariables(next.length ? next : [emptyVarRow()]) });
                          }}
                        >
                          删除
                        </Button>
                      </div>
                    ))}
                    <Button
                      type="dashed"
                      size="small"
                      disabled={disabled}
                      onClick={() => updateComposeStep(idx, { variables: rowsToVariables([...stepRows, emptyVarRow()]) })}
                    >
                      + 添加变量
                    </Button>
                  </div>
                )
              };
            })}
          />
          <Button type="dashed" disabled={disabled} onClick={addComposeStep} style={{ marginTop: 12 }}>
            + 添加步骤
          </Button>
        </div>
      )}
    </div>
  );
};

export default PipelineSourceEditor;
