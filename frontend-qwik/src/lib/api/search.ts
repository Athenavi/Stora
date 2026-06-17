/**
 * 文件搜索 API
 */
import { api } from './api-client';
import type { FileItem } from './files';

export interface SearchResponse {
  items: FileItem[];
  total: number;
  page: number;
  page_size: number;
}

export interface SearchHistoryItem {
  keyword: string;
  results_count: number;
  created_at: string;
}

export const searchFiles = (params: {
  q: string;
  file_type?: string;
  sort_by?: string;
  sort_order?: string;
  page?: number;
  page_size?: number;
}): Promise<SearchResponse> => {
  const qs = new URLSearchParams();
  qs.set('q', params.q);
  if (params.file_type) qs.set('file_type', params.file_type);
  if (params.sort_by) qs.set('sort_by', params.sort_by);
  qs.set('page', String(params.page || 1));
  qs.set('page_size', String(params.page_size || 50));
  return api.get(`/files/search?${qs.toString()}`);
};

export const searchHistory = (limit = 10): Promise<SearchHistoryItem[]> =>
  api.get(`/files/search/history?limit=${limit}`);
