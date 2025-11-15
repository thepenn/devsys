import request from '../../utils/request';

export function listCertificates(params) {
  return request({
    url: '/sys/certificates',
    method: 'get',
    params
  });
}

export function createCertificate(data) {
  return request({
    url: '/sys/certificates',
    method: 'post',
    data
  });
}

export function getCertificate(id, params) {
  return request({
    url: `/sys/certificates/${id}`,
    method: 'get',
    params
  });
}

export function updateCertificate(id, data) {
  return request({
    url: `/sys/certificates/${id}`,
    method: 'put',
    data
  });
}

export function deleteCertificate(id) {
  return request({
    url: `/sys/certificates/${id}`,
    method: 'delete'
  });
}
