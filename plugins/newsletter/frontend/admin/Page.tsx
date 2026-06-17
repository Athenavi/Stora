'use client';

import React, {useState} from 'react';
import {useQuery, useMutation, useQueryClient} from '@tanstack/react-query';
import {AdminShell} from '@/components/admin/AdminShell';
import {apiClient} from '@/lib/api/base-client';
import {Mail, Users, UserCheck, UserX} from 'lucide-react';

const PLUGIN_SLUG = 'newsletter';

function call(action: string, params: any = {}) {
  return apiClient.post(`/plugins/${PLUGIN_SLUG}/action`, {action, params});
}

export default function NewsletterAdmin() {
  const qc = useQueryClient();
  const [page, setPage] = useState(1);

  const {data: subscribers = {data: [], total: 0}} = useQuery({
    queryKey: ['nl-subs', page],
    queryFn: async () => { const r = await call('list_subscribers', {page, per_page: 50}); return r.data || {data: [], total: 0}; },
    placeholderData: (prev: any) => prev || {data: [], total: 0},
  });

  const {data: stats = {total: 0, active: 0, unsubscribed: 0}} = useQuery({
    queryKey: ['nl-stats'],
    queryFn: async () => { const r = await call('stats'); return r.data || {}; },
    placeholderData: (prev: any) => prev || {total: 0, active: 0, unsubscribed: 0},
  });

  const unsubMut = useMutation({
    mutationFn: (id: number) => call('admin_unsubscribe', {subscriber_id: id}),
    onSuccess: () => { qc.invalidateQueries({queryKey: ['nl-subs']}); qc.invalidateQueries({queryKey: ['nl-stats']}); },
  });

  return (
    <AdminShell title="Newsletter 管理" actions={
      <div className="flex items-center gap-4 text-sm">
        <span className="flex items-center gap-1 text-gray-500"><Users className="w-4 h-4"/>{stats.total} 总计</span>
        <span className="flex items-center gap-1 text-green-600"><UserCheck className="w-4 h-4"/>{stats.active} 活跃</span>
        <span className="flex items-center gap-1 text-gray-400"><UserX className="w-4 h-4"/>{stats.unsubscribed} 退订</span>
      </div>
    }>
      {stats.total === 0 ? (
        <div className="text-center py-16 text-gray-400">
          <Mail className="w-12 h-12 mx-auto mb-4 opacity-50"/>
          <p className="text-lg font-medium text-gray-500 dark:text-gray-400">暂无订阅者</p>
          <p className="text-sm mt-1">首页订阅表单收集的邮箱会显示在这里</p>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-900 rounded-xl border overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b text-xs text-gray-500 uppercase">
                <th className="text-left px-4 py-3 font-medium">Email</th>
                <th className="text-left px-4 py-3 font-medium">名称</th>
                <th className="text-left px-4 py-3 font-medium hidden sm:table-cell">来源</th>
                <th className="text-left px-4 py-3 font-medium hidden md:table-cell">订阅时间</th>
                <th className="text-center px-4 py-3 font-medium">状态</th>
                <th className="text-right px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {(subscribers.data || []).map((sub: any) => (
                <tr key={sub.id} className="border-b hover:bg-gray-50 dark:hover:bg-gray-800/30 transition-colors">
                  <td className="px-4 py-3 text-sm text-gray-900 dark:text-white">{sub.email}</td>
                  <td className="px-4 py-3 text-sm text-gray-500">{sub.name || '-'}</td>
                  <td className="px-4 py-3 text-sm text-gray-500 hidden sm:table-cell">{sub.source}</td>
                  <td className="px-4 py-3 text-sm text-gray-500 hidden md:table-cell">{sub.subscribed_at ? new Date(sub.subscribed_at).toLocaleDateString() : '-'}</td>
                  <td className="px-4 py-3 text-center">
                    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${sub.is_active ? 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400' : 'bg-gray-100 text-gray-500'}`}>
                      {sub.is_active ? '活跃' : '已退订'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    {sub.is_active && (
                      <button onClick={() => unsubMut.mutate(sub.id)} disabled={unsubMut.isPending}
                        className="px-2 py-1 text-xs text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition">
                        {unsubMut.isPending ? '...' : '退订'}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {(subscribers.total || 0) > 50 && (
            <div className="flex items-center justify-between px-4 py-3 border-t">
              <span className="text-sm text-gray-500">{subscribers.total} 总计</span>
              <div className="flex gap-2">
                <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}
                  className="px-3 py-1 text-sm border rounded-lg disabled:opacity-50">上一页</button>
                <button disabled={(subscribers.data || []).length < 50} onClick={() => setPage(p => p + 1)}
                  className="px-3 py-1 text-sm border rounded-lg disabled:opacity-50">下一页</button>
              </div>
            </div>
          )}
        </div>
      )}
    </AdminShell>
  );
}
