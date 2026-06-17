/**
 * 用户认证/资料 API
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

export interface RegisterParams {
  username: string;
  email: string;
  password: string;
  password_confirm: string;
}

export const login = async (
  username: string,
  password: string,
): Promise<LoginResponse> => {
  const fd = new FormData();
  fd.append('username', username);
  fd.append('password', password);
  const res = await api.post<LoginResponse>('/auth/login', fd);
  setToken(res.access_token);
  return res;
};

export const register = (params: RegisterParams): Promise<User> =>
  api.post('/auth/register', params);

export const logout = (): Promise<void> => api.post('/auth/logout');

export const getProfile = (): Promise<User> => api.get('/auth/me');

export const getQuota = (): Promise<{
  max_storage: number;
  used_storage: number;
  max_file_size: number;
  max_files_count: number;
  files_count: number;
  usage_percent: number;
}> => api.get('/users/me/quota');
