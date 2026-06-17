'use client';

import React, {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {AdminShell} from '@/components/admin/AdminShell';
import {apiClient} from '@/lib/api/base-client';
import {BarChart3, Shield, Ticket, Terminal, ScrollText, Bell, Loader2} from 'lucide-react';

const PLUGIN_SLUG = 'enterprise';

function call(action: string, params: any = {}) {
  return apiClient.post(`/plugins/${PLUGIN_SLUG}/action`, {action, params});
}

const TABS = [
  {key: 'overview', label: '概览', icon: BarChart3},
  {key: 'licenses', label: '许可证', icon: Shield},
  {key: 'tickets', label: '工单', icon: Ticket},
  {key: 'scripts', label: '脚本', icon: Terminal},
  {key: 'logs', label: '日志', icon: ScrollText},
  {key: 'alerts', label: '告警', icon: Bell},
];

export default function EnterprisePage() {
  const [tab, setTab] = useState('overview');
  const [page, setPage] = useState(1);

  const {data: overview} = useQuery({
    queryKey: ['ent-overview'],
    queryFn: async () => { const r = await call('get_overview'); return r.data || {}; },
    enabled: tab === 'overview',
  });

  const listQuery = (key: string, action: string) => useQuery({
    queryKey: [key, page],
    queryFn: async () => { const r = await call(action, {page, per_page: 20}); return r.data || {items: [], total: 0}; },
    enabled: tab === key,
  });

  const licenses = listQuery('licenses', 'list_licenses');
  const tickets = listQuery('tickets', 'list_tickets');
  const scripts = listQuery('scripts', 'list_scripts');
  const logs = listQuery('logs', 'list_logs');
  const alerts = listQuery('alerts', 'list_alerts');

  const dataMap: Record<string, any> = {overview, licenses: licenses.data, tickets: tickets.data, scripts: scripts.data, logs: logs.data, alerts: alerts.data};
  const loadingMap: Record<string, boolean> = {licenses: licenses.isLoading, tickets: tickets.isLoading, scripts: scripts.isLoading, logs: logs.isLoading, alerts: alerts.isLoading};
  const cur = dataMap[tab];
  const loading = loadingMap[tab] || false;

  return (
    <AdminShell title="企业管理">
      <div className="flex gap-2 mb-6 flex-wrap">
        {TABS.map(t => (
          <button key={t.key} onClick={() => { setTab(t.key); setPage(1); }}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition flex items-center gap-1.5 ${
              tab === t.key ? 'bg-blue-600 text-white' : 'bg-white dark:bg-gray-900 border hover:bg-gray-50 dark:hover:bg-gray-800'
            }`}><t.icon className="w-4 h-4"/>{t.label}</button>
        ))}
      </div>

      {tab === 'overview' && overview && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[
            {label: '总许可证', value: overview.total_licenses}, {label: '活跃许可证', value: overview.active_licenses},
            {label: '待处理工单', value: overview.open_tickets}, {label: '进行中', value: overview.in_progress_tickets},
            {label: '部署脚本', value: overview.total_scripts}, {label: '部署次数', value: overview.total_deployments},
            {label: '失败部署', value: overview.failed_deployments}, {label: '未解决告警', value: overview.unresolved_alerts},
          ].map(s => (
            <div key={s.label} className="bg-white dark:bg-gray-900 rounded-xl border p-4">
              <p className="text-xs text-gray-500 dark:text-gray-400 mb-1">{s.label}</p>
              <p className="text-2xl font-bold">{s.value ?? '—'}</p>
            </div>
          ))}
        </div>
      )}

      {tab !== 'overview' && (loading ? (
        <div className="flex justify-center py-16"><Loader2 className="w-8 h-8 animate-spin text-blue-500"/></div>
      ) : !cur?.items?.length ? (
        <div className="bg-white dark:bg-gray-900 rounded-xl border py-16 text-center text-gray-400">
          <Shield className="w-10 h-10 mx-auto mb-3 opacity-40"/><p className="text-sm">暂无数据</p>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-900 rounded-xl border overflow-hidden">
          <table className="w-full">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                {tab === 'licenses' && <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">KEY</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-right">状态</th></>}
                {tab === 'tickets' && <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">标题</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left hidden sm:table-cell">状态</th></>}
                {tab === 'scripts' && <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">名称</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-right">版本</th></>}
                {tab === 'logs' && <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">脚本</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left hidden sm:table-cell">状态</th></>}
                {tab === 'alerts' && <><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left">消息</th><th className="px-5 py-3 text-xs font-semibold text-gray-500 uppercase text-left hidden sm:table-cell">级别</th></>}
              </tr>
            </thead>
            <tbody className="divide-y dark:divide-gray-800">
              {(cur.items || []).map((item: any, i: number) => (
                <tr key={item.id || i} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                  {tab === 'licenses' && <><td className="px-5 py-4 text-sm font-mono text-gray-900 dark:text-white truncate max-w-[300px]">{item.license_key || `#${item.id}`}</td><td className="px-5 py-4 text-right"><span className={`px-2 py-0.5 text-xs rounded-full ${item.is_active ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>{item.is_active ? '活跃' : '停用'}</span></td></>}
                  {tab === 'tickets' && <><td className="px-5 py-4 text-sm font-medium text-gray-900 dark:text-white">{item.title || `#${item.id}`}</td><td className="px-5 py-4 hidden sm:table-cell"><span className={`px-2 py-0.5 text-xs rounded-full ${item.status === 'open' ? 'bg-yellow-100 text-yellow-700' : item.status === 'resolved' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>{item.status || '—'}</span></td></>}
                  {tab === 'scripts' && <><td className="px-5 py-4 text-sm font-medium text-gray-900 dark:text-white">{item.name || `#${item.id}`}</td><td className="px-5 py-4 text-sm text-gray-500 text-right">{item.version || '1.0'}</td></>}
                  {tab === 'logs' && <><td className="px-5 py-4 text-sm text-gray-900 dark:text-white">{item.script_name || `#${item.script_id}`}</td><td className="px-5 py-4 hidden sm:table-cell"><span className={`px-2 py-0.5 text-xs rounded-full ${item.status === 'success' ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>{item.status || '—'}</span></td></>}
                  {tab === 'alerts' && <><td className="px-5 py-4 text-sm text-gray-900 dark:text-white truncate max-w-xs">{item.message || `#${item.id}`}</td><td className="px-5 py-4 hidden sm:table-cell"><span className={`px-2 py-0.5 text-xs rounded-full ${item.severity === 'critical' ? 'bg-red-100 text-red-700' : 'bg-yellow-100 text-yellow-700'}`}>{item.severity || 'info'}</span></td></>}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </AdminShell>
  );
}
