/**
 * Stora API 客户端 — 基础封装
 *
 * 自动管理 JWT token 的存储和请求头附加。
 * baseURL 默认为 /api/v2，可通过环境变量 API_BASE_URL 覆盖。
 */

const BASE_URL = (typeof window !== 'undefined'
  ? (import.meta as any).env?.VITE_API_BASE_URL
  : '') || '/api/v2';

let authToken: string | null = null;

/** 设置 JWT token（登录成功后由后端 Set-Cookie 处理，仅做内存缓存） */
export function setToken(token: string) {
  authToken = token;
}

/** 清除 token（登出时调用） */
export function clearToken() {
  authToken = null;
  // 清除浏览器 cookie
  if (typeof document !== 'undefined') {
    document.cookie = 'access_token=; Path=/; Max-Age=0; SameSite=Lax;';
  }
}

/** 从 cookie 读取 token（客户端浏览器环境） */
function getTokenFromCookie(): string | null {
  if (typeof document === 'undefined') return null;
  return parseCookie(document.cookie, 'access_token');
}

/** 获取 token：优先内存缓存，回退到 cookie */
function loadToken(): string | null {
  return authToken || getTokenFromCookie();
}

/** 判断是否已登录 */
export function isAuthenticated(): boolean {
  return !!loadToken();
}

// ─── 请求封装 ───

export interface ApiResponse<T = any> {
  success: boolean;
  data?: T;
  message?: string;
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public data?: any
  ) {
    super(message);
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: any,
  options?: { headers?: Record<string, string> }
): Promise<T> {
  const url = `${BASE_URL}${path}`;
  const headers: Record<string, string> = {
    ...options?.headers,
  };

  const token = authToken || loadToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  let fetchBody: any = undefined;
  if (body instanceof FormData) {
    fetchBody = body;
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    fetchBody = JSON.stringify(body);
  }

  const res = await fetch(url, {
    method,
    headers,
    body: fetchBody,
  });

  const json: ApiResponse<T> = await res.json();

  if (!res.ok || !json.success) {
    throw new ApiError(res.status, json.message || '请求失败', json.data);
  }

  return json.data as T;
}

/** 从 cookie 字符串中解析指定名称的值 */
function parseCookie(cookie: string, name: string): string | null {
  for (const part of cookie.split(';')) {
    const [k, v] = part.trim().split('=');
    if (k === name) return v || null;
  }
  return null;
}

/**
 * 创建 SSR 环境下的 API 客户端
 * 在 Qwik routeLoader$ 中使用，从浏览器请求中提取 cookie 并转发到后端。
 * 客户端导航时 request 为 undefined，回退到普通 api。
 */
export function createServerApi(request?: Request) {
  // 客户端导航时没有 request 对象，回退到普通 api
  if (!request) {
    return api;
  }

  const cookie = request.headers.get('cookie') || '';
  const token = parseCookie(cookie, 'access_token');
  // 使用请求的 origin 作为 API 请求的 base URL（Node.js fetch 需要绝对 URL）
  const origin = new URL(request.url).origin;

  async function serverRequest<T>(method: string, path: string, body?: any): Promise<T> {
    // SSR 期间使用完整 URL，通过 Vite 代理转发到后端
    const url = `${origin}/api/v2${path}`;
    const headers: Record<string, string> = {
      'Cookie': cookie,
    };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    let fetchBody: any = undefined;
    if (body instanceof FormData) {
      fetchBody = body;
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      fetchBody = JSON.stringify(body);
    }

    const res = await fetch(url, { method, headers, body: fetchBody });
    const json: ApiResponse<T> = await res.json();
    if (!res.ok || !json.success) {
      throw new ApiError(res.status, json.message || '请求失败', json.data);
    }
    return json.data as T;
  }

  return {
    get: <T>(path: string) => serverRequest<T>('GET', path),
    post: <T>(path: string, body?: any) => serverRequest<T>('POST', path, body),
    patch: <T>(path: string, body?: any) => serverRequest<T>('PATCH', path, body),
    delete: <T>(path: string) => serverRequest<T>('DELETE', path),
    upload: <T>(path: string, formData: FormData) => serverRequest<T>('POST', path, formData),
  };
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: any) => request<T>('POST', path, body),
  patch: <T>(path: string, body?: any) => request<T>('PATCH', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),

  /** 上传文件（FormData） */
  upload: <T>(path: string, formData: FormData, onProgress?: (pct: number) => void) => {
    return request<T>('POST', path, formData, {});
  },
};
