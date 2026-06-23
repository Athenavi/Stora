/**
 * Stora Trash — flat design recycle bin
 */
import { component$, useSignal, $ } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { api, createServerApi } from "~/lib/api";
import InfiniteScroll from "~/components/ui/InfiniteScroll";

interface TrashItem {
  id: number;
  filename: string;
  file_size: number;
  file_type: string;
  deleted_at: string | null;
  original_filename?: string;
}

export const useTrashList = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  return await srv.get<{ items: TrashItem[]; total: number }>("/files/trash?page=1&per_page=50").catch(() => ({ items: [], total: 0 }));
});

function fmtSize(bytes: number): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

function fmtDate(s: string | null | undefined): string {
  if (!s) return "";
  const d = new Date(s);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export default component$(() => {
  const data = useTrashList();
  const items = useSignal(data.value.items);
  const total = useSignal(data.value.total);
  const page = useSignal(1);
  const selIds = useSignal<number[]>([]);
  const loading = useSignal(false);

  const loadMore = $(async () => {
    if (loading.value) return;
    loading.value = true;
    try {
      const next = page.value + 1;
      const res = await fetch(`/api/v2/files/trash?page=${next}&per_page=50`);
      const json = await res.json();
      const d = json.data || json;
      if (d.items?.length) {
        items.value = [...items.value, ...d.items];
        total.value = d.total;
        page.value = next;
      }
    } catch {}
    loading.value = false;
  });

  return (
    <div class="flex flex-col h-full">
      {/* Title area per spec */}
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <h1 class="text-[28px] font-bold text-stora-foreground">回收站</h1>
        <p class="text-sm text-stora-muted-foreground mt-1">已删除的文件将在30天后自动清除</p>
      </div>

      {/* Batch action bar */}
      {selIds.value.length > 0 && (
        <div class="flex items-center gap-2 px-6 py-2 bg-stora-muted border-b border-stora-border shrink-0">
          <span class="text-sm font-medium text-stora-foreground">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={async () => {
            loading.value = true;
            try { await api.post("/files/trash/batch-restore", { file_ids: selIds.value }); items.value = items.value.filter(x => !selIds.value.includes(x.id)); selIds.value = []; } catch {}
            loading.value = false;
          }} disabled={loading.value} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-primary">
            {loading.value ? "..." : `恢复 (${selIds.value.length})`}
          </button>
          <button onClick$={async () => {
            if (!confirm("永久删除选中的文件？")) return;
            loading.value = true;
            try { await api.post("/files/trash/batch-destroy", { file_ids: selIds.value }); items.value = items.value.filter(x => !selIds.value.includes(x.id)); selIds.value = []; } catch {}
            loading.value = false;
          }} disabled={loading.value} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-destructive">
            {loading.value ? "..." : "永久删除"}
          </button>
        </div>
      )}

      <div class={`flex-1 overflow-auto scrollbar-thin ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {!data.value ? (
          <div class="p-6 space-y-3">
            {[1,2,3].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><div class="w-5 h-5 bg-stora-muted" /><div class="w-10 h-10 bg-stora-muted" /><div class="flex-1 space-y-2"><div class="h-4 w-48 bg-stora-muted" /><div class="h-3 w-24 bg-stora-muted" /></div></div>)}
          </div>
        ) : items.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
            <div class="w-20 h-20 bg-stora-muted flex items-center justify-center text-4xl mb-5">🗑️</div>
            <h3 class="text-lg font-semibold text-stora-foreground mb-1">回收站为空</h3>
            <p class="text-sm text-stora-muted-foreground">删除的文件将出现在这里</p>
          </div>
        ) : (
          <>
            {/* Desktop table */}
            <div class="hidden sm:block overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="text-left text-xs font-semibold text-stora-muted-foreground bg-stora-muted">
                    <th class="w-10 px-4 py-3 font-semibold">
                      <input type="checkbox" checked={selIds.value.length === items.value.length} onChange$={() => selIds.value = selIds.value.length === items.value.length ? [] : items.value.map(f => f.id)} class="border-stora-border" />
                    </th>
                    <th class="px-2 py-3 font-semibold">文件名称</th>
                    <th class="px-2 py-3 w-[100px] font-semibold">大小</th>
                    <th class="px-2 py-3 w-[160px] font-semibold">删除时间</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-stora-muted">
                  {items.value.map(f => {
                    const sel = selIds.value.includes(f.id);
                    return (
                      <tr key={f.id} class={`group text-sm h-12 ${sel ? "bg-stora-muted" : "hover:bg-stora-muted"}`}>
                        <td class="px-4">
                          <input type="checkbox" checked={sel} onChange$={() => {
                            const i = selIds.value.indexOf(f.id);
                            if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id);
                            selIds.value = [...selIds.value];
                          }} class="border-stora-border" />
                        </td>
                        <td class="px-2">
                          <div class="flex items-center gap-3">
                            <span class="text-sm">🗑</span>
                            <span class="text-sm font-medium text-stora-foreground truncate max-w-xs">{f.filename}</span>
                          </div>
                        </td>
                        <td class="px-2 text-sm text-stora-muted-foreground">{fmtSize(f.file_size)}</td>
                        <td class="px-2 text-sm text-stora-muted-foreground">{fmtDate(f.deleted_at)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {/* Mobile card list */}
            <div class="sm:hidden space-y-1 p-4">
              {items.value.map(f => {
                const sel = selIds.value.includes(f.id);
                return (
                  <div key={f.id} class={`bg-stora-card border border-stora-border p-4 ${sel ? "border-stora-primary" : ""}`}>
                    <div class="flex items-center gap-3">
                      <input type="checkbox" checked={sel} onChange$={() => {
                        const i = selIds.value.indexOf(f.id);
                        if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id);
                        selIds.value = [...selIds.value];
                      }} class="border-stora-border w-5 h-5" />
                      <span class="text-lg">🗑</span>
                      <div class="flex-1 min-w-0">
                        <p class="text-sm font-medium text-stora-foreground truncate">{f.filename}</p>
                        <p class="text-xs text-stora-muted-foreground mt-0.5">{fmtSize(f.file_size)} · {fmtDate(f.deleted_at)}</p>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </>
        )}
        <InfiniteScroll
          hasMore={items.value.length < total.value}
          loading={loading.value}
          onLoadMore$={loadMore}
        />
      </div>

      {/* Mobile bottom action bar for batch ops */}
      {selIds.value.length > 0 && (
        <div class="fixed bottom-0 left-0 right-0 z-40 bg-white border-t border-stora-border px-4 py-3 flex items-center gap-2 sm:hidden">
          <span class="text-sm font-medium text-stora-foreground">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={async () => {
            loading.value = true;
            try { await api.post("/files/trash/batch-restore", { file_ids: selIds.value }); items.value = items.value.filter(x => !selIds.value.includes(x.id)); selIds.value = []; } catch {}
            loading.value = false;
          }} disabled={loading.value} class="touch-target px-4 py-2 text-xs font-medium text-stora-primary">恢复</button>
          <button onClick$={async () => {
            if (!confirm("永久删除选中的文件？")) return;
            loading.value = true;
            try { await api.post("/files/trash/batch-destroy", { file_ids: selIds.value }); items.value = items.value.filter(x => !selIds.value.includes(x.id)); selIds.value = []; } catch {}
            loading.value = false;
          }} disabled={loading.value} class="touch-target px-4 py-2 text-xs font-medium text-stora-destructive">永久删除</button>
        </div>
      )}
    </div>
  );
});
