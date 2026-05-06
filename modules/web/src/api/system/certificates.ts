import request from '../../utils/request';

export function listCertificates(params: Record<string, unknown>) {
  return request({
    url: '/sys/certificates',
    method: 'get',
    params
  });
}

export function createCertificate(data: unknown) {
  return request({
    url: '/sys/certificates',
    method: 'post',
    data
  });
}

export function getCertificate(id: string | number, params: Record<string, unknown>) {
  return request({
    url: `/sys/certificates/${id}`,
    method: 'get',
    params
  });
}

export function updateCertificate(id: string | number, data: unknown) {
  return request({
    url: `/sys/certificates/${id}`,
    method: 'put',
    data
  });
}

export function deleteCertificate(id: string | number) {
  return request({
    url: `/sys/certificates/${id}`,
    method: 'delete'
  });
}
