import React, { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Collapse, Form, Input, Modal, Select, Space, Switch, Tag, Tooltip, message } from 'antd';
import { BuildOutlined } from '@ant-design/icons';
import { listCertificates } from '../api/system/certificates';
import { itemsFromListResponse } from '../utils/listResponse';

// BuildStepInserter 弹一个表单, 让用户填几行就生成 kind=build 的 step YAML 片段
// 并追加到当前编辑器内容. 接入位置:
//   - PipelineSourceEditor (inline 模式)
//   - PipelineJobEditor (YAML tab)
//   - PipelineTemplateEditor
//
// 父组件传 value/onChange 受控当前 YAML 内容, disabled=只读时禁用按钮.

const DEFAULT_PLATFORMS = ['linux/amd64'];

const TagsInput = ({ value, onChange, placeholder }) => {
  const tags = Array.isArray(value) ? value : [];
  const [draft, setDraft] = useState('');
  const commit = () => {
    const t = (draft || '').trim();
    if (!t || tags.includes(t)) {
      setDraft('');
      return;
    }
    onChange?.([...tags, t]);
    setDraft('');
  };
  return (
    <div>
      {tags.map((t, i) => (
        <Tag key={`${t}-${i}`} closable onClose={() => onChange?.(tags.filter((_, idx) => idx !== i))}>
          {t}
        </Tag>
      ))}
      <Input
        size="small"
        style={{ width: 180, marginTop: 4 }}
        value={draft}
        placeholder={placeholder || '回车追加'}
        onChange={e => setDraft(e.target.value)}
        onPressEnter={commit}
        onBlur={commit}
      />
    </div>
  );
};

const BuildArgsRows = ({ value, onChange }) => {
  const rows = Array.isArray(value) ? value : [];
  const setRow = (idx, patch) => onChange?.(rows.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  const removeRow = idx => onChange?.(rows.filter((_, i) => i !== idx));
  const addRow = () => onChange?.([...rows, { key: '', value: '' }]);
  return (
    <Space direction="vertical" style={{ width: '100%' }} size="small">
      {rows.map((row, idx) => (
        <Space key={idx} size="small">
          <Input
            size="small"
            style={{ width: 160 }}
            value={row.key}
            placeholder="KEY"
            onChange={e => setRow(idx, { key: e.target.value })}
          />
          <Input
            size="small"
            style={{ width: 240 }}
            value={row.value}
            placeholder="value"
            onChange={e => setRow(idx, { value: e.target.value })}
          />
          <Button size="small" type="link" danger onClick={() => removeRow(idx)}>
            删除
          </Button>
        </Space>
      ))}
      <Button size="small" onClick={addRow}>
        + 增加 build_arg
      </Button>
    </Space>
  );
};

const renderYAMLString = value => {
  if (!value) return '""';
  // 简单兜底: 对会触发 YAML 歧义的值加引号 (含空格 / 冒号 / # / 以非字母开头)
  if (/[\s:#]/.test(value) || /^[^A-Za-z_./-]/.test(value)) {
    return JSON.stringify(value);
  }
  return value;
};

const buildStepYAML = ({ stepName, certificate, repo, tags, dockerfile, context, platforms, push, privileged, rootless, target, noCache, buildArgs }) => {
  const lines = [];
  lines.push(`  - name: ${renderYAMLString(stepName)}`);
  lines.push('    kind: build');
  if (certificate) {
    lines.push(`    certificate: ${renderYAMLString(certificate)}`);
  }
  lines.push('    build:');
  lines.push(`      repo: ${renderYAMLString(repo)}`);
  if (tags.length > 0) {
    lines.push('      tags:');
    tags.forEach(t => lines.push(`        - ${renderYAMLString(t)}`));
  }
  if (dockerfile && dockerfile !== 'Dockerfile') {
    lines.push(`      dockerfile: ${renderYAMLString(dockerfile)}`);
  }
  if (context && context !== '.') {
    lines.push(`      context: ${renderYAMLString(context)}`);
  }
  if (platforms.length > 0 && !(platforms.length === 1 && platforms[0] === 'linux/amd64')) {
    lines.push('      platforms:');
    platforms.forEach(p => lines.push(`        - ${renderYAMLString(p)}`));
  }
  if (push === false) {
    lines.push('      push: false');
  }
  if (rootless) {
    lines.push('      buildkit_image: moby/buildkit:rootless');
  }
  if (privileged) {
    lines.push('      privileged: true');
  }
  if (target) {
    lines.push(`      target: ${renderYAMLString(target)}`);
  }
  if (noCache) {
    lines.push('      no_cache: true');
  }
  const filteredArgs = (buildArgs || []).filter(r => r.key && r.key.trim());
  if (filteredArgs.length > 0) {
    lines.push('      build_args:');
    filteredArgs.forEach(r => lines.push(`        ${renderYAMLString(r.key.trim())}: ${renderYAMLString(r.value || '')}`));
  }
  return lines.join('\n');
};

const ensureStepsHeader = (existing, snippet) => {
  const trimmed = (existing || '').replace(/\s+$/, '');
  if (trimmed === '') {
    return `steps:\n${snippet}\n`;
  }
  // 已经包含 steps: 的根映射: 直接尾随追加 step.
  if (/^\s*steps\s*:/m.test(trimmed)) {
    return `${trimmed}\n${snippet}\n`;
  }
  // 顶层是 step 序列模板 (kind=step): 直接追加同级 step.
  if (/^\s*-\s+/m.test(trimmed)) {
    return `${trimmed}\n${snippet}\n`;
  }
  // 兜底: 在末尾包一层 steps:
  return `${trimmed}\n\nsteps:\n${snippet}\n`;
};

type ButtonSize = 'small' | 'middle' | 'large';

const BuildStepInserter = ({
  value,
  onChange,
  disabled = false,
  buttonText = '插入 BuildKit 构建步骤',
  size = 'small'
}: {
  value: string;
  onChange?: (next: string) => void;
  disabled?: boolean;
  buttonText?: string;
  size?: ButtonSize;
}) => {
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm();
  const [certificates, setCertificates] = useState([]);
  const [loadingCerts, setLoadingCerts] = useState(false);
  const [tagsValue, setTagsValue] = useState(['latest']);
  const [platformsValue, setPlatformsValue] = useState(DEFAULT_PLATFORMS);
  const [buildArgs, setBuildArgs] = useState([]);

  const loadCerts = async () => {
    setLoadingCerts(true);
    try {
      const data = await listCertificates({ type: 'docker', per_page: 200 });
      const items = itemsFromListResponse(data);
      setCertificates(items);
    } catch (err) {
      message.error((err as Error)?.message || '加载凭证失败');
    } finally {
      setLoadingCerts(false);
    }
  };

  useEffect(() => {
    if (open) {
      loadCerts();
      form.setFieldsValue({
        stepName: 'build-and-push-image',
        repo: '',
        certificate: undefined,
        dockerfile: 'Dockerfile',
        context: '.',
        push: true,
        rootless: false,
        privileged: false,
        target: '',
        noCache: false
      });
      setTagsValue(['latest']);
      setPlatformsValue(DEFAULT_PLATFORMS);
      setBuildArgs([]);
    }
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleConfirm = () => {
    form
      .validateFields()
      .then(values => {
        if (tagsValue.length === 0) {
          message.warning('至少需要一个 tag');
          return;
        }
        const snippet = buildStepYAML({
          stepName: (values.stepName || 'build-and-push-image').trim(),
          certificate: (values.certificate || '').trim(),
          repo: values.repo.trim(),
          tags: tagsValue,
          dockerfile: (values.dockerfile || 'Dockerfile').trim(),
          context: (values.context || '.').trim(),
          platforms: platformsValue.length > 0 ? platformsValue : DEFAULT_PLATFORMS,
          push: !!values.push,
          rootless: !!values.rootless,
          privileged: !!values.privileged,
          target: (values.target || '').trim(),
          noCache: !!values.noCache,
          buildArgs
        });
        const next = ensureStepsHeader(value, snippet);
        onChange?.(next);
        setOpen(false);
      })
      .catch(() => undefined);
  };

  const certOptions = useMemo(
    () => certificates.map(c => ({ label: `${c.name}`, value: c.name })),
    [certificates]
  );

  return (
    <>
      <Tooltip title="在 YAML 末尾追加一个 kind=build 步骤, 用 BuildKit 构建并推送镜像 (无需 dockerd)">
        <Button size={size} icon={<BuildOutlined />} disabled={disabled} onClick={() => setOpen(true)}>
          {buttonText}
        </Button>
      </Tooltip>
      <Modal
        title="插入 BuildKit 构建步骤"
        open={open}
        width={680}
        onOk={handleConfirm}
        onCancel={() => setOpen(false)}
        okText="生成并追加到 YAML"
        cancelText="取消"
        destroyOnClose
      >
        <Alert
          type="info"
          showIcon
          message="无需 dockerd"
          description={
            <span>
              引擎默认在容器内用 <code>moby/buildkit:latest</code> daemonless 模式跑 <code>buildctl-daemonless.sh build</code> 推送到 registry, 兼容所有 Docker daemon. 凭证从所选 docker 凭证回填, 无需在 settings 写 username/password/registry. 想切 rootless 在「高级选项」里开关.
            </span>
          }
          style={{ marginBottom: 12 }}
        />
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item label="步骤名 (name)" name="stepName" rules={[{ required: true, message: '请填写步骤名' }]}>
            <Input placeholder="build-and-push-image" />
          </Form.Item>
          <Form.Item label="Docker 凭证 (certificate)" name="certificate">
            <Select
              loading={loadingCerts}
              placeholder="选择类型为 docker 的凭证"
              options={certOptions}
              allowClear
              showSearch
              optionFilterProp="label"
              notFoundContent={loadingCerts ? '加载中…' : '没有 docker 类型的凭证'}
            />
          </Form.Item>
          <Form.Item label="项目路径 (build.repo)" name="repo" rules={[{ required: true, message: '必填: 例如 sixx/devsys' }]}>
            <Input placeholder="sixx/devsys (registry 主机由凭证或下方覆盖项提供)" />
          </Form.Item>
          <Form.Item label="标签 (tags)" required>
            {/* eslint-disable-next-line no-template-curly-in-string */}
            <TagsInput value={tagsValue} onChange={setTagsValue} placeholder="latest / build-${CI_PIPELINE_NUMBER} 等" />
          </Form.Item>
          <Form.Item label="Dockerfile 路径" name="dockerfile">
            <Input placeholder="Dockerfile" />
          </Form.Item>
          <Form.Item label="构建上下文 (context)" name="context">
            <Input placeholder="." />
          </Form.Item>
          <Form.Item label="平台 (platforms)">
            <TagsInput value={platformsValue} onChange={setPlatformsValue} placeholder="linux/amd64" />
          </Form.Item>
          <Form.Item label="推送 (push)" name="push" valuePropName="checked" style={{ marginBottom: 0 }}>
            <Switch checkedChildren="开" unCheckedChildren="关" defaultChecked />
          </Form.Item>
          <Collapse
            ghost
            style={{ marginTop: 16 }}
            items={[
              {
                key: 'advanced',
                label: '高级选项',
                children: (
                  <>
                    <Form.Item
                      label="rootless 模式 (生成 buildkit_image: moby/buildkit:rootless)"
                      name="rootless"
                      valuePropName="checked"
                      tooltip="默认走 privileged + moby/buildkit:latest, 兼容所有 Docker daemon. 启用 rootless 需要 Linux + Docker 20.10+, Colima/Docker Desktop/旧 Docker 可能起不来."
                    >
                      <Switch />
                    </Form.Item>
                    <Form.Item
                      label="强制 privileged (即便镜像是 rootless 也加 --privileged)"
                      name="privileged"
                      valuePropName="checked"
                      tooltip="一般无需开启: 默认就是 privileged 模式. 仅在显式选 rootless 镜像后又想强制特权时使用."
                    >
                      <Switch />
                    </Form.Item>
                    <Form.Item label="多阶段构建目标 (target)" name="target">
                      <Input placeholder="例如 production-runtime" />
                    </Form.Item>
                    <Form.Item label="禁用缓存" name="noCache" valuePropName="checked">
                      <Switch />
                    </Form.Item>
                    <Form.Item label="构建参数 (build_args)">
                      <BuildArgsRows value={buildArgs} onChange={setBuildArgs} />
                    </Form.Item>
                  </>
                )
              }
            ]}
          />
        </Form>
      </Modal>
    </>
  );
};

export default BuildStepInserter;
