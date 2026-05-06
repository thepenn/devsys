import request from '../../utils/request';

export function listJobs(params: Record<string, unknown>) {
  return request({
    url: '/pipeline-jobs',
    method: 'get',
    params
  });
}

export function getJob(id: string | number) {
  return request({
    url: `/pipeline-jobs/${id}`,
    method: 'get'
  });
}

export function createJob(data: unknown) {
  return request({
    url: '/pipeline-jobs',
    method: 'post',
    data
  });
}

export function updateJob(id: string | number, data: unknown) {
  return request({
    url: `/pipeline-jobs/${id}`,
    method: 'put',
    data
  });
}

export function deleteJob(id: string | number) {
  return request({
    url: `/pipeline-jobs/${id}`,
    method: 'delete'
  });
}

export function triggerJob(id: string | number, data?: unknown) {
  return request({
    url: `/pipeline-jobs/${id}/run`,
    method: 'post',
    data: data || {}
  });
}

export function listJobRuns(id: string | number, params: Record<string, unknown>) {
  return request({
    url: `/pipeline-jobs/${id}/runs`,
    method: 'get',
    params
  });
}

export function getJobRun(id: string | number, runId: string | number) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}`,
    method: 'get'
  });
}

export function getJobRunMeta(id: string | number, runId: string | number) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}/meta`,
    method: 'get'
  });
}

export function getJobRunStepLogs(
  id: string | number,
  runId: string | number,
  stepId: string | number,
  params?: { after_line?: number; limit?: number }
) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}/steps/${stepId}/logs`,
    method: 'get',
    params
  });
}

export function cancelJobRun(id: string | number, runId: string | number, reason?: string) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}/cancel`,
    method: 'post',
    params: reason ? { reason } : undefined,
    data: {}
  });
}

export function submitJobApproval(id: string | number, runId: string | number, stepId: string | number, data: unknown) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}/steps/${stepId}/approval`,
    method: 'post',
    data
  });
}
