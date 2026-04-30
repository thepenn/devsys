import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { message } from 'antd';
import {
  cancelJobRun,
  getJobRun,
  submitJobApproval,
  triggerJob
} from '../../api/system/pipelineJobs';
import PipelineRunDetailView from '../../components/PipelineRunDetailView';

// PipelineJobRunDetail 是独立 Job 单次运行详情, 复用与 ProjectBuildDetail
// 一致的 PipelineRunDetailView, 仅替换数据源 / 回调.
const PipelineJobRunDetail = () => {
  const { id, runId } = useParams();
  const navigate = useNavigate();
  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(false);

  const loadDetail = useCallback(async () => {
    if (!id || !runId) return;
    setLoading(true);
    try {
      const data = await getJobRun(Number(id), Number(runId));
      setDetail(data);
    } catch (err) {
      message.error(err?.message || '加载运行详情失败');
    } finally {
      setLoading(false);
    }
  }, [id, runId]);

  useEffect(() => {
    loadDetail();
  }, [loadDetail]);

  return (
    <PipelineRunDetailView
      detail={detail}
      loading={loading}
      title={`Job 运行详情 · #${detail?.pipeline?.number || runId}`}
      onReload={loadDetail}
      onBack={() => navigate(`/ops/pipeline-jobs/${id}`)}
      onCancel={async pipelineID => {
        await cancelJobRun(Number(id), pipelineID);
        loadDetail();
      }}
      onReplay={async params => {
        // Job 的 trigger 只接受 variables (branch / commit 由 Job.Git* 配置控制),
        // 把当次 run 的 variables 透传过去复现.
        const result = await triggerJob(Number(id), { variables: params.variables || {} });
        const newID = result?.id;
        message.success('已触发重新构建');
        if (newID) {
          navigate(`/ops/pipeline-jobs/${id}/runs/${newID}`);
        } else {
          loadDetail();
        }
      }}
      onApprove={async ({ pipelineID, stepID, action, comment }) => {
        await submitJobApproval(Number(id), pipelineID, stepID, { action, comment });
        loadDetail();
      }}
    />
  );
};

export default PipelineJobRunDetail;
