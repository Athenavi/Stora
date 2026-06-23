/**
 * 分享链接 API
 */
import { api } from './api-client';

export interface ShareLink {
  id: number;
  short_code: string;
  url?: string;
  permission: string;
  is_active: boolean;
  password_protected: boolean;
  view_count: number;
  download_count: number;
  max_downloads: number;
  expires_at?: string | null;
  created_at?: string;
}

export interface CreateShareParams {
  file_id?: number | null;
  folder_id?: number | null;
  permission?: string;
  password?: string;
  expires_in_hours?: number | null;
  max_downloads?: number;
}

export interface ShareListResponse {
  items: ShareLink[];
  total: number;
  page: number;
  page_size: number;
}

export interface ShareAccessResponse {
  share_info: ShareLink;
  item: any;
  need_password?: boolean;
  protected?: boolean;
  password_protected: boolean;
}

export const createShare = (params: CreateShareParams & { file_ids?: number[]; folder_ids?: number[] }): Promise<ShareLink> => {
  if (params.file_ids || params.folder_ids) {
    // Batch/mixed share — use JSON so arrays are preserved
    const body: any = {
      permission: params.permission || 'read',
      password: params.password || undefined,
      expires_in_hours: params.expires_in_hours || undefined,
      max_downloads: params.max_downloads || 0,
    };
    if (params.file_ids) body.file_ids = params.file_ids;
    if (params.folder_ids) body.folder_ids = params.folder_ids;
    return api.post('/files/shares', body);
  }
  // Single share — use FormData for backward compat
  const fd = new FormData();
  if (params.file_id !== undefined) fd.append('file_id', String(params.file_id));
  if (params.folder_id !== undefined) fd.append('folder_id', String(params.folder_id));
  fd.append('permission', params.permission || 'read');
  if (params.password) fd.append('password', params.password);
  if (params.expires_in_hours) fd.append('expires_in_hours', String(params.expires_in_hours));
  fd.append('max_downloads', String(params.max_downloads || 0));
  return api.post('/files/shares', fd);
};

export const listShares = (page = 1, pageSize = 20): Promise<ShareListResponse> =>
  api.get(`/files/shares?page=${page}&page_size=${pageSize}`);

export const revokeShare = (linkId: number): Promise<void> =>
  api.delete(`/files/shares/${linkId}`);

export const updateShare = (linkId: number, params: Partial<CreateShareParams>): Promise<void> =>
  api.put(`/files/shares/${linkId}`, params);

export const shareWithUser = (fileId: number, sharedWith: number, permission = 'view'): Promise<any> =>
  api.post('/files/shares/share-with-user', { file_id: fileId, shared_with: sharedWith, permission });

export const accessShare = (
  shortCode: string,
  password?: string,
): Promise<ShareAccessResponse> => {
  let q = `/files/shares/access/${shortCode}`;
  if (password) q += `?password=${encodeURIComponent(password)}`;
  return api.get(q);
};
