'use client';

import React, {useState} from 'react';
import {useQuery, useMutation, useQueryClient} from '@tanstack/react-query';
import {AdminShell} from '@/components/admin/AdminShell';
import {apiClient} from '@/lib/api/base-client';
import {RefreshCw, ScrollText, Loader2, Trash2} from 'lucide-react';

const PLUGIN_SLUG = 'migration';

function call(action: string, params: any = {}) {
  return apiClient.post(`/plugins/${PLUGIN_SLUG}/action`, {action, params});
}

export default function MigrationPage() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<'tasks' | 'logs'>('tasks');
  const [page, setPage] = useState(1);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({name: '', source_platform: 'wordpress'});

  const {data: tasks, isLoading: loadT} = useQuery({
    queryKey: ['mg-tasks', page],
    queryFn: async () => { const r = await call('list_tasks', {page, per_page: 20}); return r.data || {items: [], total: 0}; },
    enabled: tab === 'tasks',
  });
  const {data: logs, isLoading: loadL} = useQuery({
    queryKey: ['mg-logs', page],
    queryFn: async () => { const r = await call('list_logs', {page, per_page: 20}); return r.data || {items: [], total: 0}; },
    enabled: tab === 'logs',
  });

  const createMut = useMutation({
    mutationFn: () => call('create_task', {name: form.name, source_platform: form.source_platform}),
    onSuccess: () => { qc.invalidateQueries({queryKey: ['mg-tasks']}); setShowCreate(false); setForm({name: '', source_platform: 'wordpress'}); },
  });
  const deleteMut = useMutation({
    mutationFn: (id: number) => call('delete_task', {id}),
    onSuccess: () => qc.invalidateQueries({queryKey: ['mg-tasks']}),
  });

  return (
    <AdminShell title="迁移管理" actions={tab === 'tasks' ? (
      <button onClick={() => setShowCreate(true)} className="px-4 py-2 bg-blue-600 text-white text-sm rounded-lg hover:bg-blue-700">新建任务</button>
    ) : undefined}>
      <div className="flex gap-2 mb-6">
        {[
          {key: 'tasks' as const, label: '迁移任务', icon: RefreshCw},
          {key: 'logs' as const, label: '迁移日志', icon: ScrollText},
        ].map(t => (
          <button key={t.key} onClick={() => { setTab(t.key); setPage(1); }}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition flex items-center gap-1.5 ${
              tab === t.key ? 'bg-blue-600 text-white' : 'bg-white dark:bg-gray-900 border hover:bg-gray-50 dark:hover:bg-gray-800'
            }`}><t.icon className="w-4 h-4"/>{t.label}</button>
        ))}
      </div>

      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-white dark:bg-gray-900 rounded-2xl max-w-lg w-full p-6 space-y-4">
            <h3 className="font-semibold text-lg">新建迁移任务</h3>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">任务名称</label>
              <input value={form.name} onChange={e => setForm({...form, name: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm bg-white dark:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500"/>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">源平台</label>
              <select value={form.source_platform} onChange={e => setForm({...form, source_platform: e.target.value})}
                className="w-full px-3 py-2 border rounded-lg text-sm bg-white dark:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500">
                <option value="wordpress">WordPress</option>
                <option value="halo">Halo</option><option value="ghost">Ghost</option>
              </select>
            </div>
            <div className="flex gap-3 justify-end pt-2">
              <button onClick={() => setShowCreate(false)} className="px-4 py-2 border rounded-lg text-sm hover:bg-gray-50 dark:hover:bg-gray-800">取消</button>
              <button onClick={() => createMut.mutate()} disabled={createMut.isPending || !form.name}
                className="px-4 py-2 bg-blue-600 text-white text-sm rounded-lg hover:bg-blue-700 disabled:opacity-50">{createMut.isPending ? '创建中...' : '创建'}</button>
            </div>
          </div>
        </div>
      )}

      {(tab === 'tasks' ? loadT : loadL) ? (
        <div className="flex justify-center py-16"><Loader2 className="w-8 h-8 animate-spin text-blue-500"/></div>
      ) : (
        <div className="bg-white dark:bg-gray-900 rounded-xl border overflow-hidden">
          {(!(tab === 'tasks' ? tasks : logs)?.items?.length) ? (
            <div className="py-16 text-center text-gray-400"><RefreshCw className="w-10 h-10 mx-auto mb-3 opacity-40"/><p className="text-sm">暂无数据</p></div>
          ) : (
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-800 border-b">
                <tr>{tab === 'tasks' ? (
                  <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">名称</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left hidden sm:table-cell">平台</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">进度</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-right">操作</th></>
                ) : (
                  <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">任务</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left hidden sm:table-cell">级别</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-right">时间</th></>
                )}</tr>
              </thead>
              <tbody className="divide-y dark:divide-gray-800">
                {((tab === 'tasks' ? tasks : logs)?.items || []).map((item: any, i: number) => (
                  <tr key={item.id || i} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    {tab === 'tasks' ? (<>
                      <td className="px-5 py-4 text-sm font-medium text-gray-900 dark:text-white">{item.name || `#${item.id}`}</td>
                      <td className="px-5 py-4 text-sm text-gray-500 hidden sm:table-cell">{item.source_platform || '—'}</td>
                      <td className="px-5 py-4"><div className="flex items-center gap-2"><div className="flex-1 h-2 bg-gray-200 rounded-full overflow-hidden"><div className="h-full bg-blue-500 rounded-full" style={{width: `${item.progress || 0}%`}}/></div><span className="text-xs text-gray-500">{item.progress || 0}%</span></div></td>
                      <td className="px-5 py-4 text-right"><button onClick={() => deleteMut.mutate(item.id)} className="p-1.5 text-gray-400 hover:text-red-500"><Trash2 className="w-4 h-4"/></button></td>
                    </>) : (<>
                      <td className="px-5 py-4 text-sm text-gray-900 dark:text-white">{item.task_name || `#${item.task_id}`}</td>
                      <td className="px-5 py-4 hidden sm:table-cell"><span className={`px-2 py-0.5 text-xs rounded-full ${item.level === 'error' ? 'bg-red-100 text-red-700' : item.level === 'warning' ? 'bg-yellow-100 text-yellow-700' : 'bg-gray-100 text-gray-500'}`}>{item.level || 'info'}</span></td>
                      <td className="px-5 py-4 text-sm text-gray-500 text-right">{item.created_at ? new Date(item.created_at).toLocaleString() : '—'}</td>
                    </>)}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </AdminShell>
  );
}
