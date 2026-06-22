/**
 * Stora Favorites — bookmarked files with batch operations
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, updateFile, batchDownload, api, type FileItem } from "~/lib/api";
import { Button, Skeleton } from "~/components/ui/Button";
import { Icon } from "~/components/ui/Icon";

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
  const selIds = useSignal<number[]>([]);
  const viewMode = useSignal<"list" | "grid">("grid");
  // Context menu
  const ctxItem = useSignal<{ id: number; name: string } | null>(null);
  const ctxPos = useSignal({ x: 0, y: 0 });
  const showActionSheet = useSignal(false);

  const removeFav = async (id: number) => {
    try {
      await updateFile(id, { is_favorite: false });
      items.value = items.value.filter(f => f.id !== id);
      selIds.value = selIds.value.filter(sid => sid !== id);
    } catch {}
  };

  const batchRemoveFav = async () => {
    for (const id of selIds.value) {
      try { await updateFile(id, { is_favorite: false }); } catch {}
    }
    items.value = items.value.filter(f => !selIds.value.includes(f.id));
    selIds.value = [];
  };

  const openCtx = (item: { id: number; name: string }, e: any) => {
    if (window.innerWidth < 768) {
      ctxItem.value = item;
      showActionSheet.value = true;
    } else {
      ctxPos.value = { x: e.clientX, y: e.clientY };
      ctxItem.value = item;
    }
  };

  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <div>
          <h1 class="text-lg font-semibold text-slate-900">收藏夹</h1>
          <p class="text-sm text-slate-500 mt-0.5">{items.value.length} 个收藏文件</p>
        </div>
        <div class="flex items-center gap-2">
          <div class="flex items-center gap-1 bg-slate-100 rounded-lg p-0.5">
            <button onClick$={() => viewMode.value = "list"}
              class={`p-1.5 rounded-md ${viewMode.value === "list" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400"}`}><Icon name="list" size={18} /></button>
            <button onClick$={() => viewMode.value = "grid"}
              class={`p-1.5 rounded-md ${viewMode.value === "grid" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400"}`}><Icon name="grid" size={18} /></button>
          </div>
        </div>
      </div>

      {/* Batch action bar */}
      {selIds.value.length > 0 && (
        <div class="flex items-center gap-2 px-4 sm:px-6 py-2 bg-indigo-50/80 border-b border-indigo-100 shrink-0">
          <span class="text-sm font-medium text-indigo-700">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={batchRemoveFav}
            class="touch-target px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 rounded-lg transition-colors">取消收藏</button>
          <button onClick$={() => { batchDownload([...selIds.value]); }}
            class="touch-target px-3 py-1.5 text-xs font-medium text-indigo-600 hover:bg-indigo-100 rounded-lg transition-colors">下载</button>
          <button onClick$={() => selIds.value = []}
            class="touch-target px-3 py-1.5 text-xs font-medium text-slate-500 hover:bg-slate-100 rounded-lg transition-colors">取消选择</button>
        </div>
      )}

      <div class={`flex-1 overflow-auto scrollbar-thin ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {!data.value ? (
          <div class="p-4 sm:p-6 space-y-3">{[1,2,3].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><Skeleton class="w-10 h-10 rounded-lg" /><div class="flex-1 space-y-2"><Skeleton class="h-4 w-48" /><Skeleton class="h-3 w-24" /></div></div>)}</div>
        ) : items.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400 p-8 text-center">
            <div class="text-6xl mb-4">⭐</div>
            <h3 class="text-lg font-medium text-slate-500">暂无收藏</h3>
            <p class="text-sm mt-1">在文件列表中右键文件 → 收藏</p>
            <Button variant="primary" size="sm" class="mt-4" onClick$={() => nav("/drive")}>前往文件列表</Button>
          </div>
        ) : viewMode.value === "grid" ? (
          <div class="card-grid p-4 sm:p-6">
            {items.value.map(f => {
              const sel = selIds.value.includes(f.id);
              return (
                <div key={f.id}
                  onClick$={() => { if (selIds.value.length > 0) { const i = selIds.value.indexOf(f.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id); selIds.value = [...selIds.value]; } else nav(`/view?id=${f.id}`); }}
                  onContextMenu$={(e: any) => { e.preventDefault(); openCtx({ id: f.id, name: f.filename }, e); }}
                  class={`bg-white rounded-xl border-2 transition-all cursor-pointer p-3 relative ${sel ? "border-indigo-500 shadow-md" : "border-slate-200 hover:border-slate-300 hover:shadow-sm"}`}>
                  <div class="aspect-square rounded-lg bg-amber-50 flex items-center justify-center text-4xl mb-2">⭐</div>
                  <p class="text-xs font-medium text-slate-700 truncate text-center">{f.filename}</p>
                  <p class="text-xs text-slate-400 text-center mt-0.5">{fmtSize(f.file_size)}</p>
                  {sel && <div class="absolute top-2 right-2 w-5 h-5 bg-indigo-600 rounded-full flex items-center justify-center"><Icon name="check" size={12} class="text-white" /></div>}
                </div>
              );
            })}
          </div>
        ) : (
          <div class="overflow-x-auto">
            <table class="w-full min-w-[500px]">
              <thead>
                <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 sticky top-0 bg-slate-50/95 backdrop-blur">
                  <th class="w-10 px-4 py-3"><input type="checkbox" checked={selIds.value.length === items.value.length && items.value.length > 0}
                    onChange$={() => selIds.value = selIds.value.length === items.value.length ? [] : items.value.map(x => x.id)} class="rounded border-slate-300" /></th>
                  <th class="px-2 py-3">文件名</th>
                  <th class="px-2 py-3 w-28">大小</th>
                  <th class="px-2 py-3 w-24">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-50">
                {items.value.map(f => {
                  const sel = selIds.value.includes(f.id);
                  return (
                    <tr key={f.id} class={`group text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}
                      onContextMenu$={(e: any) => { e.preventDefault(); openCtx({ id: f.id, name: f.filename }, e); }}>
                      <td class="px-4 py-3"><input type="checkbox" checked={sel}
                        onChange$={() => { const i = selIds.value.indexOf(f.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id); selIds.value = [...selIds.value]; }} class="rounded border-slate-300" /></td>
                      <td class="px-2 py-3 cursor-pointer" onClick$={() => nav(`/view?id=${f.id}`)}>
                        <div class="flex items-center gap-3">
                          <div class="w-9 h-9 rounded-lg bg-amber-50 flex items-center justify-center text-sm">⭐</div>
                          <span class="text-slate-700 truncate max-w-xs font-medium">{f.filename}</span>
                        </div>
                      </td>
                      <td class="px-2 py-3 text-slate-500">{fmtSize(f.file_size)}</td>
                      <td class="px-2 py-3">
                        <button onClick$={() => removeFav(f.id)} class="text-xs text-red-500 hover:text-red-700 touch-target px-2 py-1">取消收藏</button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Desktop context menu */}
      {ctxItem.value && !showActionSheet.value && (
        <>
          <div class="fixed inset-0 z-50" onClick$={() => ctxItem.value = null} onContextMenu$={(e: any) => { e.preventDefault(); ctxItem.value = null; }} />
          <div class="fixed z-50 min-w-[160px] bg-white rounded-xl shadow-lg border border-slate-200 py-1.5"
            style={{ left: `${ctxPos.value.x}px`, top: `${ctxPos.value.y}px` }}>
            <button onClick$={() => { nav(`/view?id=${ctxItem.value!.id}`); ctxItem.value = null; }}
              class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">👁 预览</button>
            <button onClick$={() => { window.open(`/api/v2/files/download/${ctxItem.value!.id}`, "_blank"); ctxItem.value = null; }}
              class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">⬇ 下载</button>
            <div class="h-px bg-slate-100 my-1" />
            <button onClick$={async () => { await removeFav(ctxItem.value!.id); ctxItem.value = null; }}
              class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-red-600 hover:bg-red-50 touch-target">🗑 取消收藏</button>
          </div>
        </>
      )}

      {/* Mobile action sheet */}
      {ctxItem.value && showActionSheet.value && (
        <>
          <div class="action-sheet-overlay" onClick$={() => { ctxItem.value = null; showActionSheet.value = false; }} />
          <div class="action-sheet">
            <div class="w-10 h-1 bg-slate-300 rounded-full mx-auto mb-3" />
            <p class="text-xs text-slate-400 text-center mb-3">{ctxItem.value.name}</p>
            <div class="space-y-1">
              <button onClick$={() => { nav(`/view?id=${ctxItem.value!.id}`); ctxItem.value = null; showActionSheet.value = false; }}
                class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">👁 预览</button>
              <button onClick$={() => { window.open(`/api/v2/files/download/${ctxItem.value!.id}`, "_blank"); ctxItem.value = null; showActionSheet.value = false; }}
                class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">⬇ 下载</button>
              <div class="h-px bg-slate-100 my-1" />
              <button onClick$={async () => { await removeFav(ctxItem.value!.id); ctxItem.value = null; showActionSheet.value = false; }}
                class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-red-600 hover:bg-red-50 rounded-lg touch-target">🗑 取消收藏</button>
            </div>
            <button onClick$={() => { ctxItem.value = null; showActionSheet.value = false; }}
              class="w-full mt-3 px-4 py-3 text-sm font-medium text-slate-500 bg-slate-100 hover:bg-slate-200 rounded-xl touch-target">取消</button>
          </div>
        </>
      )}
    </div>
  );
});
