/**
 * Stora Drive — core file manager with rename, folder create, batch ops, drag-drop
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { createServerApi, listFiles, getFolderChildren, createFolder, updateFile, updateFolder, deleteFile, deleteFolder, moveFiles, uploadFile, batchDownload, createShare, api, type FileItem, type Folder } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button, Skeleton, Input } from "~/components/ui/Button";

export const useFileList = routeLoader$(async ({ url, request }) => {
  const folderId = url.searchParams.get("folder");
  const search = url.searchParams.get("search");
  const fileType = url.searchParams.get("type");
  const sortBy = url.searchParams.get("sort_by") || "created_at";
  const sortOrder = url.searchParams.get("sort_order") || "desc";
  const api = createServerApi(request);

  if (folderId) {
    const d = await api.get(`/files/folders/${folderId}/children`).catch(() => null);
    return d || { folders: [], files: [], path: [{ id: 0, name: "我的文件" }] };
  }
  if (search) {
    const d = await api.get(`/files/search?q=${encodeURIComponent(search)}&page=1&page_size=50`).catch(() => null);
    return d ? { folders: [], files: d.items || [], path: [{ id: 0, name: `搜索: ${search}` }] } : null;
  }
  const typeParam = fileType ? `&file_type=${fileType}` : "";
  const sortParam = `&sort_by=${sortBy}&sort_order=${sortOrder}`;
  const files = await api.get(`/files?page=1&page_size=50${typeParam}${sortParam}`).catch(() => null);
  return files ? { folders: [], files: files.items, path: [{ id: 0, name: "我的文件" }] } : null;
});

function fmtSize(b: number): string {
  if (!b) return "0 B";
  const k = 1024, i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

const typeMeta: Record<string, { icon: string; color: string }> = {
  image: { icon: "🖼️", color: "bg-pink-50 text-pink-600" },
  video: { icon: "🎬", color: "bg-purple-50 text-purple-600" },
  audio: { icon: "🎵", color: "bg-blue-50 text-blue-600" },
  document: { icon: "📄", color: "bg-amber-50 text-amber-600" },
  archive: { icon: "📦", color: "bg-green-50 text-green-600" },
  other: { icon: "📎", color: "bg-slate-50 text-slate-600" },
};

export default component$(() => {
  const data = useFileList();
  const nav = useNavigate();
  const loc = useLocation();
  const folderId = loc.url.searchParams.get("folder") ? Number(loc.url.searchParams.get("folder")) : undefined;

  const allItems: ({ t: "f" } & Folder | { t: "d" } & FileItem)[] = [];
  if (data.value) {
    for (const f of data.value.folders || []) allItems.push({ t: "f" as const, ...f });
    for (const f of data.value.files || []) allItems.push({ t: "d" as const, ...f });
  }

  const viewMode = useSignal<"list" | "grid">("list");
  const selIds = useSignal<number[]>([]);
  const showUpload = useSignal(false);
  const showNewFolder = useSignal(false);
  const newFolderName = useSignal("");
  const renameId = useSignal(0);
  const renameVal = useSignal("");
  const searchHistory = useSignal<any[]>([]);
  // Context menu state (desktop floating + mobile action sheet)
  const ctxItem = useSignal<{ id: number; type: "file" | "folder"; name: string; fileType?: string } | null>(null);
  const ctxPos = useSignal({ x: 0, y: 0 });
  const showActionSheet = useSignal(false);
  // Batch operation dialogs
  const showMoveDialog = useSignal(false);
  const showShareDialog = useSignal(false);
  const shareResult = useSignal<{ code: string; url: string }[]>([]);
  const moveTargetId = useSignal<number | undefined>(undefined);
  const moveTree = useSignal<any[]>([]);
  const moveTreeLoading = useSignal(false);

  const doRename = (item: { id: number; type: string; name: string }) => {
    renameId.value = item.id;
    renameVal.value = item.name;
  };

  const openCtx = $((item: any, e: any) => {
    if (window.innerWidth < 768) {
      ctxItem.value = item;
      showActionSheet.value = true;
    } else {
      ctxPos.value = { x: e.clientX, y: e.clientY };
      ctxItem.value = item;
    }
  });

  // Listen for FAB click from layout
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const handler = () => { showUpload.value = !showUpload.value; };
    window.addEventListener('stora:fab-click', handler);
    return () => window.removeEventListener('stora:fab-click', handler);
  });

  // Keyboard shortcuts (P1.10)
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      if (e.key === "Delete" || e.key === "Backspace") {
        if (selIds.value.length > 0 && confirm(`确认删除 ${selIds.value.length} 项？`)) {
          api.post("/files/batch/delete", { file_ids: [...selIds.value] }).catch(() => {});
          selIds.value = [];
          refresh();
        }
      }
      if ((e.ctrlKey || e.metaKey) && e.key === "a") {
        e.preventDefault();
        selIds.value = allItems.map(x => x.id);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  });

  // Load folder tree when move dialog opens
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ track }) => {
    track(() => showMoveDialog.value);
    if (showMoveDialog.value && moveTree.value.length === 0) {
      moveTreeLoading.value = true;
      api.get<any[]>("/files/folders/tree").then(d => { moveTree.value = d || []; }).catch(() => {}).finally(() => moveTreeLoading.value = false);
    }
  });

  const refresh = () => { location.href = `/drive${folderId ? `?folder=${folderId}` : ""}`; };

  return (
    <div class="flex flex-col h-full">
      {/* Toolbar — responsive: icons on mobile, labels on desktop */}
      <div class="flex items-center gap-2 sm:gap-3 px-4 sm:px-6 py-3 border-b border-slate-200 bg-white shrink-0 flex-wrap min-h-[56px]">
        {/* Search — collapsible on mobile */}
        <div class="flex items-center gap-2 flex-1 min-w-0">
          <div class="relative flex-1 max-w-sm">
            <Icon name="search" size={16} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none" />
            <input type="text" placeholder="搜索文件名..."
              onKeyDown$={(e: any) => { if (e.key === "Enter" && e.target.value) nav(`/drive?search=${e.target.value}`); }}
              onFocus$={async () => { try { const h = await api.get('/files/search/history?limit=5'); if (h?.length) searchHistory.value = h; } catch {} }}
              onBlur$={() => setTimeout(() => searchHistory.value = [], 200)}
              class="w-full sm:w-64 pl-9 pr-3 py-2 text-sm rounded-lg border border-slate-200 bg-slate-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent focus:bg-white placeholder:text-slate-400 transition-all" />
            {searchHistory.value.length > 0 && (
              <div class="absolute top-full left-0 right-0 mt-1 bg-white rounded-lg border border-slate-200 shadow-lg z-50 py-1">
                {searchHistory.value.map((h: any) => (
                  <button key={h.keyword} onClick$={() => nav(`/drive?search=${h.keyword}`)}
                    class="w-full text-left px-3 py-2 text-sm text-slate-600 hover:bg-slate-50 flex items-center gap-2">
                    <Icon name="search" size={12} class="text-slate-400" />
                    <span>{h.keyword}</span>
                    <span class="text-xs text-slate-400 ml-auto">{h.results_count} 项</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* View mode toggle */}
        <div class="flex items-center gap-1 bg-slate-100 rounded-lg p-0.5 shrink-0">
          <button onClick$={() => viewMode.value = "list"}
            class={`p-1.5 rounded-md ${viewMode.value === "list" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400"}`}><Icon name="list" size={18} /></button>
          <button onClick$={() => viewMode.value = "grid"}
            class={`p-1.5 rounded-md ${viewMode.value === "grid" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400"}`}><Icon name="grid" size={18} /></button>
        </div>

        {/* Desktop action buttons — hidden on mobile (FAB takes over) */}
        <Button variant="primary" size="sm" onClick$={() => showUpload.value = !showUpload.value} class="hidden sm:inline-flex">
          <Icon name="upload" size={16} /> 上传
        </Button>
        <Button variant="secondary" size="sm" onClick$={() => { showNewFolder.value = true; newFolderName.value = ""; }} class="hidden sm:inline-flex">
          <Icon name="plus" size={16} /> 新建文件夹
        </Button>

        {/* Mobile compact actions */}
        <button onClick$={() => showUpload.value = !showUpload.value} class="sm:hidden p-2 rounded-lg text-slate-500 hover:bg-slate-100" aria-label="上传">
          <Icon name="upload" size={20} />
        </button>
        <button onClick$={() => { showNewFolder.value = true; newFolderName.value = ""; }} class="sm:hidden p-2 rounded-lg text-slate-500 hover:bg-slate-100" aria-label="新建文件夹">
          <Icon name="plus" size={20} />
        </button>
      </div>

      {/* Mobile bottom action bar for batch operations */}
      {selIds.value.length > 0 && (
        <div class="bottom-action-bar lg:hidden">
          <span class="text-sm font-medium text-slate-700 shrink-0">{selIds.value.length} 项</span>
          <div class="flex-1" />
          <button onClick$={async () => { showMoveDialog.value = true; }}
            class="touch-target px-3 py-1.5 text-xs font-medium text-indigo-600 bg-indigo-50 rounded-lg">移动</button>
          <button onClick$={async () => { if (selIds.value.length > 0) batchDownload([...selIds.value]); }}
            class="touch-target px-3 py-1.5 text-xs font-medium text-slate-600 bg-slate-100 rounded-lg">下载</button>
          <button onClick$={async () => { showShareDialog.value = true; }}
            class="touch-target px-3 py-1.5 text-xs font-medium text-indigo-600 bg-indigo-50 rounded-lg">分享</button>
          <button onClick$={async () => { if (!confirm("确认删除？")) return; api.post("/files/batch/delete", { file_ids: [...selIds.value] }).catch(() => {}); selIds.value = []; refresh(); }}
            class="touch-target px-3 py-1.5 text-xs font-medium text-red-600 bg-red-50 rounded-lg">删除</button>
        </div>
      )}

      {/* Desktop batch operations — inline */}
      {selIds.value.length > 0 && (
        <div class="hidden lg:flex items-center gap-2 px-6 py-2 bg-indigo-50/80 border-b border-indigo-100 shrink-0 animate-slide-down">
          <span class="text-sm font-medium text-indigo-700">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={async () => { showMoveDialog.value = true; }}
            class="px-3 py-1.5 text-xs font-medium text-indigo-600 hover:bg-indigo-100 rounded-lg transition-colors">移动</button>
          <button onClick$={async () => { if (selIds.value.length > 0) batchDownload([...selIds.value]); }}
            class="px-3 py-1.5 text-xs font-medium text-indigo-600 hover:bg-indigo-100 rounded-lg transition-colors">下载</button>
          <button onClick$={async () => { showShareDialog.value = true; }}
            class="px-3 py-1.5 text-xs font-medium text-indigo-600 hover:bg-indigo-100 rounded-lg transition-colors">分享</button>
          <button onClick$={async () => { if (!confirm("确认删除？")) return; api.post("/files/batch/delete", { file_ids: [...selIds.value] }).catch(() => {}); selIds.value = []; refresh(); }}
            class="px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 rounded-lg transition-colors">删除</button>
        </div>
      )}

      {/* Upload zone */}
      {showUpload.value && (
        <div class="px-4 sm:px-6 py-3 border-b bg-white"
          preventdefault:dragover preventdefault:drop
          onDragOver$={() => document.getElementById("drop-hint")!.classList.remove("hidden")}
          onDragLeave$={() => document.getElementById("drop-hint")!.classList.add("hidden")}
          onDrop$={async (e: DragEvent) => {
            document.getElementById("drop-hint")!.classList.add("hidden");
            for (const f of e.dataTransfer?.files || []) await uploadFile(f, folderId).catch(() => {});
            refresh();
          }}
        >
          <div id="drop-hint" class="hidden border-2 border-dashed border-indigo-300 bg-indigo-50 rounded-xl p-4 text-center text-indigo-600 text-sm mb-3">📥 释放以上传文件</div>
          <div class="flex items-center gap-3">
            <input type="file" multiple class="text-sm" onChange$={async (e: any) => {
              for (const f of e.target.files || []) await uploadFile(f, folderId).catch(() => {});
              refresh();
            }} />
            <span class="text-xs text-slate-400">或拖拽文件到上方虚线区域</span>
          </div>
        </div>
      )}

      {/* Breadcrumb */}
      <div class="flex items-center gap-1.5 px-4 sm:px-6 py-2.5 text-sm border-b border-slate-100 bg-white/80 shrink-0 overflow-x-auto">
        {folderId && (
          <button onClick$={() => {
            const idx = (data.value?.path || []).findIndex(p => p.id === folderId);
            const parent = idx > 0 ? data.value?.path[idx - 1] : null;
            nav(parent ? `/drive?folder=${parent.id}` : "/drive");
          }} class="flex items-center gap-1 px-2 py-1 rounded text-slate-500 hover:text-indigo-600 hover:bg-slate-100 transition-colors mr-1 shrink-0 touch-target">
            <Icon name="chevronLeft" size={14} /> 返回
          </button>
        )}
        <a href="/drive" class="text-slate-500 hover:text-indigo-600 font-medium shrink-0">我的文件</a>
        {data.value?.path?.slice(1).map(p => (
          <span class="flex items-center gap-1.5 min-w-0" key={p.id}>
            <Icon name="chevronRight" size={14} class="text-slate-300 shrink-0" />
            <a href={`/drive?folder=${p.id}`} class="text-slate-500 hover:text-indigo-600 truncate">{p.name}</a>
          </span>
        ))}
        {allItems.length > 0 && <><Icon name="chevronRight" size={14} class="text-slate-300 shrink-0" /><span class="text-slate-400 shrink-0">{allItems.length} 项</span></>}
      </div>

      {/* New Folder Dialog */}
      {showNewFolder.value && (
        <div class="px-4 sm:px-6 py-3 border-b bg-white flex items-center gap-3">
          <input type="text" bind:value={newFolderName} placeholder="文件夹名称"
            class="flex-1 max-w-xs px-3 py-2 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            onKeyDown$={async (e: any) => {
              if (e.key === "Enter" && newFolderName.value) {
                try { await createFolder(newFolderName.value, folderId); showNewFolder.value = false; refresh(); } catch {}
              }
            }} />
          <Button size="sm" onClick$={async () => {
            if (newFolderName.value) { await createFolder(newFolderName.value, folderId).catch(() => {}); showNewFolder.value = false; refresh(); }
          }}>创建</Button>
          <Button variant="ghost" size="sm" onClick$={() => showNewFolder.value = false}>取消</Button>
        </div>
      )}

      {/* Rename Dialog */}
      {renameId.value > 0 && (
        <div class="px-4 sm:px-6 py-3 border-b bg-white flex items-center gap-3">
          <input type="text" bind:value={renameVal} placeholder="新文件名"
            class="flex-1 max-w-xs px-3 py-2 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            onKeyDown$={async (e: any) => {
              if (e.key === "Enter" && renameVal.value) {
                try { await updateFile(renameId.value, { filename: renameVal.value }); renameId.value = 0; refresh(); } catch {}
              }
            }} />
          <Button size="sm" onClick$={async () => {
            if (renameVal.value) { await updateFile(renameId.value, { filename: renameVal.value }).catch(() => {}); renameId.value = 0; refresh(); }
          }}>确认</Button>
          <Button variant="ghost" size="sm" onClick$={() => renameId.value = 0}>取消</Button>
        </div>
      )}

      {/* Filter tabs */}
      <div class="flex gap-1 px-4 sm:px-6 py-2 border-b border-slate-100 bg-white/60 shrink-0 overflow-x-auto scrollbar-thin">
        {["all", "image", "video", "audio", "document", "archive"].map(ft => (
          <button key={ft} onClick$={() => { const p = folderId ? `?folder=${folderId}${ft !== "all" ? `&type=${ft}` : ""}` : `${ft !== "all" ? `?type=${ft}` : ""}`; nav(`/drive${p}`); }}
            class={`px-3 py-1.5 text-xs font-medium rounded-full transition-colors whitespace-nowrap touch-target ${(!loc.url.searchParams.get("type") && ft === "all") || loc.url.searchParams.get("type") === ft ? "bg-indigo-100 text-indigo-700" : "text-slate-500 hover:text-slate-700 hover:bg-slate-100"}`}>
            {{ all: "全部", image: "🖼 图片", video: "🎬 视频", audio: "🎵 音频", document: "📄 文档", archive: "📦 压缩包", other: "📎 其他" }[ft] || ft}
          </button>
        ))}
      </div>

      {/* Content */}
      <div class={`flex-1 overflow-auto scrollbar-thin ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {!data.value ? (
          <div class="p-6 space-y-3">{[1,2,3,4,5].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><Skeleton class="w-5 h-5 rounded" /><Skeleton class="w-10 h-10 rounded-lg" /><div class="flex-1 space-y-2"><Skeleton class="h-4 w-48" /><Skeleton class="h-3 w-24" /></div></div>)}</div>
        ) : allItems.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400 p-8 text-center">
            <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">📂</div>
            <h3 class="text-lg font-semibold text-slate-500 mb-1">空目录</h3>
            <p class="text-sm text-slate-400 mb-6">拖拽文件到此处，或点击上传按钮</p>
            <Button variant="primary" class="hidden sm:inline-flex" onClick$={() => showUpload.value = true}><Icon name="upload" size={16} /> 上传文件</Button>
            <button onClick$={() => showUpload.value = true} class="sm:hidden touch-target px-6 py-3 bg-indigo-600 text-white rounded-xl text-sm font-medium">上传文件</button>
          </div>
        ) : viewMode.value === "list" ? <ListView items={allItems} selIds={selIds} renameId={renameId} renameVal={renameVal} nav={nav} folderId={folderId}
          onContextItem$={(item: any, e: any) => openCtx(item, e)} /> : <GridView items={allItems} selIds={selIds} nav={nav} folderId={folderId}
          onContextItem$={(item: any, e: any) => openCtx(item, e)} />}
      </div>

      {/* Desktop context menu overlay */}
      {ctxItem.value && !showActionSheet.value && (
        <>
          <div class="fixed inset-0 z-50" onClick$={() => ctxItem.value = null} onContextMenu$={(e: any) => { e.preventDefault(); ctxItem.value = null; }} />
          <div class="fixed z-50 min-w-[180px] bg-white rounded-xl shadow-lg border border-slate-200 py-1.5"
            style={{ left: `${ctxPos.value.x}px`, top: `${ctxPos.value.y}px` }}>
            {ctxItem.value.type === "file" ? (
              <>
                <button onClick$={() => { nav(`/view?id=${ctxItem.value!.id}`); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">👁 预览</button>
                <button onClick$={() => { window.open(`/api/v2/files/download/${ctxItem.value!.id}`, "_blank"); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">⬇ 下载</button>
                <div class="h-px bg-slate-100 my-1" />
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">✏ 重命名</button>
                <button onClick$={async () => {
                  const id = ctxItem.value!.id;
                  ctxItem.value = null;
                  updateFile(id, { is_favorite: true }).catch(() => {});
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">⭐ 收藏</button>
                <div class="h-px bg-slate-100 my-1" />
                <button onClick$={async () => {
                  if (confirm("删除?")) { await deleteFile(ctxItem.value!.id).catch(() => {}); location.reload(); }
                  ctxItem.value = null;
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-red-600 hover:bg-red-50 touch-target">🗑 删除</button>
              </>
            ) : (
              <>
                <button onClick$={() => { nav(`/drive?folder=${ctxItem.value!.id}`); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">📂 打开</button>
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 touch-target">✏ 重命名</button>
                <div class="h-px bg-slate-100 my-1" />
                <button onClick$={async () => {
                  if (confirm("删除?")) { await deleteFolder(ctxItem.value!.id).catch(() => {}); location.reload(); }
                  ctxItem.value = null;
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-red-600 hover:bg-red-50 touch-target">🗑 删除</button>
              </>
            )}
          </div>
        </>
      )}

      {/* Mobile action sheet (replaces context menu on small screens) */}
      {ctxItem.value && showActionSheet.value && (
        <>
          <div class="action-sheet-overlay" onClick$={() => { ctxItem.value = null; showActionSheet.value = false; }} />
          <div class="action-sheet">
            <div class="w-10 h-1 bg-slate-300 rounded-full mx-auto mb-3" />
            <p class="text-xs text-slate-400 text-center mb-3">{ctxItem.value.type === "file" ? ctxItem.value.name : ctxItem.value.name}</p>
            {ctxItem.value.type === "file" ? (
              <div class="space-y-1">
                <button onClick$={() => { nav(`/view?id=${ctxItem.value!.id}`); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">👁 预览</button>
                <button onClick$={() => { window.open(`/api/v2/files/download/${ctxItem.value!.id}`, "_blank"); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">⬇ 下载</button>
                <div class="h-px bg-slate-100 my-1" />
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">✏ 重命名</button>
                <button onClick$={async () => { await updateFile(ctxItem.value!.id, { is_favorite: true }).catch(() => {}); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">⭐ 收藏</button>
                <div class="h-px bg-slate-100 my-1" />
                <button onClick$={async () => { if (confirm("删除?")) { await deleteFile(ctxItem.value!.id).catch(() => {}); location.reload(); } ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-red-600 hover:bg-red-50 rounded-lg touch-target">🗑 删除</button>
              </div>
            ) : (
              <div class="space-y-1">
                <button onClick$={() => { nav(`/drive?folder=${ctxItem.value!.id}`); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">📂 打开</button>
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">✏ 重命名</button>
                <div class="h-px bg-slate-100 my-1" />
                <button onClick$={async () => { if (confirm("删除?")) { await deleteFolder(ctxItem.value!.id).catch(() => {}); location.reload(); } ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-red-600 hover:bg-red-50 rounded-lg touch-target">🗑 删除</button>
              </div>
            )}
            <button onClick$={() => { ctxItem.value = null; showActionSheet.value = false; }}
              class="w-full mt-3 px-4 py-3 text-sm font-medium text-slate-500 bg-slate-100 hover:bg-slate-200 rounded-xl touch-target">取消</button>
          </div>
        </>
      )}

      {/* Folder picker modal for batch move */}
      {showMoveDialog.value && (
        <>
          <div class="fixed inset-0 z-50 bg-black/40" onClick$={() => showMoveDialog.value = false} />
          <div class="fixed z-50 bottom-0 sm:top-1/2 sm:left-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2 w-full sm:w-96 sm:max-h-[70vh] bg-white rounded-t-2xl sm:rounded-2xl shadow-xl flex flex-col">
            <div class="flex items-center justify-between px-5 py-4 border-b border-slate-100">
              <h3 class="text-sm font-semibold text-slate-900">移动到文件夹</h3>
              <button onClick$={() => showMoveDialog.value = false} class="touch-target p-1.5 rounded-lg hover:bg-slate-100 text-slate-400">
                <Icon name="close" size={18} />
              </button>
            </div>
            <div class="flex-1 overflow-auto p-4 max-h-[50vh]">
              <button onClick$={async () => {
                const ids = [...selIds.value]; selIds.value = [];
                showMoveDialog.value = false;
                try { await api.post('/files/batch/move', { file_ids: ids }); refresh(); } catch {}
              }} class="w-full text-left px-3 py-2.5 rounded-lg text-sm text-slate-700 hover:bg-slate-50 flex items-center gap-3 touch-target mb-1">
                <Icon name="folder" size={16} class="text-amber-500" />
                <span>根目录（我的文件）</span>
              </button>
              {moveTree.value.length > 0 && (
                <div class="space-y-0.5">
                  {moveTree.value.map((node: any) => (
                    <button key={node.id} onClick$={async () => {
                      const ids = [...selIds.value]; selIds.value = [];
                      showMoveDialog.value = false;
                      try { await api.post('/files/batch/move', { file_ids: ids, target_folder_id: node.id }); refresh(); } catch {}
                    }} class="w-full text-left px-3 py-2.5 rounded-lg text-sm text-slate-700 hover:bg-slate-50 flex items-center gap-3 touch-target">
                      <Icon name="folder" size={16} class="text-amber-500" />
                      <span class="truncate">{node.name}</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </>
      )}

      {/* Share dialog for batch share */}
      {showShareDialog.value && (
        <>
          <div class="fixed inset-0 z-50 bg-black/40" onClick$={() => { showShareDialog.value = false; shareResult.value = []; }} />
          <div class="fixed z-50 bottom-0 sm:top-1/2 sm:left-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2 w-full sm:w-96 bg-white rounded-t-2xl sm:rounded-2xl shadow-xl flex flex-col p-5">
            <h3 class="text-sm font-semibold text-slate-900 mb-3">批量分享</h3>
            {shareResult.value.length === 0 ? (
              <>
                <p class="text-xs text-slate-500 mb-4">将为 {selIds.value.length} 个文件创建分享链接</p>
                <button onClick$={async () => {
                  const results: { code: string; url: string }[] = [];
                  for (const id of selIds.value) {
                    try {
                      const link = await createShare({ file_id: id, permission: 'read' });
                      results.push({ code: link.short_code, url: `${window.location.origin}/s/${link.short_code}` });
                    } catch {}
                  }
                  shareResult.value = results;
                }} class="w-full touch-target px-4 py-3 bg-indigo-600 text-white text-sm font-medium rounded-xl hover:bg-indigo-700 transition-colors text-center">
                  创建分享链接
                </button>
                <button onClick$={() => { showShareDialog.value = false; shareResult.value = []; }}
                  class="w-full mt-2 touch-target px-4 py-3 text-sm text-slate-500 rounded-xl hover:bg-slate-50 transition-colors text-center">取消</button>
              </>
            ) : (
              <>
                <p class="text-xs text-green-600 mb-3">已创建 {shareResult.value.length} 个分享链接</p>
                <div class="max-h-48 overflow-auto space-y-2 mb-3">
                  {shareResult.value.map((r, i) => (
                    <div key={i} class="flex items-center gap-2 bg-slate-50 rounded-lg px-3 py-2">
                      <span class="text-xs text-slate-500 truncate flex-1">{r.url}</span>
                      <button onClick$={() => navigator.clipboard.writeText(r.url)} class="touch-target text-xs text-indigo-600 hover:text-indigo-800 shrink-0">复制</button>
                    </div>
                  ))}
                </div>
                <button onClick$={() => { showShareDialog.value = false; shareResult.value = []; }}
                  class="w-full touch-target px-4 py-3 text-sm font-medium text-slate-600 bg-slate-100 rounded-xl hover:bg-slate-200 transition-colors text-center">完成</button>
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
});

// ─── List View ───

export const ListView = component$<{ items: any[]; selIds: any; renameId: any; renameVal: any; nav: any; folderId?: number; onContextItem$?: any }>(({ items, selIds, renameId, renameVal, nav, folderId, onContextItem$ }) => {
  const sortBy = useSignal("");
  const sortOrder = useSignal("");
  const loc = useLocation();
  sortBy.value = loc.url.searchParams.get("sort_by") || "created_at";
  sortOrder.value = loc.url.searchParams.get("sort_order") || "desc";

  const sortIcon = (field: string) => {
    if (sortBy.value !== field) return "↕";
    return sortOrder.value === "asc" ? "↑" : "↓";
  };
  const sortUrl = $((field: string) => {
    const order = sortBy.value === field && sortOrder.value === "desc" ? "asc" : "desc";
    const params = new URLSearchParams(loc.url.search);
    params.set("sort_by", field);
    params.set("sort_order", order);
    return `/drive?${params.toString()}`;
  });

  return (
  <div class="overflow-x-auto">
  <table class="w-full min-w-[600px]">
    <thead>
      <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 sticky top-0 bg-slate-50/95 backdrop-blur">
        <th class="w-10 px-4 py-3"><input type="checkbox" checked={selIds.value.length === items.length && items.length > 0}
          onChange$={() => selIds.value = selIds.value.length === items.length ? [] : items.map((x: any) => x.id)} class="rounded border-slate-300" /></th>
        <th class="px-2 py-3 cursor-pointer hover:text-indigo-600 select-none" onClick$={() => nav(sortUrl("filename"))}>文件名 <span class="text-slate-300 ml-1">{sortIcon("filename")}</span></th>
        <th class="px-2 py-3 w-28 cursor-pointer hover:text-indigo-600 select-none" onClick$={() => nav(sortUrl("file_size"))}>大小 <span class="text-slate-300 ml-1">{sortIcon("file_size")}</span></th>
        <th class="px-2 py-3 w-24">类型</th>
        <th class="px-2 py-3 w-40">操作</th>
      </tr>
    </thead>
    <tbody class="divide-y divide-slate-50">
      {items.map((item: any) => {
        if (item.t === "f") {
          const sel = selIds.value.includes(item.id);
          return (
            <tr key={`f-${item.id}`} draggable class={`group text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}
              onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
              onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "folder", name: item.name }, e); }}>
              <td class="px-4 py-3" onClick$={(e: any) => e.stopPropagation()}>
                <input type="checkbox" checked={sel}
                  onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }} class="rounded border-slate-300" />
              </td>
              <td class="px-2 py-3 cursor-pointer" onClick$={() => nav(`/drive?folder=${item.id}`)}>
                <div class="flex items-center gap-3"><div class="w-9 h-9 rounded-lg bg-amber-50 text-amber-600 flex items-center justify-center text-sm">📁</div><span class="text-slate-700 font-medium">{item.name}</span></div>
              </td>
              <td class="px-2 py-3 text-slate-500">—</td>
              <td class="px-2 py-3"><span class="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-600">folder</span></td>
              <td class="px-2 py-3"><div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity"><Button variant="ghost" size="sm" onClick$={() => { renameId.value = item.id; renameVal.value = item.name; }}>重命名</Button><Button variant="ghost" size="sm" class="!text-red-500" onClick$={async () => { if (confirm("删除?")) { await deleteFolder(item.id).catch(() => {}); location.reload(); } }}>删除</Button></div></td>
            </tr>
          );
        }
        const sel = selIds.value.includes(item.id);
        const tc = typeMeta[item.file_type] || typeMeta.other;
        return (
          <tr key={item.id} draggable class={`group text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}
            onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
            onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "file", name: item.filename, fileType: item.file_type }, e); }}>
            <td class="px-4 py-3"><input type="checkbox" checked={sel}
              onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }} class="rounded border-slate-300" /></td>
            <td class="px-2 py-3 cursor-pointer" onClick$={() => nav(`/view?id=${item.id}`)}>
              <div class="flex items-center gap-3">
                <div class={`w-9 h-9 rounded-lg flex items-center justify-center text-sm shrink-0 ${tc.color}`}>{tc.icon}</div>
                <span class="text-slate-700 truncate max-w-xs font-medium">{item.filename}</span>
              </div>
            </td>
            <td class="px-2 py-3 text-slate-500">{fmtSize(item.file_size)}</td>
            <td class="px-2 py-3"><span class="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-600">{item.file_type}</span></td>
            <td class="px-2 py-3">
              <div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <Button variant="ghost" size="sm" onClick$={() => nav(`/view?id=${item.id}`)}>预览</Button>
                <Button variant="ghost" size="sm" onClick$={() => {
                  const newVal = !item.is_favorite;
                  updateFile(item.id, { is_favorite: newVal }).then(() => { item.is_favorite = newVal; }).catch(() => {});
                }}>{item.is_favorite ? "★" : "☆"}</Button>
                <Button variant="ghost" size="sm" onClick$={() => { renameId.value = item.id; renameVal.value = item.filename; }}>重命名</Button>
                <Button variant="ghost" size="sm" class="!text-red-500" onClick$={async () => { if (confirm("删除?")) { await deleteFile(item.id).catch(() => {}); location.reload(); } }}>删除</Button>
              </div>
            </td>
          </tr>
        );
      })}
    </tbody>
  </table>
  </div>
  );
});

// ─── Grid View ───

export const GridView = component$<{ items: any[]; selIds: any; nav: any; folderId?: number; onContextItem$?: any }>(({ items, selIds, nav, onContextItem$ }) => (
  <div class="card-grid p-4 sm:p-6">
    {items.map((item: any) => {
      if (item.t === "f") return (
        <div key={`f-${item.id}`} draggable onClick$={() => nav(`/drive?folder=${item.id}`)}
          onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
          onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "folder", name: item.name }, e); }}
          class="bg-white rounded-xl border-2 border-slate-100 hover:border-amber-300 hover:shadow-sm transition-all cursor-pointer p-3">
          <div class="aspect-square rounded-lg bg-amber-50 flex items-center justify-center text-5xl mb-2.5">📁</div>
          <p class="text-xs font-medium text-slate-700 truncate text-center">{item.name}</p>
        </div>
      );
      const sel = selIds.value.includes(item.id);
      const tc = typeMeta[item.file_type] || typeMeta.other;
      return (
        <div key={item.id} draggable onClick$={() => nav(`/view?id=${item.id}`)}
          onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
          onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "file", name: item.filename, fileType: item.file_type }, e); }}
          class={`bg-white rounded-xl border-2 transition-all cursor-pointer p-3 relative ${sel ? "border-indigo-500 shadow-md" : "border-slate-100 hover:border-slate-200 hover:shadow-sm"}`}>
          <div class={`aspect-square rounded-lg flex items-center justify-center text-4xl mb-2.5 overflow-hidden ${tc.color}`}>
            {item.file_type === "image" ? (
              <img src={`/api/v2/files/preview/${item.id}/thumbnail?size=256`} alt={item.filename}
                class="w-full h-full object-cover" loading="lazy" />
            ) : (
              <span>{tc.icon}</span>
            )}
          </div>
          <p class="text-xs font-medium text-slate-700 truncate text-center">{item.filename}</p>
          <p class="text-xs text-slate-400 text-center mt-0.5">{fmtSize(item.file_size)}</p>
          {sel && <div class="absolute top-2 right-2 w-5 h-5 bg-indigo-600 rounded-full flex items-center justify-center"><Icon name="check" size={12} class="text-white" /></div>}
        </div>
      );
    })}
  </div>
));
