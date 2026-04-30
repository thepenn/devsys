import request from '../../utils/request';

// listTemplates 列出通用 pipeline 模板. params: page, per_page, keyword,
// published (boolean, true 时只返回有 published_content 的模板, 用于
// 项目侧选择器).
export function listTemplates(params) {
  return request({
    url: '/pipeline-templates',
    method: 'get',
    params
  });
}

export function getTemplate(id) {
  return request({
    url: `/pipeline-templates/${id}`,
    method: 'get'
  });
}

export function createTemplate(data) {
  return request({
    url: '/pipeline-templates',
    method: 'post',
    data
  });
}

// saveDraft 修改草稿元数据 + draft_content. 字段为 nullable 时可省略,
// 后端按 null 视作不修改.
export function saveDraft(id, data) {
  return request({
    url: `/pipeline-templates/${id}/draft`,
    method: 'put',
    data
  });
}

export function publishTemplate(id) {
  return request({
    url: `/pipeline-templates/${id}/publish`,
    method: 'post'
  });
}

export function deleteTemplate(id) {
  return request({
    url: `/pipeline-templates/${id}`,
    method: 'delete'
  });
}

export function listReferencingRepos(id) {
  return request({
    url: `/pipeline-templates/${id}/projects`,
    method: 'get'
  });
}

// renderTemplate 用变量预览模板渲染结果, 不写库. 用于项目侧 Drawer 实时
// 预览最终 YAML.
//
// repoId 可选: 项目 Drawer 预览时传当前项目 id, 让后端按 repo 上下文
// (CI_REPO_FULL_NAME / REPO_CLONE_URL_AUTH / BRANCH ...) 注入变量,
// 预览结果与真实触发完全一致.
export function renderTemplate(id, variables, repoId) {
  const body = { variables: variables || {} };
  if (repoId) {
    body.repo_id = Number(repoId);
  }
  return request({
    url: `/pipeline-templates/${id}/render`,
    method: 'post',
    data: body
  });
}
