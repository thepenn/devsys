import request from '../../utils/request';

export function getPipelineConfig(repoId: string | number) {
  return request({
    url: `/repos/${repoId}/pipeline/config`,
    method: 'get'
  });
}

export function updatePipelineConfig(repoId: string | number, data: unknown) {
  return request({
    url: `/repos/${repoId}/pipeline/config`,
    method: 'put',
    data
  });
}

export function listPipelineRuns(repoId: string | number, params: Record<string, unknown>) {
  return request({
    url: `/repos/${repoId}/pipeline/runs`,
    method: 'get',
    params
  });
}

export function getPipelineRun(repoId: string | number, pipelineId: string | number) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}`,
    method: 'get'
  });
}

export function getPipelineRunMeta(repoId: string | number, pipelineId: string | number) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}/meta`,
    method: 'get'
  });
}

export function getPipelineStepLogs(
  repoId: string | number,
  pipelineId: string | number,
  stepId: string | number,
  params?: { after_line?: number; limit?: number }
) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}/steps/${stepId}/logs`,
    method: 'get',
    params
  });
}

export function triggerPipelineRun(repoId: string | number, data: unknown) {
  return request({
    url: `/repos/${repoId}/pipeline/run`,
    method: 'post',
    data
  });
}

export function cancelPipelineRun(repoId: string | number, pipelineId: string | number) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}/cancel`,
    method: 'post'
  });
}

export function submitPipelineApproval(
  repoId: string | number,
  pipelineId: string | number,
  stepId: string | number,
  data: unknown
) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}/steps/${stepId}/approval`,
    method: 'post',
    data
  });
}

export function getPipelineSettings(repoId: string | number) {
  return request({
    url: `/repos/${repoId}/pipeline/settings`,
    method: 'get'
  });
}

export function updatePipelineSettings(repoId: string | number, data: unknown) {
  return request({
    url: `/repos/${repoId}/pipeline/settings`,
    method: 'put',
    data
  });
}
