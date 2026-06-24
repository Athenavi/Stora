/**
 * Stora 用户认证/资料 API
 */
import { api, setToken } from './api-client';

export interface User {
  id: number;
  username: string;
  email: string;
  profile_picture?: string;
  bio?: string;
  is_active: boolean;
  is_superuser: boolean;
  date_joined?: string;
  last_login_at?: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token?: string;
  token_type: string;
  expires_in: number;
  user: User;
}

/** 登录 */
export const login = async (username: string, password: string): Promise<LoginResponse> => {
  const fd = new FormData();
  fd.append('username', username);
  fd.append('password', password);
  const res = await api.post<LoginResponse>('/auth/login', fd);
  setToken(res.access_token);
  return res;
};

/** 发送验证码 */
export const sendCode = (email: string) => {
  const fd = new FormData();
  fd.append('email', email);
  return api.post('/auth/send-code', fd);
};

/** 验证码登录 */
export const loginWithCode = async (email: string, code: string): Promise<LoginResponse> => {
  const fd = new FormData();
  fd.append('email', email);
  fd.append('code', code);
  const res = await api.post<LoginResponse>('/auth/login-with-code', fd);
  setToken(res.access_token);
  return res;
};

/** 注册 */
export const register = (params: {
  username: string;
  email: string;
  password: string;
  password_confirm: string;
}) => {
  const fd = new FormData();
  fd.append('username', params.username);
  fd.append('email', params.email);
  fd.append('password', params.password);
  fd.append('password_confirm', params.password_confirm);
  return api.post('/auth/register', fd);
};

/** 登出 */
export const logout = (): Promise<void> => api.post('/auth/logout');

/** 获取当前用户信息 */
export const getProfile = (): Promise<User> => api.get('/auth/me');

/** 更新用户资料 */
export const updateProfile = (data: { username?: string; email?: string; bio?: string }): Promise<User> =>
  api.patch('/auth/me', data);

/** 修改密码 */
export const changePassword = (data: { current_password: string; new_password: string; new_password_confirm: string }): Promise<void> =>
  api.post('/auth/change-password', data);

/** 获取存储配额 */
export const getQuota = () => api.get('/users/me/quota');
