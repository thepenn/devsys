import request from '@/utils/request'

export function getPipelineConfig(repoId) {
  return request({
    url: `/repos/${repoId}/pipeline/config`,
    method: 'get'
  })
}

export function updatePipelineConfig(repoId, data) {
  return request({
    url: `/repos/${repoId}/pipeline/config`,
    method: 'put',
    data
  })
}

export function listPipelineRuns(repoId, params) {
  return request({
    url: `/repos/${repoId}/pipeline/runs`,
    method: 'get',
    params
  })
}

export function getPipelineRun(repoId, pipelineId) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}`,
    method: 'get'
  })
}

export function triggerPipelineRun(repoId, data) {
  return request({
    url: `/repos/${repoId}/pipeline/run`,
    method: 'post',
    data
  })
}

export function cancelPipelineRun(repoId, pipelineId) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}/cancel`,
    method: 'post'
  })
}

export function submitPipelineApproval(repoId, pipelineId, stepId, data) {
  return request({
    url: `/repos/${repoId}/pipeline/runs/${pipelineId}/steps/${stepId}/approval`,
    method: 'post',
    data
  })
}

export function getPipelineSettings(repoId) {
  return request({
    url: `/repos/${repoId}/pipeline/settings`,
    method: 'get'
  })
}

export function updatePipelineSettings(repoId, data) {
  return request({
    url: `/repos/${repoId}/pipeline/settings`,
    method: 'put',
    data
  })
}
