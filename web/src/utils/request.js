import axios from 'axios';
import { message } from 'antd';
import { getToken, setToken, clearToken } from './auth';

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
      let errorMessage = '请求失败';
      if (response?.data) {
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
      message.error(errorMessage);
    }
    return Promise.reject(error);
  }
);

export default service;
