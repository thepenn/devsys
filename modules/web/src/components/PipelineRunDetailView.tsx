import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Card, Empty, Input, Modal, Space, Spin, Switch, Tag, Tooltip, message } from 'antd';
import { ReloadOutlined, ArrowLeftOutlined, StopOutlined, CopyOutlined, RedoOutlined } from '@ant-design/icons';
import clsx from 'clsx';
import {
  formatPipelineStatus,
  getPipelineStatusClass,
  getPipelineBulletClass,
  isPipelineStatusActive,
  PIPELINE_STATUS,
  formatApprovalAction,
  normalizePipelineStatus
} from '../constants/pipeline';
import { formatDuration, formatStepDuration, formatTime } from '../utils/time';
import { consumePipelineLogSSE, pipelineLogTypeToString } from '../utils/pipelineLogStream';
import '../views/project/project.less';

function maxLogLine(logs) {
  if (!logs || !logs.length) return 0;
  return logs.reduce((m, e) => Math.max(m, Number(e.line) || 0), 0);
}

function findStepInRun(runDetail, stepId) {
  if (!runDetail?.workflows || !stepId) return null;
  for (const wf of runDetail.workflows) {
    const hit = (wf.steps || []).find(s => s.id === stepId);
    if (hit) return hit;
  }
  return null;
}

function mergeRunDetailWithMeta(prev, meta) {
  if (!meta?.pipeline) return prev;
  const prevLogs = new Map();
  (prev?.workflows || []).forEach(wf => {
    (wf.steps || []).forEach(s => prevLogs.set(s.id, s.logs || []));
  });
  const workflows = (meta.workflows || []).map(wf => ({
    ...wf,
    steps: (wf.steps || []).map(s => ({
      ...s,
      logs: prevLogs.has(s.id) ? prevLogs.get(s.id) : s.logs || []
    }))
  }));
  return { pipeline: meta.pipeline, workflows };
}

function appendLogItemsToStep(prev, stepId, items) {
  if (!prev?.workflows || !stepId || !items?.length) return prev;
  const workflows = prev.workflows.map(wf => ({
    ...wf,
    steps: (wf.steps || []).map(s => {
      if (s.id !== stepId) return s;
      const byLine = new Map(
        (s.logs || []).map(l => [l.line, l as { line?: number }])
      );
      for (const it of items) {
        const row = it as { line?: number };
        if (row && typeof row.line === 'number' && !byLine.has(row.line)) {
          byLine.set(row.line, row);
        }
      }
      const logs = Array.from(byLine.values()).sort((a, b) => {
        const al = Number((a as { line?: number }).line) || 0;
        const bl = Number((b as { line?: number }).line) || 0;
        return al - bl;
      });
      return { ...s, logs };
    })
  }));
  return { ...prev, workflows };
}

function patchStepState(prev, stepId, patch) {
  if (!prev?.workflows || !stepId || !patch) return prev;
  const workflows = prev.workflows.map(wf => ({
    ...wf,
    steps: (wf.steps || []).map(s => (s.id === stepId ? { ...s, ...patch } : s))
  }));
  return { ...prev, workflows };
}

// PipelineRunDetailView 是一次 pipeline run 的渲染骨架, 抽离自
// ProjectBuildDetail 以便项目构建 / 独立 Job 两条链路共用同一份 UI.
//
// Props:
//   - detail: { pipeline, workflows: [{ steps: [...] }] } 后端返回的 run detail.
//   - loading: bool, 用于刷新按钮动画.
//   - title: 卡片标题, 例如 "构建详情 · #12" 或 "Job 运行 · #12".
//   - onReload: () => Promise<void>, 用户点击刷新.
//   - onCancel: (pipelineID) => Promise<void> | undefined, 取消构建; 不传则不显示按钮.
//   - onReplay: ({branch, commit, variables}) => Promise<void> | undefined, 重新构建;
//       不传则不显示按钮. 仅在 run 已结束 (success/failure/killed/error/skipped) 时显示,
//       与 取消构建 互斥. 调用方应在内部触发新 run 并 navigate 到对应详情页.
//   - onApprove: ({ pipelineID, stepID, action, comment }) => Promise<void>, 审批回调.
//   - onBack: () => void, 返回按钮.
//   - livePoll: 可选, 运行中轮询 /meta 与增量 /logs; buildStreamUrl(stepId) 存在时对当前步骤用 Woodpecker 式 SSE.
const PipelineRunDetailView = ({
  detail,
  loading,
  title,
  onReload,
  onCancel,
  onReplay,
  onApprove,
  onBack,
  livePoll
}) => {
  const [currentStepId, setCurrentStepId] = useState(null);
  const [canceling, setCanceling] = useState(false);
  const [replaying, setReplaying] = useState(false);
  const [approvalModal, setApprovalModal] = useState({ visible: false, step: null, action: 'approve', comment: '' });
  const [mergedDetail, setMergedDetail] = useState(null);
  const [liveRefresh, setLiveRefresh] = useState(true);

  useEffect(() => {
    if (!livePoll) {
      setMergedDetail(null);
      return;
    }
    setMergedDetail(detail);
  }, [detail, livePoll]);

  const displayDetail = livePoll ? mergedDetail ?? detail : detail;
  const displayRef = useRef(displayDetail);
  displayRef.current = displayDetail;
  const currentStepIdRef = useRef(currentStepId);
  currentStepIdRef.current = currentStepId;

  const flatSteps = useMemo(() => {
    const list = [];
    (displayDetail?.workflows || []).forEach(workflow => {
      (workflow.steps || []).forEach(step => list.push(step));
    });
    return list;
  }, [displayDetail]);

  const isBranchSkippedStep = useCallback(step => {
    if (!step) return false;
    if (step.state === PIPELINE_STATUS.SKIPPED) return true;
    const logs = step.logs || [];
    return logs.some(entry => {
      const content = entry?.content || entry?.message || '';
      if (typeof content !== 'string') return false;
      // 兼容旧 (因分支条件) 与新 (因 when 条件) 两种日志文案,
      // 都视为"被 when 条件跳过"以便折叠显示.
      return content.includes('步骤因分支条件被跳过') || content.includes('步骤因 when 条件被跳过');
    });
  }, []);

  const visibleSteps = useMemo(() => flatSteps.filter(step => !isBranchSkippedStep(step)), [flatSteps, isBranchSkippedStep]);

  useEffect(() => {
    if (!visibleSteps.length) return;
    const activeStep = visibleSteps.find(step => isPipelineStatusActive(step.state));
    if (currentStepId) {
      const exists = visibleSteps.some(step => step.id === currentStepId);
      if (exists) return;
    }
    const fallback = activeStep || visibleSteps[0];
    if (fallback) setCurrentStepId(fallback.id);
  }, [currentStepId, visibleSteps]);

  const getStepDisplayState = step => {
    if (!step) return PIPELINE_STATUS.UNKNOWN;
    const hasStarted = Number(step?.started) > 0;
    const normalized = normalizePipelineStatus(step?.state);
    if (!hasStarted) {
      if (normalized === PIPELINE_STATUS.SUCCESS || normalized === PIPELINE_STATUS.FAILURE || normalized === PIPELINE_STATUS.ERROR) {
        return normalized;
      }
      return PIPELINE_STATUS.NOT_RUN;
    }
    if (normalized && normalized !== PIPELINE_STATUS.UNKNOWN) return normalized;
    if (Number(step?.finished) > 0) return PIPELINE_STATUS.SUCCESS;
    return PIPELINE_STATUS.RUNNING;
  };

  const selectedStep = visibleSteps.find(step => step.id === currentStepId) || visibleSteps[0];
  const selectedStepIndex = selectedStep ? visibleSteps.findIndex(s => s.id === selectedStep.id) : -1;
  const selectedDisplayState = getStepDisplayState(selectedStep);
  const showApprovalActions = step => step?.approval && step.state === PIPELINE_STATUS.BLOCKED;

  const openApprovalModal = (step, action) => setApprovalModal({ visible: true, step, action, comment: '' });

  const submitApprovalLocal = async () => {
    const action = approvalModal.action || 'approve';
    if (!approvalModal.step?.id || !displayDetail?.pipeline?.id) {
      setApprovalModal({ visible: false, step: null, action: 'approve', comment: '' });
      return;
    }
    try {
      await onApprove?.({
        pipelineID: displayDetail.pipeline.id,
        stepID: approvalModal.step.id,
        action,
        comment: approvalModal.comment || ''
      });
      message.success(action === 'approve' ? '审批通过' : '审批已驳回');
      setApprovalModal({ visible: false, step: null, action: 'approve', comment: '' });
    } catch (err) {
      message.error(err?.message || '审批失败');
    }
  };

  // buildStepLogText 把一个 step 的审批信息 + 日志条目拼成一段纯文本,
  // 给 <pre> 渲染和 复制日志 按钮共用. 没日志且没审批信息时返回空串,
  // 调用方根据空串决定显示 "暂无日志" 占位.
  const buildStepLogText = step => {
    if (!step) return '';
    const extraLines = [];
    if (step.approval) {
      if (step.approval.message) {
        extraLines.push(`等待审批：${step.approval.message}`);
      }
      const decisions = step.approval.decisions || [];
      decisions.forEach(decision => {
        const actionLabel = formatApprovalAction(decision.action);
        const comment = decision.comment ? ` · 意见：${decision.comment}` : '';
        extraLines.push(`审批${actionLabel ? `（${actionLabel}）` : ''}${decision.user ? ` - ${decision.user}` : ''}${comment}`);
      });
    }
    const logs = step.logs;
    if (!logs || !logs.length) {
      return extraLines.join('\n');
    }
    const content = logs
      .map(entry => {
        let value = '';
        let raw = entry;
        if (typeof entry === 'string') {
          raw = entry;
        } else if (entry && typeof entry === 'object') {
          raw = entry.content || entry.out || entry.stdout || entry.stderr || entry.message || '';
        }
        if (typeof raw === 'string' && raw.trim().startsWith('{')) {
          try {
            const parsed = JSON.parse(raw);
            value = parsed.content || parsed.message || raw;
          } catch (err) {
            value = raw;
          }
        } else {
          value = raw;
        }
        if (!value.endsWith('\n')) value += '\n';
        return value;
      })
      .join('');
    const extraPrefix = extraLines.length ? `${extraLines.join('\n')}\n` : '';
    return `${extraPrefix}${content}`;
  };

  const logViewportRef = useRef(null);

  const renderLogs = step => {
    const text = buildStepLogText(step);
    const logStyle = { maxHeight: '50vh', overflow: 'auto' as const };
    const refProp = step?.id === currentStepId ? { ref: logViewportRef } : {};
    if (!text) {
      return (
        <pre {...refProp} className="build-log" style={logStyle}>
          暂无日志
        </pre>
      );
    }
    return (
      <pre {...refProp} className="build-log" style={logStyle}>
        {text}
      </pre>
    );
  };

  const selectedLogText = useMemo(() => buildStepLogText(selectedStep), [selectedStep]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!liveRefresh || !logViewportRef.current) return;
    const el = logViewportRef.current;
    el.scrollTop = el.scrollHeight;
  }, [selectedLogText, liveRefresh, currentStepId]);

  useEffect(() => {
    if (!livePoll?.fetchMeta || !liveRefresh) return;
    const tick = async () => {
      const d = displayRef.current;
      if (!d?.pipeline?.id || !isPipelineStatusActive(d.pipeline.status)) return;
      try {
        const meta = await livePoll.fetchMeta();
        if (!meta?.pipeline) return;
        setMergedDetail(prev => mergeRunDetailWithMeta(prev ?? displayRef.current, meta));
      } catch {
        // ignore transient polling errors
      }
    };
    const id = window.setInterval(tick, 3000);
    void tick();
    return () => window.clearInterval(id);
  }, [livePoll, liveRefresh, detail?.pipeline?.id]);

  useEffect(() => {
    if (!livePoll?.fetchLogs || !liveRefresh) return;

    if (livePoll.buildStreamUrl) {
      const ac = new AbortController();
      const sid = currentStepId;
      const run = async () => {
        const d = displayRef.current;
        const st = findStepInRun(d, sid);
        if (!sid || !st || !isPipelineStatusActive(st.state)) return;
        await consumePipelineLogSSE(
          livePoll.buildStreamUrl(sid),
          {
            onLogLine: row => {
              const item = {
                line: row.line,
                type: pipelineLogTypeToString(row.type),
                time: row.time ?? 0,
                content: row.content || row.out || ''
              };
              setMergedDetail(prev => appendLogItemsToStep(prev ?? displayRef.current, sid, [item]));
            }
          },
          ac.signal
        );
      };
      void run();
      return () => ac.abort();
    }

    const iv = window.setInterval(async () => {
      const d = displayRef.current;
      const sid = currentStepIdRef.current;
      if (!d?.pipeline?.id || !isPipelineStatusActive(d.pipeline.status)) return;
      const st = findStepInRun(d, sid);
      if (!sid || !st || !isPipelineStatusActive(st.state)) return;
      const after = maxLogLine(st.logs);
      try {
        const chunk = await livePoll.fetchLogs(sid, after);
        const items = chunk?.items || [];
        if (items.length) {
          setMergedDetail(prev => appendLogItemsToStep(prev ?? displayRef.current, sid, items));
        }
        if (chunk?.step_state) {
          setMergedDetail(prev => patchStepState(prev ?? displayRef.current, sid, { state: chunk.step_state }));
        }
      } catch {
        // ignore
      }
    }, 2000);
    return () => window.clearInterval(iv);
  }, [livePoll, liveRefresh, currentStepId, detail?.pipeline?.id]);

  const summaryItems = [
    {
      label: '状态',
      value: (
        <Tag className={clsx('project-status', `project-status--${getPipelineStatusClass(displayDetail?.pipeline?.status)}`)}>
          {formatPipelineStatus(displayDetail?.pipeline?.status)}
        </Tag>
      )
    },
    { label: '分支', value: displayDetail?.pipeline?.branch || '—' },
    { label: '提交', value: (displayDetail?.pipeline?.commit || '').slice(0, 8) || '—' },
    { label: '触发', value: displayDetail?.pipeline?.message || '—' },
    { label: '提交人', value: displayDetail?.pipeline?.author || '—' },
    { label: '耗时', value: formatDuration(displayDetail?.pipeline?.created * 1000, displayDetail?.pipeline?.finished * 1000) || '—' },
    { label: '开始时间', value: formatTime((displayDetail?.pipeline?.created || 0) * 1000) || '—' },
    { label: '结束时间', value: displayDetail?.pipeline?.finished ? formatTime(displayDetail.pipeline.finished * 1000) : '—' }
  ];

  const cancelable = onCancel && displayDetail?.pipeline && isPipelineStatusActive(displayDetail.pipeline.status);
  // replay 仅在 run 已结束时启用, 与 取消构建 互斥 (cancel 只在 active 时显示).
  const replayable = onReplay && displayDetail?.pipeline && !isPipelineStatusActive(displayDetail.pipeline.status);

  return (
    <div className="ops-build-detail">
      <Space style={{ marginBottom: 16 }}>
        {onBack && (
          <Button icon={<ArrowLeftOutlined />} onClick={onBack}>
            返回列表
          </Button>
        )}
        {onReload && (
          <Button icon={<ReloadOutlined />} onClick={onReload} loading={loading}>
            刷新
          </Button>
        )}
        {livePoll && (
          <Tooltip title="关闭后不再自动拉取运行状态与日志">
            <Space>
              <span>实时刷新</span>
              <Switch checked={liveRefresh} onChange={setLiveRefresh} />
            </Space>
          </Tooltip>
        )}
        {replayable && (
          <Button
            type="primary"
            icon={<RedoOutlined />}
            loading={replaying}
            onClick={async () => {
              if (!displayDetail?.pipeline) return;
              setReplaying(true);
              try {
                await onReplay({
                  branch: displayDetail.pipeline.branch || '',
                  commit: displayDetail.pipeline.commit || '',
                  variables: displayDetail.pipeline.additional_variables || {}
                });
              } catch (err) {
                message.error(err?.message || '重新构建失败');
              } finally {
                setReplaying(false);
              }
            }}
          >
            重新构建
          </Button>
        )}
        {cancelable && (
          <Button
            danger
            icon={<StopOutlined />}
            loading={canceling}
            onClick={async () => {
              if (!displayDetail?.pipeline?.id) return;
              setCanceling(true);
              try {
                await onCancel(displayDetail.pipeline.id);
                message.success('已发送取消请求');
              } catch (err) {
                message.error(err?.message || '取消失败');
              } finally {
                setCanceling(false);
              }
            }}
          >
            取消构建
          </Button>
        )}
      </Space>

      <Card title={title || `构建详情 · #${displayDetail?.pipeline?.number || ''}`} bodyStyle={{ padding: '12px 16px' }}>
        {loading && !displayDetail ? (
          <div className="ops-build-detail__placeholder">
            <Spin />
          </div>
        ) : !displayDetail ? (
          <Empty description="暂无详情" />
        ) : (
          <>
            <div className="build-summary-grid">
              {summaryItems.map(item => (
                <div key={item.label} className="build-summary-item">
                  <span>{item.label}</span>
                  <div>{item.value}</div>
                </div>
              ))}
            </div>

            <div className="build-flow">
              {visibleSteps.map((step, index) => {
                const isApproval = step?.approval;
                const displayState = getStepDisplayState(step);
                return (
                  <React.Fragment key={step.id || step.name || index}>
                    <div className={clsx('build-flow__node', { 'build-flow__node--approval': isApproval })}>
                      <div className="build-flow__name">
                        {step.name || `Step ${index + 1}`}
                      </div>
                      <Tag className={clsx('project-status', `project-status--${getPipelineStatusClass(displayState)}`)}>
                        {formatPipelineStatus(displayState)}
                      </Tag>
                      <div className="build-flow__meta">
                        {formatStepDuration(step, displayDetail?.pipeline, visibleSteps, index) || '—'}
                      </div>
                      {isApproval && showApprovalActions(step) && (
                        <div className="approval-actions approval-actions--inline">
                          <button className="approval-btn approval-btn--approve" onClick={() => openApprovalModal(step, 'approve')}>
                            ✅ 同意
                          </button>
                          <button className="approval-btn approval-btn--reject" onClick={() => openApprovalModal(step, 'reject')}>
                            ✖ 驳回
                          </button>
                        </div>
                      )}
                    </div>
                    {index < visibleSteps.length - 1 && <div className="build-flow__connector" />}
                  </React.Fragment>
                );
              })}
            </div>

            <div className="build-steps">
              <div className="build-steps__sidebar">
                {visibleSteps.length ? (
                  visibleSteps.map((step, stepIdx) => {
                    const displayState = getStepDisplayState(step);
                    return (
                      <div
                        key={step.id || step.name}
                        className={clsx('build-steps__item', {
                          'build-steps__item--active': currentStepId === step.id
                        })}
                        onClick={() => setCurrentStepId(step.id)}
                      >
                        <span className={clsx('pipeline-status-bullet', `pipeline-status-bullet--${getPipelineBulletClass(displayState)}`)} />
                        <div>
                          <strong>{step.name || step.title || '步骤'}</strong>
                          <div>
                            {formatPipelineStatus(displayState)} ·{' '}
                            {formatStepDuration(step, displayDetail?.pipeline, visibleSteps, stepIdx) || '—'}
                          </div>
                        </div>
                      </div>
                    );
                  })
                ) : (
                  <span className="build-steps__empty">暂无步骤信息</span>
                )}
              </div>
              <div className="build-steps__content">
                {selectedStep ? (
                  <div className="build-log-card">
                    <div className="build-log-card__header">
                      <div className="build-log-card__header-left">
                        <Space size="small" wrap>
                          <strong>{selectedStep.name || selectedStep.title}</strong>
                          <Tag className={clsx('project-status', `project-status--${getPipelineStatusClass(selectedDisplayState)}`)}>
                            {formatPipelineStatus(selectedDisplayState)}
                          </Tag>
                          {showApprovalActions(selectedStep) && (
                            <div className="approval-actions approval-actions--inline">
                              <button className="approval-btn approval-btn--approve" onClick={() => openApprovalModal(selectedStep, 'approve')}>
                                ✅ 同意
                              </button>
                              <button className="approval-btn approval-btn--reject" onClick={() => openApprovalModal(selectedStep, 'reject')}>
                                ✖ 驳回
                              </button>
                            </div>
                          )}
                          <span className="build-log-card__header-duration">
                            {formatStepDuration(selectedStep, displayDetail?.pipeline, visibleSteps, selectedStepIndex) || '—'}
                          </span>
                        </Space>
                      </div>
                      <div className="build-log-card__header-actions">
                        <Tooltip title="复制完整日志到剪贴板">
                          <Button
                            size="small"
                            type="text"
                            icon={<CopyOutlined />}
                            disabled={!selectedLogText}
                            onClick={async () => {
                              if (!selectedLogText) return;
                              try {
                                if (navigator.clipboard?.writeText) {
                                  await navigator.clipboard.writeText(selectedLogText);
                                } else {
                                  // 兜底: 旧浏览器 / 非 https 没有 navigator.clipboard.
                                  const ta = document.createElement('textarea');
                                  ta.value = selectedLogText;
                                  ta.style.position = 'fixed';
                                  ta.style.opacity = '0';
                                  document.body.appendChild(ta);
                                  ta.select();
                                  document.execCommand('copy');
                                  document.body.removeChild(ta);
                                }
                                message.success('日志已复制');
                              } catch (err) {
                                message.error(err?.message || '复制失败, 请检查浏览器权限');
                              }
                            }}
                          />
                        </Tooltip>
                      </div>
                    </div>
                    {renderLogs(selectedStep)}
                  </div>
                ) : (
                  <div className="build-log-card">
                    <pre className="build-log">请选择步骤查看日志</pre>
                  </div>
                )}
              </div>
            </div>

            <Modal
              open={approvalModal.visible}
              title={`审批 · ${approvalModal.step?.name || ''}`}
              onCancel={() => setApprovalModal({ visible: false, step: null, action: 'approve', comment: '' })}
              onOk={submitApprovalLocal}
              okText={approvalModal.action === 'approve' ? '通过' : '驳回'}
              okButtonProps={{ danger: approvalModal.action === 'reject' }}
              centered
            >
              <p>操作：{approvalModal.action === 'approve' ? '通过' : '驳回'}</p>
              <Input.TextArea
                rows={4}
                placeholder="请输入审批意见"
                value={approvalModal.comment}
                onChange={e => setApprovalModal(prev => ({ ...prev, comment: e.target.value }))}
              />
            </Modal>
          </>
        )}
      </Card>
    </div>
  );
};

export default PipelineRunDetailView;
