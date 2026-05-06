import request from '../../utils/request';

export function listTemplates(params: Record<string, unknown>) {
  return request({
    url: '/pipeline-templates',
    method: 'get',
    params
  });
}

export function getTemplate(id: string | number) {
  return request({
    url: `/pipeline-templates/${id}`,
    method: 'get'
  });
}

export function createTemplate(data: unknown) {
  return request({
    url: '/pipeline-templates',
    method: 'post',
    data
  });
}

export function saveDraft(id: string | number, data: unknown) {
  return request({
    url: `/pipeline-templates/${id}/draft`,
    method: 'put',
    data
  });
}

export function publishTemplate(id: string | number) {
  return request({
    url: `/pipeline-templates/${id}/publish`,
    method: 'post'
  });
}

export function deleteTemplate(id: string | number) {
  return request({
    url: `/pipeline-templates/${id}`,
    method: 'delete'
  });
}

export function listReferencingRepos(id: string | number) {
  return request({
    url: `/pipeline-templates/${id}/projects`,
    method: 'get'
  });
}

export function renderTemplate(
  id: string | number,
  variables?: Record<string, unknown>,
  repoId?: string | number
) {
  const body: Record<string, unknown> = { variables: variables || {} };
  if (repoId) {
    body.repo_id = Number(repoId);
  }
  return request({
    url: `/pipeline-templates/${id}/render`,
    method: 'post',
    data: body
  });
}
