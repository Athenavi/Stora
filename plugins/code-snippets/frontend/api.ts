/**
 * API service for code-snippets plugin.
 * Communicates with the plugin through the generic plugin action endpoint.
 */

import {apiClient} from '@/lib/api/base-client';

const PLUGIN_SLUG = 'code-snippets';
const ACTION_URL = `/plugins/${PLUGIN_SLUG}/action`;

export interface Snippet {
  id: number;
  title: string;
  code: string;
  language: string;
  description: string;
  tags: string[];
  visibility: 'public' | 'private' | 'unlisted';
  user_id: number;
  view_count: number;
  embed_count: number;
  created_at: string;
  updated_at: string;
}

export interface SnippetInput {
  title: string;
  code: string;
  language?: string;
  description?: string;
  tags?: string[];
  visibility?: string;
}

export class SnippetService {
  /** 获取用户的全部代码片段 */
  static async list(userId: number): Promise<Snippet[]> {
    const res = await apiClient.post(ACTION_URL, {
      action: 'get_user_snippets',
      params: {user_id: userId, limit: 100, offset: 0},
    });
    if (res.success && res.data) return Array.isArray(res.data) ? res.data : (res.data.data || []);
    return [];
  }

  /** 创建代码片段 */
  static async create(input: SnippetInput, userId: number): Promise<Snippet | null> {
    const res = await apiClient.post(ACTION_URL, {
      action: 'create_snippet',
      params: {...input, user_id: userId},
    });
    if (res.success && res.data) return res.data.data || res.data;
    return null;
  }

  /** 更新代码片段 */
  static async update(id: number, input: Partial<SnippetInput>, userId: number): Promise<boolean> {
    const res = await apiClient.post(ACTION_URL, {
      action: 'update_snippet',
      params: {snippet_id: id, updates: input, user_id: userId},
    });
    return res.success;
  }

  /** 删除代码片段 */
  static async delete(id: number, userId: number): Promise<boolean> {
    const res = await apiClient.post(ACTION_URL, {
      action: 'delete_snippet',
      params: {snippet_id: id, user_id: userId},
    });
    return res.success;
  }

  /** 搜索公开代码片段 */
  static async search(query: string, language?: string): Promise<Snippet[]> {
    const res = await apiClient.post(ACTION_URL, {
      action: 'search_snippets',
      params: {query, language, limit: 50},
    });
    if (res.success && res.data) return Array.isArray(res.data) ? res.data : [];
    return [];
  }

  /** 获取热门片段 */
  static async popular(): Promise<Snippet[]> {
    const res = await apiClient.post(ACTION_URL, {
      action: 'get_popular_snippets',
      params: {limit: 10},
    });
    if (res.success && res.data) return Array.isArray(res.data) ? res.data : [];
    return [];
  }
}
