/**
 * Stora Favorites — flat design card grid
 */
import { component$, useSignal, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, updateFile, batchDownload, api, type FileItem } from "~/lib/api";
import InfiniteScroll from "~/components/ui/InfiniteScroll";

export const useFavList = routeLoader$(async ({ request }) => {
  const api = createServerApi(request);
  return await api.get<{ items: FileItem[]; total: number; page: number; page_size: number }>("/files?page=1&page_size=200&is_favorite=true").catch(() => null);
});

function fmtSize(b: number): string {
  if (!b) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

export default component$(() => {
  const data = useFavList();
  const nav = useNavigate();
  const items = useSignal(data.value?.items?.filter(f => f.is_favorite) || []);
  const total = useSignal(data.value?.total || 0);
  const page = useSignal(1);
  const loadingMore = useSignal(false);
  const selIds = useSignal<number[]>([]);

  const loadMore = $(async () => {
    if (loadingMore.value) return;
    loadingMore.value = true;
    try {
      const next = page.value + 1;
      const res = await fetch(`/api/v2/files?page=${next}&page_size=200&is_favorite=true`);
      const json = await res.json();
      const d = json.data || json;
      if (d.items?.length) {
        items.value = [...items.value, ...d.items.filter((f: any) => f.is_favorite)];
        total.value = d.total;
        page.value = next;
      }
    } catch {}
    loadingMore.value = false;
  });

  const removeFav = async (id: number) => {
    try { await updateFile(id, { is_favorite: false }); items.value = items.value.filter(f => f.id !== id); selIds.value = selIds.value.filter(sid => sid !== id); } catch {}
  };

  const batchRemoveFav = async () => {
    for (const id of selIds.value) { try { await updateFile(id, { is_favorite: false }); } catch {} }
    items.value = items.value.filter(f => !selIds.value.includes(f.id));
    selIds.value = [];
  };

  type IconMeta = { color: string; icon: string };
  const fileIcon = (f: FileItem): IconMeta => {
    if (f.file_type === "image") return { color: "#DBEAFE", icon: "🖼" };
    if (f.file_type === "document") return { color: "#FEF3C7", icon: "📄" };
    if (f.file_type === "presentation") return { color: "#DBEAFE", icon: "📊" };
    if (f.file_type === "video") return { color: "#FCE7F3", icon: "🎬" };
    if (f.file_type === "audio") return { color: "#D1FAE5", icon: "🎵" };
    if (f.file_type === "archive") return { color: "#D1FAE5", icon: "📦" };
    return { color: "#F1F5FD", icon: "📎" };
  };

  return (
    <div class="flex flex-col h-full">
      {/* Title area per spec */}
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <h1 class="text-[28px] font-bold text-stora-foreground">收藏夹</h1>
        <p class="text-sm text-stora-muted-foreground mt-1">你收藏的文件和文件夹</p>
      </div>

      {/* Batch action bar */}
      {selIds.value.length > 0 && (
        <div class="flex items-center gap-2 px-6 py-2 bg-stora-muted border-b border-stora-border shrink-0">
          <span class="text-sm font-medium text-stora-foreground">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={batchRemoveFav} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-destructive hover:bg-red-50">取消收藏</button>
          <button onClick$={() => { batchDownload([...selIds.value]); }} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-primary hover:bg-stora-muted">下载</button>
          <button onClick$={() => selIds.value = []} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-muted-foreground">取消选择</button>
        </div>
      )}

      <div class={`flex-1 overflow-auto scrollbar-thin p-6 ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {!data.value ? (
          <div class="flex items-center justify-center h-full text-stora-muted-foreground">加载中...</div>
        ) : items.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
            <div class="text-6xl mb-4">⭐</div>
            <h3 class="text-lg font-medium text-stora-foreground">暂无收藏</h3>
            <p class="text-sm mt-1 text-stora-muted-foreground">在文件列表中将文件加入收藏</p>
            <button onClick$={() => nav("/drive")} class="mt-4 px-4 py-2 bg-stora-primary text-white text-sm font-medium">前往文件列表</button>
          </div>
        ) : (
          /* Card grid per spec: 200x180px cards */
          <div class="flex flex-wrap gap-4">
            {items.value.map(f => {
              const sel = selIds.value.includes(f.id);
              const icon = fileIcon(f);
              return (
                <div key={f.id}
                  onClick$={() => { if (selIds.value.length > 0) { const i = selIds.value.indexOf(f.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id); selIds.value = [...selIds.value]; } else nav(`/view?id=${f.id}`); }}
                  class={`w-[200px] h-[180px] bg-stora-card border border-stora-border flex flex-col items-center justify-center p-4 gap-2.5 cursor-pointer ${sel ? "border-stora-primary" : "hover:border-stora-muted-foreground"}`}>
                  {/* Icon 56x56, border radius 12px per spec */}
                  <div class="w-14 h-14 flex items-center justify-center text-2xl" style={{ backgroundColor: icon.color }}>
                    {icon.icon}
                  </div>
                  {/* Filename 13px SemiBold */}
                  <p class="text-sm font-semibold text-stora-foreground text-center truncate w-full">{f.filename}</p>
                  {/* Size 11px */}
                  <p class="text-xs text-stora-nav-text">{fmtSize(f.file_size)}</p>
                  {sel && <div class="absolute top-2 right-2 w-5 h-5 bg-stora-primary flex items-center justify-center"><span class="text-white text-xs">✓</span></div>}
                </div>
              );
            })}
          </div>
        )}
        <InfiniteScroll
          hasMore={items.value.length < total.value}
          loading={loadingMore.value}
          onLoadMore$={loadMore}
        />
      </div>
    </div>
  );
});
