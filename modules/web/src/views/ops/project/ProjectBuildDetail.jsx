import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { Button, Card, Empty, Modal, Space, Spin, Tag, message, Input } from 'antd';
import { ReloadOutlined, ArrowLeftOutlined, StopOutlined } from '@ant-design/icons';
import clsx from 'clsx';
import { getPipelineRun, cancelPipelineRun, submitPipelineApproval } from '../../../api/project/pipeline';
import { formatPipelineStatus, getPipelineStatusClass, getPipelineBulletClass, isPipelineStatusActive, PIPELINE_STATUS, formatApprovalAction, normalizePipelineStatus } from '../../../constants/pipeline';
import { formatDuration, formatTime } from '../../../utils/time';
import './project.less';

const ProjectBuildDetail = () => {
  const { repoId, runId } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(false);

  const repoName = searchParams.get('name') || '';

  const loadDetail = useCallback(async () => {
    if (!repoId || !runId) return;
    setLoading(true);
    try {
      const data = await getPipelineRun(Number(repoId), Number(runId));
      setDetail(data);
    } catch (err) {
      message.error(err?.message || '加载构建详情失败');
    } finally {
      setLoading(false);
    }
  }, [repoId, runId]);

  useEffect(() => {
    loadDetail();
  }, [loadDetail]);

  const [currentStepId, setCurrentStepId] = useState(null);
  const [canceling, setCanceling] = useState(false);
  const [approvalModal, setApprovalModal] = useState({ visible: false, step: null, action: 'approve', comment: '' });
  const flatSteps = useMemo(() => {
    const list = [];
    (detail?.workflows || []).forEach(workflow => {
      (workflow.steps || []).forEach(step => list.push(step));
    });
    return list;
  }, [detail]);
  const isBranchSkippedStep = useCallback(step => {
    if (!step) return false;
    if (step.state === PIPELINE_STATUS.SKIPPED) {
      return true;
    }
    const logs = step.logs || [];
    return logs.some(entry => {
      const content = entry?.content || entry?.message || '';
      if (typeof content !== 'string') return false;
      return content.includes('步骤因分支条件被跳过');
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
    if (fallback) {
      setCurrentStepId(fallback.id);
    }
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
    if (normalized && normalized !== PIPELINE_STATUS.UNKNOWN) {
      return normalized;
    }
    if (Number(step?.finished) > 0) {
      return PIPELINE_STATUS.SUCCESS;
    }
    return PIPELINE_STATUS.RUNNING;
  };
  const selectedStep = visibleSteps.find(step => step.id === currentStepId) || visibleSteps[0];
  const selectedDisplayState = getStepDisplayState(selectedStep);
  const showApprovalActions = step => step?.approval && step.state === PIPELINE_STATUS.BLOCKED;

  const openApprovalModal = (step, action) => {
    setApprovalModal({ visible: true, step, action, comment: '' });
  };

  const submitApproval = async () => {
    const action = approvalModal.action || 'approve';
    if (!approvalModal.step?.id || !detail?.pipeline?.id) {
      setApprovalModal({ visible: false, step: null, action: 'approve', comment: '' });
      return;
    }
    const isApprove = action === 'approve';
    try {
      await submitPipelineApproval(Number(repoId), detail.pipeline.id, approvalModal.step.id, {
        action,
        comment: approvalModal.comment || ''
      });
      message.success(isApprove ? '审批通过' : '审批已驳回');
      setApprovalModal({ visible: false, step: null, action: 'approve', comment: '' });
      loadDetail();
    } catch (err) {
      message.error(err?.message || '审批失败');
    }
  };

  const goBack = () => {
    const query = new URLSearchParams();
    query.set('repo', repoId);
    if (repoName) {
      query.set('name', repoName);
    }
    navigate(`/ops/projects/pipeline?${query.toString()}`);
  };

  const renderLogs = step => {
    const logs = step?.logs;
    const extraLines = [];
    if (step?.approval) {
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
    if (!logs || !logs.length) {
      if (extraLines.length) {
        return <pre className="build-log">{extraLines.join('\n')}</pre>;
      }
      return <pre className="build-log">暂无日志</pre>;
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
        if (!value.endsWith('\n')) {
          value += '\n';
        }
        return value;
      })
      .join('');
    const extraPrefix = extraLines.length ? `${extraLines.join('\n')}\n` : '';
    return <pre className="build-log">{`${extraPrefix}${content}`}</pre>;
  };

  const summaryItems = [
    { label: '状态', value: <Tag className={clsx('project-status', `project-status--${getPipelineStatusClass(detail?.pipeline?.status)}`)}>{formatPipelineStatus(detail?.pipeline?.status)}</Tag> },
    { label: '分支', value: detail?.pipeline?.branch || '—' },
    { label: '提交', value: (detail?.pipeline?.commit || '').slice(0, 8) || '—' },
    { label: '触发', value: detail?.pipeline?.message || '—' },
    { label: '提交人', value: detail?.pipeline?.author || '—' },
    { label: '耗时', value: formatDuration(detail?.pipeline?.created * 1000, detail?.pipeline?.finished * 1000) || '—' },
    { label: '开始时间', value: formatTime((detail?.pipeline?.created || 0) * 1000) || '—' },
    { label: '结束时间', value: detail?.pipeline?.finished ? formatTime(detail.pipeline.finished * 1000) : '—' }
  ];

  return (
    <div className="ops-build-detail">
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={goBack}>
          返回列表
        </Button>
        <Button icon={<ReloadOutlined />} onClick={loadDetail} loading={loading}>
          刷新
        </Button>
        {detail?.pipeline && isPipelineStatusActive(detail.pipeline.status) && (
          <Button
            danger
            icon={<StopOutlined />}
            loading={canceling}
            onClick={async () => {
              if (!detail?.pipeline?.id) return;
              setCanceling(true);
              try {
                await cancelPipelineRun(Number(repoId), detail.pipeline.id);
                message.success('已发送取消请求');
                loadDetail();
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

      <Card title={`构建详情 · #${detail?.pipeline?.number || runId}`}>
        {loading && !detail ? (
          <div className="ops-build-detail__placeholder">
            <Spin />
          </div>
        ) : !detail ? (
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
                      <div className="build-flow__meta">{formatDuration(step.started * 1000, step.finished * 1000) || '—'}</div>
                      {/* 审批操作按钮挪到日志区域 */}
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
                  visibleSteps.map(step => {
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
                        <div>{formatPipelineStatus(displayState)} · {formatDuration(step.started * 1000, step.finished * 1000)}</div>
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
      <Space>
        <strong>{selectedStep.name || selectedStep.title}</strong>
        <Tag className={clsx('project-status', `project-status--${getPipelineStatusClass(selectedDisplayState)}`)}>
          {formatPipelineStatus(selectedDisplayState)}
        </Tag>
      </Space>
      <Space>
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
        <span>{formatDuration(selectedStep.started * 1000, selectedStep.finished * 1000) || '—'}</span>
      </Space>
    </div>
    {renderLogs(selectedStep)}
  </div>
) : (
  <div className="build-log-card">
    <pre className="build-log">请选择步骤查看日志</pre>
  </div>
)}
              </div>

      <Modal
        open={approvalModal.visible}
        title={`审批 · ${approvalModal.step?.name || ''}`}
        onCancel={() => setApprovalModal({ visible: false, step: null, action: 'approve', comment: '' })}
        onOk={submitApproval}
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
            </div>
          </>
        )}
      </Card>
    </div>
  );
};

export default ProjectBuildDetail;
