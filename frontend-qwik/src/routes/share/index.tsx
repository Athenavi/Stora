/**
 * Stora Share Management — with multi-select batch revoke, search, filter
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, revokeShare, type ShareLink } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";

interface ShareListResponse {
  items: ShareLink[];
  total: number;
  page: number;
  page_size: number;
}

export const useShareList = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  return await srv.get<ShareListResponse>("/files/shares?page=1&page_size=50").catch(() => ({ items: [], total: 0, page: 1, page_size: 50 }));
});

function fmtDate(d: string | undefined | null): string {
  if (!d) return "永久";
  return new Date(d).toLocaleDateString("zh-CN");
}

function permLabel(p: string): string {
  return p === "read" ? "仅查看" : p === "download" ? "可下载" : "可编辑";
}

function isExpired(s: ShareLink): boolean {
  if (!s.is_active) return true;
  if (s.expires_at && new Date(s.expires_at) < new Date()) return true;
  return false;
}

export default component$(() => {
  const data = useShareList();
  const nav = useNavigate();
  const shares = useSignal(data.value.items);
  const selIds = useSignal<number[]>([]);
  const searchQuery = useSignal("");
  const filterStatus = useSignal<"all" | "active" | "expired">("all");

  const filteredShares = () => {
    let list = shares.value;
    // Search
    if (searchQuery.value) {
      const q = searchQuery.value.toLowerCase();
      list = list.filter(s => s.short_code.toLowerCase().includes(q) || s.filename?.toLowerCase().includes(q));
    }
    // Status filter
    if (filterStatus.value === "active") {
      list = list.filter(s => !isExpired(s));
    } else if (filterStatus.value === "expired") {
      list = list.filter(s => isExpired(s));
    }
    return list;
  };

  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <div class="min-w-0">
          <h1 class="text-lg font-semibold text-slate-900">我的分享</h1>
          <p class="text-sm text-slate-500 mt-0.5">管理所有分享链接</p>
        </div>
        <span class="text-sm text-slate-400 bg-slate-100 px-3 py-1 rounded-full shrink-0">{shares.value.length} 个链接</span>
      </div>

      {/* Search + Filter */}
      <div class="flex items-center gap-2 px-4 sm:px-6 py-2 border-b border-slate-100 bg-white/80 shrink-0 flex-wrap">
        <div class="relative flex-1 min-w-[120px] max-w-xs">
          <Icon name="search" size={14} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none" />
          <input type="text" bind:value={searchQuery} placeholder="搜索文件名或链接..."
            class="w-full pl-8 pr-3 py-1.5 text-xs rounded-lg border border-slate-200 bg-slate-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent focus:bg-white placeholder:text-slate-400" />
        </div>
        <div class="flex gap-1">
          {(["all", "active", "expired"] as const).map(f => (
            <button key={f} onClick$={() => filterStatus.value = f}
              class={`touch-target px-2.5 py-1.5 text-xs font-medium rounded-lg transition-colors ${
                filterStatus.value === f ? "bg-indigo-100 text-indigo-700" : "text-slate-500 hover:bg-slate-100"
              }`}>
              {{ all: "全部", active: "有效", expired: "已失效" }[f]}
            </button>
          ))}
        </div>
      </div>

      {/* Batch action bar */}
      {selIds.value.length > 0 && (
        <div class="flex items-center gap-2 px-4 sm:px-6 py-2 bg-indigo-50/80 border-b border-indigo-100 shrink-0">
          <span class="text-sm font-medium text-indigo-700">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={async () => {
            if (!confirm(`确认撤销 ${selIds.value.length} 个分享链接？`)) return;
            for (const id of selIds.value) {
              try { await revokeShare(id); } catch {}
            }
            shares.value = shares.value.filter(x => !selIds.value.includes(x.id));
            selIds.value = [];
          }} class="touch-target px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 rounded-lg transition-colors">批量撤销</button>
          <button onClick$={() => selIds.value = []} class="touch-target px-3 py-1.5 text-xs font-medium text-slate-500 hover:bg-slate-100 rounded-lg transition-colors">取消</button>
        </div>
      )}

      <div class={`flex-1 overflow-auto scrollbar-thin p-4 sm:p-6 ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {filteredShares().length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-16 h-16 rounded-2xl bg-slate-100 flex items-center justify-center text-3xl mb-4">
              {searchQuery.value || filterStatus.value !== "all" ? "🔍" : "🔗"}
            </div>
            <h3 class="text-lg font-medium text-slate-500 mb-1">
              {searchQuery.value ? "未找到匹配的分享链接" : filterStatus.value !== "all" ? "暂无该状态的链接" : "暂无分享链接"}
            </h3>
            <p class="text-sm">{searchQuery.value ? "尝试其他搜索词" : "在文件列表中选择文件创建分享"}</p>
          </div>
        ) : (
          <div class="space-y-3 animate-stagger">
            {filteredShares().map((s) => {
              const sel = selIds.value.includes(s.id);
              const expired = isExpired(s);
              return (
                <div key={s.id}
                  onClick$={() => { const i = selIds.value.indexOf(s.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(s.id); selIds.value = [...selIds.value]; }}
                  class={`flex flex-col sm:flex-row sm:items-center gap-3 sm:gap-4 px-4 sm:px-5 py-4 bg-white rounded-xl border transition-all cursor-pointer ${sel ? 'border-indigo-400 shadow-sm' : 'border-slate-200 hover:shadow-sm'}`}>
                  <div class="flex items-center gap-3 sm:gap-4 flex-1 min-w-0">
                    <input type="checkbox" checked={sel} class="rounded border-slate-300 w-4 h-4 shrink-0"
                      onChange$={() => {/* handled by row click */}} />
                    <div class={`w-10 h-10 rounded-xl flex items-center justify-center text-lg shrink-0 ${s.is_folder ? 'bg-amber-50 text-amber-600' : 'bg-indigo-50 text-indigo-600'}`}>
                      {s.is_folder ? "📁" : s.password_protected ? "🔒" : "🔓"}
                    </div>
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="text-sm font-medium text-slate-700 truncate">{s.filename || `分享 ${s.short_code}`}</span>
                        {s.is_folder && <span class="px-1.5 py-0.5 bg-amber-100 text-amber-700 text-xs rounded-full">文件夹</span>}
                        {expired && <span class="px-1.5 py-0.5 bg-slate-100 text-slate-500 text-xs rounded-full">已失效</span>}
                        {!expired && s.expires_at && (new Date(s.expires_at).getTime() - Date.now() < 86400000) && (
                          <span class="px-1.5 py-0.5 bg-orange-100 text-orange-700 text-xs rounded-full">即将过期</span>
                        )}
                      </div>
                      <div class="flex items-center gap-2 sm:gap-3 mt-1 text-xs text-slate-400 flex-wrap">
                        <span>{s.is_folder ? "文件夹" : permLabel(s.permission)}</span>
                        <span>· 浏览 {s.view_count} 次</span>
                        <span>· 下载 {s.download_count} 次</span>
                        {s.expires_at && <span>· {fmtDate(s.expires_at)} 过期</span>}
                      </div>
                    </div>
                  </div>
                  <div class="flex items-center gap-2 shrink-0 ml-0 sm:ml-2" onClick$={(e: any) => e.stopPropagation()}>
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
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
});
