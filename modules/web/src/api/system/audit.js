import request from '../../utils/request';

// listAuditLogs 查询操作审计日志.
// params 支持: page, per_page, login, method, path, user_id, start, end (unix sec).
export function listAuditLogs(params) {
  return request({
    url: '/audit/logs',
    method: 'get',
    params
  });
}
