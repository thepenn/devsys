import request from '../../utils/request';

// listJobs 查询独立 Job. params: page / per_page / keyword.
export function listJobs(params) {
  return request({
    url: '/pipeline-jobs',
    method: 'get',
    params
  });
}

export function getJob(id) {
  return request({
    url: `/pipeline-jobs/${id}`,
    method: 'get'
  });
}

export function createJob(data) {
  return request({
    url: '/pipeline-jobs',
    method: 'post',
    data
  });
}

// updateJob 局部更新, 字段省略表示不修改. clear_credential=true 用于
// 主动解绑 git 凭证.
export function updateJob(id, data) {
  return request({
    url: `/pipeline-jobs/${id}`,
    method: 'put',
    data
  });
}

export function deleteJob(id) {
  return request({
    url: `/pipeline-jobs/${id}`,
    method: 'delete'
  });
}

// triggerJob 立即运行. variables 会与 Job.Variables 合并 (前者优先).
export function triggerJob(id, data) {
  return request({
    url: `/pipeline-jobs/${id}/run`,
    method: 'post',
    data: data || {}
  });
}

export function listJobRuns(id, params) {
  return request({
    url: `/pipeline-jobs/${id}/runs`,
    method: 'get',
    params
  });
}

export function getJobRun(id, runId) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}`,
    method: 'get'
  });
}

export function cancelJobRun(id, runId, reason) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}/cancel`,
    method: 'post',
    params: reason ? { reason } : undefined,
    data: {}
  });
}

export function submitJobApproval(id, runId, stepId, data) {
  return request({
    url: `/pipeline-jobs/${id}/runs/${runId}/steps/${stepId}/approval`,
    method: 'post',
    data
  });
}
