/**
 * Stora Admin Dashboard — system overview
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { Skeleton } from "~/components/ui/Button";
import { api } from "~/lib/api";

interface DashboardData {
  quota: { max_storage: number; used_storage: number; usage_percent: number; files_count: number };
  totalFiles: number;
  totalFolders: number;
  fileTypeDistribution: Record<string, number>;
}

export const useDashboard = routeLoader$(async () => {
  return await api.get<DashboardData>("/dashboard").catch(() => null);
});

export default component$(() => {
  const data = useDashboard();

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">管理面板</h1>
        <p class="text-sm text-slate-500 mt-0.5">系统概览与统计</p>
      </div>

      <div class="flex-1 overflow-auto scrollbar-thin p-6">
        {!data.value ? (
          <div class="grid grid-cols-3 gap-6">
            {[1,2,3,4,5,6].map(i => <div key={i} class="h-32"><Skeleton class="w-full h-full rounded-xl" /></div>)}
          </div>
        ) : (
          <>
            {/* Stats cards */}
            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
              <StatCard title="文件总数" value={String(data.value.totalFiles)} icon="📄" color="bg-blue-50" />
              <StatCard title="文件夹数" value={String(data.value.totalFolders)} icon="📁" color="bg-amber-50" />
              <StatCard title="存储使用" value={`${data.value.quota.usage_percent}%`} icon="💾" color="bg-green-50" />
              <StatCard title="空间总量" value={`${(data.value.quota.max_storage / 1073741824).toFixed(1)} GB`} icon="☁️" color="bg-purple-50" />
            </div>

            {/* Storage bar */}
            <div class="bg-white rounded-xl border border-slate-200 p-6 mb-8">
              <h3 class="text-sm font-medium text-slate-700 mb-3">存储使用情况</h3>
              <div class="w-full bg-slate-100 rounded-full h-3 overflow-hidden">
                <div class="h-full bg-indigo-500 rounded-full transition-all" style={{ width: `${Math.min(data.value.quota.usage_percent, 100)}%` }} />
              </div>
              <div class="flex justify-between mt-2 text-xs text-slate-400">
                <span>已用 {(data.value.quota.used_storage / 1073741824).toFixed(2)} GB</span>
                <span>共 {(data.value.quota.max_storage / 1073741824).toFixed(1)} GB</span>
              </div>
            </div>

            {/* File type distribution */}
            <div class="bg-white rounded-xl border border-slate-200 p-6">
              <h3 class="text-sm font-medium text-slate-700 mb-4">文件类型分布</h3>
              <div class="space-y-3">
                {Object.entries(data.value.fileTypeDistribution).map(([type, count]) => {
                  const total = data.value.totalFiles || 1;
                  const pct = Math.round((count / total) * 100);
                  const colors: Record<string, string> = { image: "bg-pink-500", video: "bg-purple-500", audio: "bg-blue-500", document: "bg-amber-500", archive: "bg-green-500", other: "bg-slate-400" };
                  return (
                    <div key={type} class="flex items-center gap-3">
                      <span class="w-20 text-xs text-slate-600 font-medium capitalize">{type}</span>
                      <div class="flex-1 h-2 bg-slate-100 rounded-full overflow-hidden">
                        <div class={`h-full rounded-full ${colors[type] || "bg-slate-400"}`} style={{ width: `${pct}%` }} />
                      </div>
                      <span class="w-12 text-xs text-slate-400 text-right">{count}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
});

function StatCard({ title, value, icon, color }: { title: string; value: string; icon: string; color: string }) {
  return (
    <div class={`${color} rounded-xl border border-slate-200/50 p-5`}>
      <div class="flex items-center justify-between mb-2">
        <span class="text-2xl">{icon}</span>
      </div>
      <p class="text-2xl font-bold text-slate-900">{value}</p>
      <p class="text-sm text-slate-500 mt-0.5">{title}</p>
    </div>
  );
}
