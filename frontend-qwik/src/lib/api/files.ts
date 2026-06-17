/**
 * 文件/文件夹 API
 */
import { api } from './api-client';

// ─── Types ───

export interface FileItem {
  id: number;
  filename: string;
  original_filename?: string;
  file_size: number;
  mime_type?: string;
  file_type: string;
  folder_id?: number | null;
  is_favorite: boolean;
  file_hash?: string;
  thumbnail_url?: string;
  file_url?: string;
  description?: string;
  download_count: number;
  width?: number;
  height?: number;
  duration?: number;
  created_at?: string;
  updated_at?: string;
}

export interface Folder {
  id: number;
  name: string;
  parent_id?: number | null;
  color?: string;
  icon?: string;
  is_shared: boolean;
  sort_order: number;
  created_at?: string;
  children?: Folder[];
}

export interface FileListResponse {
  items: FileItem[];
  total: number;
  page: number;
  page_size: number;
}

export interface FolderChildrenResponse {
  folders: Folder[];
  files: FileItem[];
  path: { id: number; name: string }[];
}

// ─── File CRUD ───

export const listFiles = (params?: {
  folder_id?: number | null;
  search?: string;
  file_type?: string;
  sort_by?: string;
  sort_order?: string;
  page?: number;
  page_size?: number;
}): Promise<FileListResponse> => {
  const q = new URLSearchParams();
  if (params?.folder_id !== undefined) q.set('folder_id', String(params.folder_id));
  if (params?.search) q.set('search', params.search);
  if (params?.file_type) q.set('file_type', params.file_type);
  if (params?.sort_by) q.set('sort_by', params.sort_by);
  if (params?.sort_order) q.set('sort_order', params.sort_order);
  q.set('page', String(params?.page || 1));
  q.set('page_size', String(params?.page_size || 50));
  return api.get(`/files?${q.toString()}`);
};

export const getFile = (id: number): Promise<FileItem> =>
  api.get(`/files/${id}`);

export const updateFile = (id: number, data: {
  filename?: string;
  description?: string;
  is_favorite?: boolean;
}): Promise<FileItem> =>
  api.patch(`/files/${id}`, data);

export const deleteFile = (id: number, permanent = false): Promise<void> =>
  api.delete(`/files/${id}?permanent=${permanent}`);

// ─── Folder CRUD ───

export const createFolder = (name: string, parentId?: number | null): Promise<Folder> =>
  api.post('/files/folders', { name, parent_id: parentId });

export const updateFolder = (id: number, name: string): Promise<Folder> =>
  api.patch(`/files/folders/${id}`, { name });

export const deleteFolder = (id: number, recursive = false): Promise<void> =>
  api.delete(`/files/folders/${id}?recursive=${recursive}`);

export const getFolderChildren = (id: number): Promise<FolderChildrenResponse> =>
  api.get(`/files/folders/${id}/children`);

export const getFolderTree = (): Promise<Folder[]> =>
  api.get('/files/folders/tree');

// ─── Move ───

export const moveFiles = (fileIds: number[], targetFolderId?: number | null): Promise<void> =>
  api.post('/files/move', { file_ids: fileIds, target_folder_id: targetFolderId });

// ─── Upload ───

export const uploadFile = async (
  file: File,
  folderId?: number | null,
  onProgress?: (pct: number) => void,
): Promise<{ file: FileItem; instant: boolean }> => {
  const fd = new FormData();
  fd.append('file', file);
  if (folderId !== undefined && folderId !== null) {
    fd.append('folder_id', String(folderId));
  }
  return api.upload('/files/upload', fd, onProgress);
};

export const checkUpload = (fileHash: string, filename: string, fileSize: number) =>
  api.get<{ exists: boolean; size?: number; mime_type?: string }>(
    `/files/upload/check?file_hash=${fileHash}&filename=${encodeURIComponent(filename)}&file_size=${fileSize}`
  );

export const initChunkedUpload = (data: {
  filename: string;
  total_size: number;
  total_chunks: number;
  file_hash?: string;
  folder_id?: number | null;
}): Promise<{ upload_id: string; chunk_size: number }> =>
  api.post('/files/upload/init', data);

export const uploadChunk = (uploadId: string, chunkIndex: number, chunk: Blob) => {
  const fd = new FormData();
  fd.append('upload_id', uploadId);
  fd.append('chunk_index', String(chunkIndex));
  fd.append('chunk', chunk);
  return api.post('/files/upload/chunk', fd);
};

export const completeUpload = (
  uploadId: string,
  folderId?: number | null,
): Promise<{ file: FileItem }> =>
  api.post('/files/upload/complete', { upload_id: uploadId, folder_id: folderId });

// ─── Download ───

export const getDownloadToken = (fileId: number): Promise<{ token: string; expires_in: number }> =>
  api.get(`/files/download/token/${fileId}`);
