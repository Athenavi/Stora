/**
 * Stora Share Management
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { listShares, revokeShare } from "~/lib/api";

export const useShareList = routeLoader$(async () => {
  return await listShares(1, 50).catch(() => ({ items: [], total: 0, page: 1, page_size: 50 }));
});

function fmtDate(d: string | undefined | null): string {
  if (!d) return "永久";
  return new Date(d).toLocaleDateString("zh-CN");
}

function permLabel(p: string): string {
  return p === "read" ? "仅查看" : p === "download" ? "可下载" : "可编辑";
}

export default component$(() => {
  const data = useShareList();
  const shares = useSignal(data.value.items);

  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <div class="min-w-0">
          <h1 class="text-lg font-semibold text-slate-900">我的分享</h1>
          <p class="text-sm text-slate-500 mt-0.5">管理所有分享链接</p>
        </div>
        <span class="text-sm text-slate-400 bg-slate-100 px-3 py-1 rounded-full shrink-0">{shares.value.length} 个链接</span>
      </div>

      <div class="flex-1 overflow-auto scrollbar-thin p-4 sm:p-6">
        {shares.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-16 h-16 rounded-2xl bg-slate-100 flex items-center justify-center text-3xl mb-4">🔗</div>
            <h3 class="text-lg font-medium text-slate-500 mb-1">暂无分享链接</h3>
            <p class="text-sm">在文件列表中选择文件创建分享</p>
          </div>
        ) : (
          <div class="space-y-3 animate-stagger">
            {shares.value.map((s) => (
              <div key={s.id} class="flex flex-col sm:flex-row sm:items-center gap-3 sm:gap-4 px-4 sm:px-5 py-4 bg-white rounded-xl border border-slate-200 hover:shadow-sm transition-shadow">
                <div class="flex items-center gap-3 sm:gap-4 flex-1 min-w-0">
                  <div class="w-10 h-10 rounded-xl bg-indigo-50 flex items-center justify-center text-indigo-600 text-lg shrink-0">
                    {s.password_protected ? "🔒" : "🔓"}
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="text-sm font-medium text-slate-700 truncate">分享链接 {s.short_code}</span>
                      {!s.is_active && <span class="px-2 py-0.5 bg-slate-100 text-slate-500 text-xs rounded-full">已失效</span>}
                    </div>
                    <div class="flex items-center gap-2 sm:gap-3 mt-1 text-xs text-slate-400 flex-wrap">
                      <span>{permLabel(s.permission)}</span>
                      <span>· 浏览 {s.view_count} 次</span>
                      <span>· 下载 {s.download_count} 次</span>
                      {s.expires_at && <span>· {fmtDate(s.expires_at)} 过期</span>}
                    </div>
                  </div>
                </div>
                <div class="flex items-center gap-2 shrink-0 ml-0 sm:ml-2">
                  <button
                    onClick$={() => navigator.clipboard.writeText(`${window.location.origin}/s/${s.short_code}`)}
                    class="touch-target px-3 py-1.5 text-xs font-medium text-indigo-600 bg-indigo-50 hover:bg-indigo-100 rounded-lg transition-colors">
                    复制链接
                  </button>
                  {s.is_active && (
                    <button
                      onClick$={async () => {
                        if (!confirm("确认撤销此分享链接？撤销后链接将立即失效。")) return;
                        try {
                          await revokeShare(s.id);
                          shares.value = shares.value.filter(x => x.id !== s.id);
                        } catch { /* ignore */ }
                      }}
                      class="touch-target px-3 py-1.5 text-xs font-medium text-red-600 bg-red-50 hover:bg-red-100 rounded-lg transition-colors">
                      撤销
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
