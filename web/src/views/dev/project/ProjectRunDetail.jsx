import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Button, Card, Empty, Input, Modal, Spin, Tag, message } from 'antd';
import clsx from 'clsx';
import { useProjectContext } from './ProjectContext';
import {
  getPipelineRun,
  cancelPipelineRun,
  submitPipelineApproval
} from 'api/project/pipeline';
import {
  formatPipelineStatus,
  getPipelineStatusClass,
  getPipelineBulletClass,
  isPipelineStatusCancellable,
  isPipelineStatusActive,
  formatApprovalAction,
  formatApprovalState,
  getApprovalActionClass,
  normalizePipelineStatus,
  PIPELINE_STATUS
} from 'constants/pipeline';
import { isApprovalStep } from 'constants/step';
import { formatDuration, formatTime } from 'utils/time';
import { normalizeError } from 'utils/error';
import './ProjectRunDetail.less';

const ProjectRunDetail = () => {
  const { owner, name, repo } = useProjectContext();
  const { runId } = useParams();
  const repoId = repo?.id;
  const navigate = useNavigate();

  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [currentStepKey, setCurrentStepKey] = useState('');
  const [approvalComment, setApprovalComment] = useState('');
  const [approvalModal, setApprovalModal] = useState({ visible: false, action: '', step: null, comment: '' });
  const [approvalSubmitting, setApprovalSubmitting] = useState('');
  const [canceling, setCanceling] = useState(false);
  const timerRef = useRef(null);

  const flatSteps = useMemo(() => {
    const list = [];
    (detail?.workflows || []).forEach(workflow => {
      (workflow.steps || []).forEach(step => list.push(step));
    });
    return list;
  }, [detail]);

  const currentStep = useMemo(() => {
    if (!currentStepKey) return null;
    return flatSteps.find(step => stepKey(step) === currentStepKey) || null;
  }, [flatSteps, currentStepKey]);

  const currentLogs = currentStep?.logs || [];
  const currentApproval = currentStep?.approval || null;
  const approvalDecisions = currentApproval?.decisions || [];
  const approvalPending = currentApproval?.pending_approvers || [];

  useEffect(() => {
    if (!repoId) return;
    loadDetail();
    return () => clearPolling();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoId, runId]);

  const clearPolling = () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  };

  const schedulePolling = status => {
    clearPolling();
    if (isPipelineStatusActive(status)) {
      timerRef.current = setTimeout(() => loadDetail(false), 3000);
    }
  };

  const loadDetail = async (showSpinner = true) => {
    if (!repoId) return;
    if (showSpinner) {
      setLoading(true);
    }
    setError('');
    try {
      const data = await getPipelineRun(repoId, runId);
      setDetail(data);
      if (!currentStepKey && data?.workflows?.length) {
        const first = data.workflows[0]?.steps?.[0];
        if (first) {
          setCurrentStepKey(stepKey(first));
        }
      }
      schedulePolling(data?.pipeline?.status);
    } catch (err) {
      const normalized = normalizeError(err, '加载构建详情失败');
      setError(normalized.message);
    } finally {
      setLoading(false);
    }
  };

  const handleCancelRun = async () => {
    if (!detail?.pipeline?.id || !isPipelineStatusCancellable(detail.pipeline.status)) {
      return;
    }
    setCanceling(true);
    try {
      await cancelPipelineRun(repoId, detail.pipeline.id);
      message.success('取消构建成功');
      loadDetail();
    } catch (err) {
      message.error(normalizeError(err, '取消构建失败').message);
    } finally {
      setCanceling(false);
    }
  };

  const handleApproval = async (action, payload = {}) => {
    const targetStep = payload.step || currentStep;
    const targetApproval = targetStep?.approval;
    if (!targetStep || !targetApproval) return;
    setApprovalSubmitting(action);
    setError('');
    try {
      await submitPipelineApproval(repoId, runId, targetStep.id, {
        action,
        comment: payload.comment ?? approvalComment
      });
      message.success('审批操作成功');
      if (payload.step) {
        setApprovalModal({ visible: false, action: '', step: null, comment: '' });
      } else {
        setApprovalComment('');
      }
      loadDetail(false);
    } catch (err) {
      message.error(normalizeError(err, '审批操作失败').message);
    } finally {
      setApprovalSubmitting('');
    }
  };

  const openApprovalModal = (step, action) => {
    setApprovalModal({ visible: true, step, action, comment: '' });
  };

  const closeApprovalModal = () => {
    setApprovalModal({ visible: false, action: '', step: null, comment: '' });
  };

  const goBack = () => {
    navigate(`/dev/projects/${owner}/${name}/pipeline?highlight=${runId}`);
  };

  const stepClasses = step => clsx('pipeline-status-bullet', `pipeline-status-bullet--${getPipelineBulletClass(stepVisualState(step))}`, {
    'pipeline-status-bullet--empty': stepVisualState(step) === PIPELINE_STATUS.NOT_RUN
  });

  const stepVisualState = step => {
    if (!stepHasRun(step)) {
      return PIPELINE_STATUS.NOT_RUN;
    }
    const normalized = normalizePipelineStatus(step?.state);
    if (normalized && normalized !== PIPELINE_STATUS.UNKNOWN) {
      return normalized;
    }
    if (step?.finished) {
      return PIPELINE_STATUS.SUCCESS;
    }
    return PIPELINE_STATUS.RUNNING;
  };

  const stepHasRun = step => Number(step?.started) > 0;

  const handleDownloadLogs = step => {
    if (!step) return;
    const lines = (step.logs || []).map(log => log.content).join('\n');
    const blob = new Blob([lines], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${owner}-${name}-step-${stepKey(step)}.log`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const approvalActions = step => {
    if (!stepHasRun(step)) return [];
    const approval = step?.approval;
    if (!approval || (approval.state || '').toLowerCase() !== 'pending') {
      return [];
    }
    const actions = [];
    if (approval.can_approve) actions.push('approve');
    if (approval.can_reject) actions.push('reject');
    return actions;
  };

  const statusLabel = formatPipelineStatus(detail?.pipeline?.status);
  const statusClass = getPipelineStatusClass(detail?.pipeline?.status);

  return (
    <div className="build-detail">
      <Card className="build-detail__header">
        <div className="build-detail__meta">
          <Button type="link" onClick={goBack} className="build-detail__meta-back">
            ← 返回流水线
          </Button>
          <h2>构建 #{detail?.pipeline?.number || runId}</h2>
          <p className="build-detail__meta-line">
            <span>分支：{detail?.pipeline?.branch || '—'}</span>
            <span>Commit：{detail?.pipeline?.commit || '—'}</span>
            <span>触发时间：{formatTime(detail?.pipeline?.created) || '—'}</span>
            <span>耗时：{formatDuration(detail?.pipeline?.created, detail?.pipeline?.finished)}</span>
          </p>
        </div>
        <div className="build-detail__meta-actions">
          <Tag className={clsx('pipeline-status', `pipeline-status--${statusClass}`)}>{statusLabel}</Tag>
          {isPipelineStatusCancellable(detail?.pipeline?.status) && (
            <Button danger loading={canceling} onClick={handleCancelRun}>
              {canceling ? '取消中…' : '取消构建'}
            </Button>
          )}
        </div>
      </Card>

      {error && <Alert type="error" message={error} showIcon className="build-detail__error" />}

      <Card className="build-detail__steps">
        {loading ? (
          <div className="build-detail__loading">
            <Spin tip="加载中..." />
          </div>
        ) : !flatSteps.length ? (
          <Empty description="暂无步骤" />
        ) : (
          <>
            <div className="build-detail__flow">
              {flatSteps.map((step, idx) => (
                <React.Fragment key={stepKey(step)}>
                  <div
                    className={clsx('build-detail__flow-step', {
                      'build-detail__flow-step--active': stepKey(step) === currentStepKey
                    })}
                    onClick={() => setCurrentStepKey(stepKey(step))}
                  >
                    <div className="build-detail__flow-main">
                      <span className={stepClasses(step)} />
                      <div className="build-detail__flow-info">
                        <span className="build-detail__flow-name">{step.name || `Step #${step.pid}`}</span>
                        <span className="build-detail__flow-meta">
                          {formatPipelineStatus(stepVisualState(step))} · {formatDuration(step.started, step.finished)}
                        </span>
                      </div>
                    </div>
                    {approvalActions(step).length ? (
                      <div className="build-detail__flow-actions">
                        {approvalActions(step).map(action => (
                          <Button
                            key={`${stepKey(step)}-${action}`}
                            size="small"
                            className={`build-detail__flow-button build-detail__flow-button--${action}`}
                            onClick={e => {
                              e.stopPropagation();
                              openApprovalModal(step, action);
                            }}
                          >
                            {action === 'approve' ? '同意' : '拒绝'}
                          </Button>
                        ))}
                      </div>
                    ) : null}
                  </div>
                  {idx < flatSteps.length - 1 && <span className="build-detail__flow-arrow">→</span>}
                </React.Fragment>
              ))}
            </div>

            <div className="build-detail__layout">
              <aside className="build-detail__sidebar">
                <ul className="build-detail__step-list">
                  {flatSteps.map(step => (
                    <li
                      key={`sidebar-${stepKey(step)}`}
                      className={clsx('build-detail__step-item', {
                        'build-detail__step-item--active': stepKey(step) === currentStepKey
                      })}
                      onClick={() => setCurrentStepKey(stepKey(step))}
                    >
                      <span className={stepClasses(step)} />
                      <span className="build-detail__step-name">{step.name || `Step #${step.pid}`}</span>
                    </li>
                  ))}
                </ul>
              </aside>

              <div className="build-detail__logs">
                {!currentStep ? (
                  <div className="build-detail__logs-empty">请选择左侧的步骤查看日志。</div>
                ) : isApprovalStep(currentStep) ? (
                  <ApprovalPanel
                    step={currentStep}
                    approval={currentApproval}
                    approvalComment={approvalComment}
                    onCommentChange={setApprovalComment}
                    pendingApprovers={approvalPending}
                    decisions={approvalDecisions}
                    onApprove={() => handleApproval('approve')}
                    onReject={() => handleApproval('reject')}
                    submittingAction={approvalSubmitting}
                  />
                ) : (
                  <LogsPanel
                    step={currentStep}
                    logs={currentLogs}
                    onDownload={() => handleDownloadLogs(currentStep)}
                  />
                )}
              </div>
            </div>
          </>
        )}
      </Card>

      <Modal
        open={approvalModal.visible}
        title={approvalModal.action === 'approve' ? '同意审批' : '拒绝审批'}
        onCancel={closeApprovalModal}
        onOk={() => handleApproval(approvalModal.action, { comment: approvalModal.comment, step: approvalModal.step })}
        confirmLoading={approvalSubmitting === approvalModal.action}
      >
        <p>步骤：{approvalModal.step?.name || approvalModal.step?.pid || '--'}</p>
        <Input.TextArea
          rows={4}
          placeholder="填写审批备注（可选）"
          value={approvalModal.comment}
          onChange={e => setApprovalModal(prev => ({ ...prev, comment: e.target.value }))}
        />
      </Modal>
    </div>
  );
};

const LogsPanel = ({ step, logs, onDownload }) => (
  <>
    <header className="build-detail__logs-header">
      <div>
        <h4>{step.name || `Step #${step.pid}`}</h4>
        <p className="build-detail__logs-meta">
          <span>状态：{formatPipelineStatus(stepVisualState(step))}</span>
          <span>耗时：{formatDuration(step.started, step.finished)}</span>
        </p>
      </div>
      <Button onClick={onDownload}>下载日志</Button>
    </header>
    {logs?.length ? (
      <pre className="build-detail__log-viewer">
        {logs.map(log => (
          <code key={`${stepKey(step)}-${log.line || Math.random()}`}>{log.content}</code>
        ))}
      </pre>
    ) : (
      <div className="build-detail__logs-empty">暂无日志</div>
    )}
  </>
);

const ApprovalPanel = ({ step, approval, approvalComment, onCommentChange, pendingApprovers, decisions, onApprove, onReject, submittingAction }) => (
  <>
    <header className="build-detail__logs-header">
      <div>
        <h4>{step.name || `Step #${step.pid}`}</h4>
        <p className="build-detail__logs-meta">
          <span>状态：{formatApprovalState(approval?.state)}</span>
          {approval?.expires_at && approval?.state === 'pending' && (
            <span>超时：{formatTime(approval.expires_at)}</span>
          )}
        </p>
      </div>
    </header>
    <div className="build-detail__approval-body">
      <p className="build-detail__approval-message">{approval?.message || '等待审批'}</p>
      <div className="build-detail__approval-meta">
        <span>审批策略：{approval?.strategy === 'all' ? '会签（全部通过）' : '或签（任意一人通过）'}</span>
        {pendingApprovers?.length ? <span>剩余审批人：{pendingApprovers.join(', ')}</span> : null}
      </div>
      {(approval?.can_approve || approval?.can_reject) && (
        <>
          <Input.TextArea
            rows={3}
            value={approvalComment}
            placeholder="填写审批备注"
            onChange={e => onCommentChange(e.target.value)}
          />
          <div className="build-detail__approval-actions">
            {approval?.can_approve && (
              <Button type="primary" loading={submittingAction === 'approve'} disabled={submittingAction && submittingAction !== 'approve'} onClick={onApprove}>
                同意
              </Button>
            )}
            {approval?.can_reject && (
              <Button danger loading={submittingAction === 'reject'} disabled={submittingAction && submittingAction !== 'reject'} onClick={onReject}>
                拒绝
              </Button>
            )}
          </div>
        </>
      )}
      {decisions?.length ? (
        <div className="build-detail__approval-history">
          <h5>审批记录</h5>
          <ul>
            {decisions.map(record => (
              <li key={`${record.user}-${record.timestamp}`}> 
                <span className="build-detail__approval-user">{record.user}</span>
                <span className={clsx('build-detail__approval-action', getApprovalActionClass(record.action))}>
                  {formatApprovalAction(record.action)}
                </span>
                <span className="build-detail__approval-time">{formatTime(record.timestamp)}</span>
                {record.comment && <span className="build-detail__approval-comment-text">{record.comment}</span>}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  </>
);

const stepVisualState = step => {
  if (!step) return PIPELINE_STATUS.UNKNOWN;
  if (!Number(step?.started)) return PIPELINE_STATUS.NOT_RUN;
  const normalized = normalizePipelineStatus(step?.state);
  if (normalized && normalized !== PIPELINE_STATUS.UNKNOWN) {
    return normalized;
  }
  if (step?.finished) {
    return PIPELINE_STATUS.SUCCESS;
  }
  return PIPELINE_STATUS.RUNNING;
};

const stepKey = step => {
  if (!step) return '';
  return String(step.id || step.pid || '');
};

export default ProjectRunDetail;
