/**
 * Stora Share Management — flat design card layout
 */
import { component$, useSignal, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, revokeShare, type ShareLink } from "~/lib/api";
import InfiniteScroll from "~/components/ui/InfiniteScroll";

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
  const total = useSignal(data.value.total);
  const page = useSignal(1);
  const loading = useSignal(false);
  const selIds = useSignal<number[]>([]);
  const searchQuery = useSignal("");
  const filterStatus = useSignal<"all" | "active" | "expired">("all");

  const loadMore = $(async () => {
    if (loading.value) return;
    loading.value = true;
    try {
      const next = page.value + 1;
      const res = await fetch(`/api/v2/files/shares?page=${next}&page_size=50`);
      const json = await res.json();
      const d = json.data || json;
      if (d.items?.length) {
        shares.value = [...shares.value, ...d.items];
        total.value = d.total;
        page.value = next;
      }
    } catch {}
    loading.value = false;
  });

  const filteredShares = () => {
    let list = shares.value;
    if (searchQuery.value) {
      const q = searchQuery.value.toLowerCase();
      list = list.filter(s => s.short_code.toLowerCase().includes(q) || s.filename?.toLowerCase().includes(q));
    }
    if (filterStatus.value === "active") {
      list = list.filter(s => !isExpired(s));
    } else if (filterStatus.value === "expired") {
      list = list.filter(s => isExpired(s));
    }
    return list;
  };

  return (
    <div class="flex flex-col h-full">
      {/* Page title area per spec */}
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <h1 class="text-[28px] font-bold text-stora-foreground">我的分享</h1>
        <p class="text-sm text-stora-muted-foreground mt-1">管理你的所有分享链接</p>
      </div>

      {/* Search + Filter */}
      <div class="flex items-center gap-2 px-6 py-2 border-b border-stora-border bg-stora-card shrink-0 flex-wrap">
        <div class="relative flex-1 min-w-[120px] max-w-xs">
          <span class="absolute left-3 top-1/2 -translate-y-1/2 text-stora-nav-text text-sm pointer-events-none">🔍</span>
          <input type="text" bind:value={searchQuery} placeholder="搜索文件名或链接..."
            class="w-full h-9 pl-8 pr-3 text-xs border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary" />
        </div>
        <div class="flex gap-1">
          {(["all", "active", "expired"] as const).map(f => (
            <button key={f} onClick$={() => filterStatus.value = f}
              class={`touch-target px-3 py-1.5 text-xs font-medium ${
                filterStatus.value === f ? "bg-stora-primary text-white" : "text-stora-muted-foreground hover:bg-stora-muted"
              }`}>
              {{ all: "全部", active: "有效", expired: "已失效" }[f]}
            </button>
          ))}
        </div>
      </div>

      {/* Batch action bar */}
      {selIds.value.length > 0 && (
        <div class="flex items-center gap-2 px-6 py-2 bg-stora-muted border-b border-stora-border shrink-0">
          <span class="text-sm font-medium text-stora-foreground">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={async () => {
            if (!confirm(`确认撤销 ${selIds.value.length} 个分享链接？`)) return;
            for (const id of selIds.value) {
              try { await revokeShare(id); } catch {}
            }
            shares.value = shares.value.filter(x => !selIds.value.includes(x.id));
            selIds.value = [];
          }} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-destructive hover:bg-red-50">批量撤销</button>
          <button onClick$={() => selIds.value = []} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-muted-foreground hover:bg-stora-muted">取消</button>
        </div>
      )}

      <div class={`flex-1 overflow-auto scrollbar-thin ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {filteredShares().length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground p-8">
            <div class="w-16 h-16 bg-stora-muted flex items-center justify-center text-3xl mb-4">
              {searchQuery.value || filterStatus.value !== "all" ? "🔍" : "🔗"}
            </div>
            <h3 class="text-lg font-medium text-stora-foreground mb-1">
              {searchQuery.value ? "未找到匹配的分享链接" : filterStatus.value !== "all" ? "暂无该状态的链接" : "暂无分享链接"}
            </h3>
            <p class="text-sm text-stora-muted-foreground">{searchQuery.value ? "尝试其他搜索词" : "在文件列表中选择文件创建分享"}</p>
          </div>
        ) : (
          <div class="divide-y divide-stora-border">
            {filteredShares().map((s) => {
              const sel = selIds.value.includes(s.id);
              const expired = isExpired(s);
              return (
                <div key={s.id}
                  onClick$={() => { const i = selIds.value.indexOf(s.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(s.id); selIds.value = [...selIds.value]; }}
                  class={`flex items-center gap-4 px-6 py-5 bg-stora-card cursor-pointer ${sel ? 'bg-stora-muted' : 'hover:bg-stora-muted'}`}>
                  <input type="checkbox" checked={sel} class="border-stora-border w-4 h-4 shrink-0"
                    onChange$={() => {/* handled by row click */}} />
                  {/* File icon 44x44 per spec */}
                  <div class={`w-11 h-11 flex items-center justify-center text-xl shrink-0 ${s.is_folder ? 'bg-stora-accent' : 'bg-[#FEF3C7]'}`}>
                    {s.is_folder ? "📁" : s.password_protected ? "🔒" : "📄"}
                  </div>
                  {/* Info area */}
                  <div class="flex-1 min-w-0">
                    <p class="text-[15px] font-semibold text-stora-foreground truncate">{s.filename || `分享 ${s.short_code}`}</p>
                    <p class="text-xs text-stora-muted-foreground mt-1">
                      分享链接 · {s.view_count}人访问 · {fmtDate(s.created_at)}
                      {expired && <span class="ml-2 text-stora-destructive">已失效</span>}
                    </p>
                  </div>
                  {/* Link area 240x36 per spec */}
                  <div class="hidden sm:flex items-center bg-stora-muted px-3 h-9 w-[240px] shrink-0">
                    <span class="text-xs text-stora-primary truncate">{window.location.origin}/s/{s.short_code}</span>
                  </div>
                  {/* Copy button 64x32 per spec */}
                  <button onClick$={(e: any) => { e.stopPropagation(); navigator.clipboard.writeText(`${window.location.origin}/s/${s.short_code}`); }}
                    class="h-8 px-4 text-xs font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] shrink-0">
                    复制
                  </button>
                  {s.is_active && (
                    <button onClick$={async (e: any) => { e.stopPropagation(); if (!confirm("确认撤销？")) return; try { await revokeShare(s.id); shares.value = shares.value.filter(x => x.id !== s.id); } catch {} }}
                      class="h-8 px-3 text-xs font-medium text-stora-destructive hover:bg-red-50 shrink-0">
                      撤销
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        )}
        <InfiniteScroll
          hasMore={shares.value.length < total.value}
          loading={loading.value}
          onLoadMore$={loadMore}
        />
      </div>
    </div>
  );
});
