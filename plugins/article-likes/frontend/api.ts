import {apiClient} from '@/lib/api/base-client';

const PLUGIN_SLUG = 'article-likes';
const ACTION_URL = `/plugins/${PLUGIN_SLUG}/action`;

async function call(action: string, params: Record<string, any> = {}) {
  const res = await apiClient.post(ACTION_URL, {action, params});
  return res.success ? (res.data || {}) : {success: false, error: res.error || 'Request failed'};
}

export class ArticleLikesService {
  static like(articleId: number, userId: number) {
    return call('like', {article_id: articleId, user_id: userId});
  }

  static unlike(articleId: number, userId: number) {
    return call('unlike', {article_id: articleId, user_id: userId});
  }

  static status(articleId: number, userId?: number) {
    return call('status', {article_id: articleId, user_id: userId || 0});
  }

  static popular(limit = 10) {
    return call('popular', {limit});
  }
}
