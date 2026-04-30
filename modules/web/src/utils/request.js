import axios from 'axios';
import { message } from 'antd';
import { getToken, setToken, clearToken } from './auth';

// 全局限制同时显示的 toast 上限. 与下面拦截器里的"按 key 去重"配合,
// 后端不在线时多个并发请求失败也只会显示一条 backend-unreachable
// 弹窗 (而不是堆叠刷屏).
message.config({ maxCount: 3 });

const API_PREFIX = process.env.REACT_APP_API_PREFIX || '/api/v1';

const trimTrailingSlash = value => (value || '').replace(/\/+$/, '');

const resolveDevBackendOrigin = () => {
  if (process.env.REACT_APP_DEV_BACKEND_ORIGIN) {
    return trimTrailingSlash(process.env.REACT_APP_DEV_BACKEND_ORIGIN);
  }
  if (typeof window !== 'undefined' && window.location) {
    const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:';
    const host = window.location.hostname || 'localhost';
    return `${protocol}//${host}:8080`;
  }
  return 'http://localhost:8080';
};

const buildBaseUrl = () => {
  const explicit = process.env.REACT_APP_BASE_API;
  if (explicit) {
    return trimTrailingSlash(explicit);
  }
  if (process.env.NODE_ENV === 'development') {
    return `${resolveDevBackendOrigin()}${API_PREFIX}`;
  }
  return API_PREFIX;
};

export const API_BASE_URL = buildBaseUrl();
export const AUTH_BASE_URL = API_BASE_URL;
// @deprecated AUTH_PROVIDER 是历史遗留: 后端 provider 由 SERVER_AUTH_PROVIDER
// 决定, 前端不应该再编译期固定. 新代码请用 api/system/auth.js#listProviders()
// 在运行时拉取激活 provider 信息. 保留 export 仅供 api/system/auth.js#getCurrentUser
// 仍然拼 `/auth/{provider}/me` 这一处使用, 后续会迁移走.
export const AUTH_PROVIDER = process.env.REACT_APP_AUTH_PROVIDER || 'gitlab';

const REQUEST_TIMEOUT = Number(process.env.REACT_APP_REQUEST_TIMEOUT) || 15000;

const service = axios.create({
  baseURL: API_BASE_URL,
  timeout: REQUEST_TIMEOUT
});

service.interceptors.request.use(
  config => {
    const token = getToken();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  error => Promise.reject(error)
);

service.interceptors.response.use(
  response => {
    const newToken = response.headers?.token;
    if (newToken) {
      setToken(newToken);
    }
    return response.data;
  },
  error => {
    const { response } = error || {};
    if (response?.status === 401) {
      clearToken();
      if (!window.location.hash.includes('#/login')) {
        const params = new URLSearchParams({ error: '请先登录' });
        window.location.hash = `#/login?${params.toString()}`;
      }
      return Promise.reject(error);
    }

    const isLoginPage = window.location.hash.includes('#/login');
    if (!isLoginPage) {
      // 区分网络错误与业务错误, 用固定 key 让 antd message 替换同 key
      // 的旧 toast, 避免后端不在线 / 弱网时 N 个并发请求各自弹一条.
      if (!response) {
        message.error({
          key: 'backend-unreachable',
          content: '后端服务不可用，请确认后端已启动 (make run)',
          duration: 3
        });
      } else {
        let errorMessage = '请求失败';
        if (response.data) {
          if (typeof response.data === 'string') {
            errorMessage = response.data;
          } else if (response.data.message) {
            errorMessage = response.data.message;
          } else if (response.data.error) {
            errorMessage = response.data.error;
          }
        } else if (error.message) {
          errorMessage = error.message;
        }
        message.error({
          key: `api-error-${response.status}`,
          content: errorMessage,
          duration: 3
        });
      }
    }
    return Promise.reject(error);
  }
);

export default service;
