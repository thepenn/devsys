import request from '@/utils/request'

// 获取RSA公钥
export function getRSAPublicKey() {
  return request({
    url: '/sys/rsa/public-key',
    method: 'get'
  })
}
