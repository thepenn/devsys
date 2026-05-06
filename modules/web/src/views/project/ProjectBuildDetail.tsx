import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { message } from 'antd';
import {
  getPipelineRun,
  getPipelineRunMeta,
  getPipelineStepLogs,
  cancelPipelineRun,
  submitPipelineApproval,
  triggerPipelineRun
} from '../../api/project/pipeline';
import PipelineRunDetailView from '../../components/PipelineRunDetailView';
import './project.less';

// ProjectBuildDetail 是单次 repo pipeline 运行的详情页. 主要逻辑全部下沉到
// shared 的 PipelineRunDetailView, 这里只负责数据加载与回调注入. 同结构的
// PipelineJobRunDetail 复用同一份 view.
const ProjectBuildDetail = () => {
  const { repoId, runId } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(false);

  const repoName = searchParams.get('name') || '';

  const livePoll = useMemo(
    () => ({
      fetchMeta: () => getPipelineRunMeta(Number(repoId), Number(runId)),
      fetchLogs: (stepId, afterLine) =>
        getPipelineStepLogs(Number(repoId), Number(runId), stepId, { after_line: afterLine, limit: 1000 }),
      ...(detail?.pipeline != null
        ? {
            buildStreamUrl: (stepId: number) =>
              `/stream/logs/${repoId}/${detail.pipeline.number}/${stepId}`
          }
        : {})
    }),
    [repoId, runId, detail?.pipeline]
  );

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

  const goBack = () => {
    const query = new URLSearchParams();
    query.set('repo', repoId);
    if (repoName) query.set('name', repoName);
    navigate(`/ops/projects/pipeline?${query.toString()}`);
  };

  return (
    <PipelineRunDetailView
      detail={detail}
      loading={loading}
      title={`构建详情 · #${detail?.pipeline?.number || runId}`}
      livePoll={repoId && runId ? livePoll : undefined}
      onReload={loadDetail}
      onBack={goBack}
      onCancel={async pipelineID => {
        await cancelPipelineRun(Number(repoId), pipelineID);
        loadDetail();
      }}
      onReplay={async params => {
        // 拿当前 run 的 branch/commit/variables 触发新 run, 拿到新 run 的 id 后
        // 直接 navigate 过去, 用户进新页就能看到 pending → running.
        const result = (await triggerPipelineRun(Number(repoId), params)) as { id?: number };
        const newID = result?.id;
        message.success('已触发重新构建');
        if (newID) {
          const query = new URLSearchParams();
          if (repoName) query.set('name', repoName);
          const qs = query.toString();
          navigate(qs ? `/ops/projects/build/${repoId}/${newID}?${qs}` : `/ops/projects/build/${repoId}/${newID}`);
        } else {
          loadDetail();
        }
      }}
      onApprove={async ({ pipelineID, stepID, action, comment }) => {
        await submitPipelineApproval(Number(repoId), pipelineID, stepID, { action, comment });
        loadDetail();
      }}
    />
  );
};

export default ProjectBuildDetail;
