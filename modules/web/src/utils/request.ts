import { message } from 'antd';
import { getToken, setToken, clearToken } from './auth';

message.config({ maxCount: 3 });

const API_PREFIX = process.env.REACT_APP_API_PREFIX || '/api/v1';

const trimTrailingSlash = (value: string) => (value || '').replace(/\/+$/, '');

const resolveDevBackendOrigin = (): string => {
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

const buildBaseUrl = (): string => {
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
export const AUTH_PROVIDER = process.env.REACT_APP_AUTH_PROVIDER || 'gitlab';

const REQUEST_TIMEOUT = Number(process.env.REACT_APP_REQUEST_TIMEOUT) || 15000;

export type RequestMethod = 'get' | 'post' | 'put' | 'patch' | 'delete' | 'head' | 'options';

export interface RequestConfig {
  url: string;
  method?: RequestMethod | string;
  data?: unknown;
  params?: Record<string, unknown>;
  headers?: Record<string, string>;
}

function buildQueryString(params: Record<string, unknown>): string {
  const usp = new URLSearchParams();
  for (const [key, raw] of Object.entries(params)) {
    if (raw === undefined || raw === null) {
      continue;
    }
    if (Array.isArray(raw)) {
      usp.set(key, raw.map(String).join(','));
    } else if (typeof raw === 'object') {
      usp.set(key, String(raw));
    } else {
      usp.set(key, String(raw));
    }
  }
  return usp.toString();
}

function joinUrl(base: string, path: string, params?: Record<string, unknown>): string {
  const p = path.startsWith('/') ? path : `/${path}`;
  const qs = params ? buildQueryString(params) : '';
  const withQuery = qs ? (p.includes('?') ? `${p}&${qs}` : `${p}?${qs}`) : p;
  const baseTrim = base.replace(/\/+$/, '');
  return `${baseTrim}${withQuery}`;
}

export interface RequestErrorShape {
  response?: {
    status: number;
    data?: unknown;
  };
  message?: string;
}

async function parseSuccessBody(response: Response): Promise<unknown> {
  if (response.status === 204) {
    return undefined;
  }
  const len = response.headers.get('content-length');
  if (len === '0') {
    return undefined;
  }
  const ct = response.headers.get('content-type') || '';
  const text = await response.text();
  if (!text) {
    return undefined;
  }
  if (ct.includes('application/json')) {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return text;
    }
  }
  return text;
}

async function parseErrorBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) {
    return undefined;
  }
  const ct = response.headers.get('content-type') || '';
  if (ct.includes('application/json')) {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return text;
    }
  }
  return text;
}

function applyResponseErrorHandling(
  status: number,
  data: unknown,
  fallbackMessage: string
): never {
  if (status === 401) {
    clearToken();
    if (!window.location.hash.includes('#/login')) {
      const params = new URLSearchParams({ error: '请先登录' });
      window.location.hash = `#/login?${params.toString()}`;
    }
    const err: RequestErrorShape = { response: { status, data } };
    throw err;
  }

  const isLoginPage = window.location.hash.includes('#/login');
  if (!isLoginPage) {
    let errorMessage = '请求失败';
    if (data !== undefined && data !== null) {
      if (typeof data === 'string') {
        errorMessage = data;
      } else if (typeof data === 'object') {
        const o = data as Record<string, unknown>;
        if (typeof o.message === 'string') {
          errorMessage = o.message;
        } else if (typeof o.error === 'string') {
          errorMessage = o.error;
        }
      }
    } else if (fallbackMessage) {
      errorMessage = fallbackMessage;
    }
    message.error({
      key: `api-error-${status}`,
      content: errorMessage,
      duration: 3
    });
  }
  const err: RequestErrorShape = { response: { status, data }, message: fallbackMessage };
  throw err;
}

export default async function request<T = unknown>(config: RequestConfig): Promise<T> {
  const method = (config.method || 'get').toUpperCase();
  const url = joinUrl(API_BASE_URL, config.url, config.params);

  const headers: Record<string, string> = { ...(config.headers || {}) };
  const token = getToken();
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  let body: BodyInit | undefined;
  if (method !== 'GET' && method !== 'HEAD' && config.data !== undefined) {
    if (config.data instanceof FormData) {
      body = config.data;
    } else {
      if (!headers['Content-Type'] && !headers['content-type']) {
        headers['Content-Type'] = 'application/json';
      }
      body = JSON.stringify(config.data);
    }
  }

  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), REQUEST_TIMEOUT);

  try {
    const response = await fetch(url, {
      method,
      headers,
      body,
      signal: controller.signal
    });
    window.clearTimeout(timer);

    const newToken = response.headers.get('token');
    if (newToken) {
      setToken(newToken);
    }

    if (!response.ok) {
      const data = await parseErrorBody(response);
      applyResponseErrorHandling(response.status, data, response.statusText);
    }

    const parsed = (await parseSuccessBody(response)) as T;
    return parsed;
  } catch (error: unknown) {
    window.clearTimeout(timer);

    const err = error as RequestErrorShape & { name?: string };
    if (err?.response?.status) {
      throw error;
    }

    const isLoginPage = window.location.hash.includes('#/login');
    if (!isLoginPage) {
      if (err?.name === 'AbortError') {
        message.error({
          key: 'backend-unreachable',
          content: '请求超时，请稍后重试',
          duration: 3
        });
      } else {
        message.error({
          key: 'backend-unreachable',
          content: '后端服务不可用，请确认后端已启动 (make run)',
          duration: 3
        });
      }
    }
    throw error;
  }
}
