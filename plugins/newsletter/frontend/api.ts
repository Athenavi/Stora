import {apiClient} from '@/lib/api/base-client';

const PLUGIN_SLUG = 'newsletter';
const ACTION_URL = `/plugins/${PLUGIN_SLUG}/action`;

export interface Subscriber {
  id: number;
  email: string;
  name: string;
  source: string;
  is_active: boolean;
  subscribed_at: string;
  unsubscribed_at: string | null;
}

export interface SubListResult {
  data: Subscriber[];
  total: number;
  page: number;
  per_page: number;
}

export interface Stats {
  success: boolean;
  total: number;
  active: number;
  unsubscribed: number;
}

async function callAction(action: string, params: Record<string, any> = {}) {
  const res = await apiClient.post(ACTION_URL, {action, params});
  return res.success ? (res.data || {}) : {success: false, error: res.error || 'Request failed'};
}

export class NewsletterService {
  static subscribe(email: string, name = '') {
    return callAction('subscribe', {email, name});
  }

  static unsubscribe(email: string) {
    return callAction('unsubscribe', {email});
  }

  static list(page = 1, perPage = 50): Promise<SubListResult> {
    return callAction('list_subscribers', {page, per_page: perPage}).then(r => r as any);
  }

  static stats(): Promise<Stats> {
    return callAction('stats', {}).then(r => r as any);
  }
}
