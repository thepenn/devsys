/**
 * 前端RSA加密工具
 * 使用JSEncrypt库进行RSA加密
 */

import JSEncrypt from 'jsencrypt'
import { getRSAPublicKey as requestRSAPublicKey } from '@/api/system/rsa'

let publicKey = null
let pendingPublicKeyRequest = null

// 清除缓存的公钥（开发调试用）
function clearPublicKeyCache() {
  publicKey = null
  pendingPublicKeyRequest = null
}

// 暴露清除缓存函数到全局（调试用）
if (typeof window !== 'undefined') {
  window.clearRSACache = clearPublicKeyCache
}

/**
 * 获取RSA公钥
 * @param {boolean} forceRefresh 是否强制刷新公钥
 */
export async function getRSAPublicKey(forceRefresh = false) {
  if (publicKey && !forceRefresh) {
    return publicKey
  }

  // 如果强制刷新，清除缓存
  if (forceRefresh) {
    publicKey = null
    pendingPublicKeyRequest = null
  }

  // 防止并发请求导致的竞态条件
  if (pendingPublicKeyRequest) {
    return pendingPublicKeyRequest
  }

  pendingPublicKeyRequest = (async() => {
    try {
      // 从后端获取RSA公钥
      const response = await requestRSAPublicKey()
      publicKey = response.public_key

      return publicKey
    } catch (error) {
      console.error('获取RSA公钥失败:', error)
      return null
    } finally {
      pendingPublicKeyRequest = null
    }
  })()

  return pendingPublicKeyRequest
}

/**
 * RSA加密用户密码（用于传输）
 * @param {string} password - 明文密码
 * @returns {Promise<string>} 加密后的密码
 */
export async function encryptUserPassword(password) {
  if (!password) return ''

  // 第一次尝试加密（强制刷新公钥以确保使用最新密钥）
  try {
    const key = await getRSAPublicKey(true) // 强制刷新
    if (!key) {
      console.warn('RSA公钥获取失败，尝试再次刷新公钥...')
      // 再次尝试刷新公钥重试
      const newKey = await getRSAPublicKey(true)
      if (!newKey) {
        console.warn('刷新公钥后仍然失败，使用明文传输')
        return password
      }

      const encrypt = new JSEncrypt()
      encrypt.setPublicKey(newKey)
      const encrypted = encrypt.encrypt(password)

      if (!encrypted) {
        console.warn('RSA加密失败，使用明文传输')
        return password
      }

      return encrypted
    }

    // 验证公钥格式
    if (!key.includes('-----BEGIN PUBLIC KEY-----')) {
      console.warn('RSA公钥格式错误，使用明文传输')
      return password
    }

    const encrypt = new JSEncrypt()
    encrypt.setPublicKey(key)

    const encrypted = encrypt.encrypt(password)

    if (!encrypted) {
      console.warn('RSA加密失败，使用明文传输')
      return password
    }

    return encrypted
  } catch (error) {
    console.error('用户密码RSA加密失败:', error)
    return password // 如果加密失败，返回原密码
  }
}

/**
 * RSA加密数据库密码（用于传输）
 * @param {string} password - 明文密码
 * @returns {Promise<string>} 加密后的密码
 */
export async function encryptDatabasePassword(password) {
  // 数据库密码和用户密码使用相同的RSA加密
  return await encryptUserPassword(password)
}

/**
 * 生成随机字符串
 * @param {number} length - 长度
 * @returns {string} 随机字符串
 */
export function generateRandomString(length = 32) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

// 导出默认对象
export default {
  encryptUserPassword,
  encryptDatabasePassword,
  generateRandomString,
  getRSAPublicKey
}
