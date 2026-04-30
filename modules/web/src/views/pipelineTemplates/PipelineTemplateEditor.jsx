import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, Drawer, Form, Input, Modal, Space, Spin, Tabs, Tag, Tooltip, message } from 'antd';
import { ArrowLeftOutlined, CloudUploadOutlined, ProfileOutlined, ReloadOutlined } from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { hasAnyLabel } from '../../utils/permission';
import {
  getTemplate,
  listReferencingRepos,
  publishTemplate,
  saveDraft
} from '../../api/system/pipelineTemplates';
import CodeEditor from '../../components/CodeEditor';
import BuildStepInserter from '../../components/BuildStepInserter';
import { formatTime } from '../../utils/time';
import './pipeline-templates.less';

/* eslint-disable no-template-curly-in-string */
const PIPELINE_PLACEHOLDER =
  'name: example\nworkspace: /workspace\nsteps:\n  build:\n    image: golang:1.22\n    commands:\n      - go build ./...';
const STEP_PLACEHOLDER =
  'steps:\n  clone:\n    image: alpine/git:2.45.2\n    commands:\n      - git clone "${REPO_CLONE_URL_AUTH}" .';
/* eslint-enable no-template-curly-in-string */

const PipelineTemplateEditor = () => {
  const { id } = useParams();
  const numericId = Number(id);
  const navigate = useNavigate();
  const { user } = useAuth();
  const canWrite = useMemo(() => hasAnyLabel(user, ['pipeline_template:write']), [user]);

  const [loading, setLoading] = useState(false);
  const [template, setTemplate] = useState(null);
  const [meta, setMeta] = useState({ display_name: '', description: '' });
  const [draft, setDraft] = useState('');
  const [activeTab, setActiveTab] = useState('draft');

  const [savingDraft, setSavingDraft] = useState(false);
  const [publishing, setPublishing] = useState(false);

  const [refsOpen, setRefsOpen] = useState(false);
  const [refsLoading, setRefsLoading] = useState(false);
  const [refs, setRefs] = useState([]);

  const fetchTemplate = useCallback(async () => {
    if (!Number.isFinite(numericId) || numericId <= 0) {
      message.error('模板 id 非法');
      return;
    }
    setLoading(true);
    try {
      const data = await getTemplate(numericId);
      setTemplate(data);
      setMeta({
        display_name: data?.display_name || '',
        description: data?.description || ''
      });
      setDraft(data?.draft_content || '');
    } catch (err) {
      message.error(err?.message || '加载模板失败');
    } finally {
      setLoading(false);
    }
  }, [numericId]);

  useEffect(() => {
    fetchTemplate();
  }, [fetchTemplate]);

  const dirty = useMemo(() => {
    if (!template) return false;
    return (
      draft !== (template.draft_content || '') ||
      meta.display_name !== (template.display_name || '') ||
      meta.description !== (template.description || '')
    );
  }, [draft, meta, template]);

  // 错误提示统一交给 request.js 拦截器 (按 status 去重 + maxCount=3),
  // 这里不再 message.error 二次弹, 否则会把同 key toast 互相覆盖,
  // 用户反而看不到真正的后端错误信息 (例如 YAML 校验 detail).
  const handleSaveDraft = async () => {
    if (!canWrite || !template) return false;
    setSavingDraft(true);
    try {
      const updated = await saveDraft(template.id, {
        display_name: meta.display_name,
        description: meta.description,
        draft_content: draft
      });
      setTemplate(updated);
      message.success('草稿已保存');
      return true;
    } catch (_err) {
      return false;
    } finally {
      setSavingDraft(false);
    }
  };

  const handlePublish = () => {
    if (!canWrite || !template) return;
    if (dirty) {
      Modal.confirm({
        title: '当前草稿尚未保存',
        content: '是否先保存草稿再发布?',
        okText: '保存并发布',
        onOk: async () => {
          // 关键: save 失败时不继续 publish, 否则 publish 端拿到的还是
          // 上一份 (可能为空) 的 DraftContent, 报 "draft is empty" 把
          // 真正的 YAML 校验错误覆盖掉.
          const ok = await handleSaveDraft();
          if (!ok) return;
          await doPublish();
        }
      });
      return;
    }
    Modal.confirm({
      title: `确认发布模板 "${template.display_name || template.name}"?`,
      content: '发布后所有引用该模板的项目下次触发将立即使用新版本.',
      okText: '发布',
      okButtonProps: { type: 'primary' },
      onOk: doPublish
    });
  };

  const doPublish = async () => {
    if (!template) return;
    setPublishing(true);
    try {
      const updated = await publishTemplate(template.id);
      setTemplate(updated);
      message.success('已发布');
    } catch (_err) {
      // 错误提示由 request.js 拦截器弹出.
    } finally {
      setPublishing(false);
    }
  };

  const openReferences = async () => {
    if (!template) return;
    setRefsOpen(true);
    setRefsLoading(true);
    try {
      const data = await listReferencingRepos(template.id);
      setRefs(Array.isArray(data) ? data : []);
    } catch (_err) {
      setRefs([]);
    } finally {
      setRefsLoading(false);
    }
  };

  if (loading && !template) {
    return (
      <Card className="pipeline-template-editor">
        <Spin />
      </Card>
    );
  }

  return (
    <Card
      className="pipeline-template-editor"
      title={
        <Space>
          <Button type="link" icon={<ArrowLeftOutlined />} onClick={() => navigate('/ops/pipeline-templates')}>
            返回列表
          </Button>
          <strong>{template ? template.display_name || template.name : '模板'}</strong>
          {template?.kind === 'step' ? <Tag color="purple">步骤模板</Tag> : <Tag color="geekblue">完整 Pipeline</Tag>}
          {template?.is_published ? <Tag color="green">已发布</Tag> : <Tag color="orange">仅草稿</Tag>}
          {dirty && <Tag color="gold">未保存</Tag>}
        </Space>
      }
      extra={
        <Space>
          <Tooltip title="查看引用此模板的项目">
            <Button icon={<ProfileOutlined />} onClick={openReferences}>
              引用项目
            </Button>
          </Tooltip>
          <Tooltip title="重新加载">
            <Button icon={<ReloadOutlined />} onClick={fetchTemplate} />
          </Tooltip>
          {canWrite && (
            <>
              <Button onClick={handleSaveDraft} loading={savingDraft} disabled={!dirty}>
                保存草稿
              </Button>
              <Button
                type="primary"
                icon={<CloudUploadOutlined />}
                onClick={handlePublish}
                loading={publishing}
                disabled={!template}
              >
                发布
              </Button>
            </>
          )}
        </Space>
      }
    >
      {!template ? (
        <Alert type="error" message="模板不存在或已被删除" showIcon />
      ) : (
        <>
          <div className="pipeline-template-meta">
            <Form layout="vertical">
              <div className="pipeline-template-meta__row">
                <Form.Item label="模板标识 (name)">
                  <Input value={template.name} disabled />
                </Form.Item>
                <Form.Item label="显示名称 (display_name)">
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
              {template.is_published && (
                <Alert
                  type="success"
                  showIcon
                  message={`当前已发布版本 · ${formatTime(template.published_at)}${
                    template.published_by ? ' · ' + template.published_by : ''
                  }`}
                />
              )}
              {!template.is_published && (
                <Alert
                  type="warning"
                  showIcon
                  message="当前模板从未发布, 项目侧无法引用. 编辑草稿后点击右上角 [发布] 可对外生效."
                />
              )}
            </Form>
          </div>

          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'draft',
                label: `草稿${dirty ? ' (未保存)' : ''}`,
                children: (
                  <>
                    <Space style={{ marginBottom: 8 }} wrap>
                      <BuildStepInserter value={draft} onChange={setDraft} disabled={!canWrite} />
                    </Space>
                    <CodeEditor
                      language="yaml"
                      value={draft}
                      onChange={setDraft}
                      readOnly={!canWrite}
                      placeholder={template?.kind === 'step' ? STEP_PLACEHOLDER : PIPELINE_PLACEHOLDER}
                    />
                  </>
                )
              },
              {
                key: 'published',
                label: '已发布版本',
                children: template.is_published ? (
                  <CodeEditor
                    language="yaml"
                    value={template.published_content || ''}
                    onChange={() => {}}
                    readOnly
                    placeholder=""
                  />
                ) : (
                  <Alert type="info" showIcon message="尚未发布过, 引用方暂不可用" />
                )
              }
            ]}
          />
          <p className="pipeline-template-editor__hint">
            {/* eslint-disable-next-line no-template-curly-in-string */}
            草稿与已发布版本独立存储. 模板内可使用 <code>{'${VAR}'}</code> 或{' '}
            {/* eslint-disable-next-line no-template-curly-in-string */}
            <code>{'${VAR:-default}'}</code> 占位符, 项目侧引用时可填写 template_variables 进行替换.
          </p>
        </>
      )}

      <Drawer
        title="引用此模板的项目"
        open={refsOpen}
        width={520}
        onClose={() => setRefsOpen(false)}
      >
        {refsLoading ? (
          <Spin />
        ) : refs.length === 0 ? (
          <Alert type="info" showIcon message="暂无项目引用此模板" />
        ) : (
          <ul className="pipeline-template-refs">
            {refs.map(item => (
              <li key={item.repo_id}>
                <strong>{item.full_name || item.name}</strong>
                {item.owner && <span className="pipeline-template-refs__owner"> · {item.owner}</span>}
              </li>
            ))}
          </ul>
        )}
      </Drawer>
    </Card>
  );
};

export default PipelineTemplateEditor;
