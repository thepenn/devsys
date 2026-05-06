import request from '../../utils/request';

export function listAuditLogs(params: Record<string, unknown>) {
  return request({
    url: '/audit/logs',
    method: 'get',
    params
  });
}
