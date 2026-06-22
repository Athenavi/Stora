/**
 * Stora Drive — core file manager with rename, folder create, batch ops, drag-drop
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { createServerApi, getFolderChildrenByPath, createFolderByPath, updateFile, updateFolder, deleteFile, deleteFolder, moveFiles, uploadFile, batchDownload, createShare, api, type PathChildrenResponse, type FileItem } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button, Input } from "~/components/ui/Button";

export const useFileList = routeLoader$(async ({ url, request }) => {
  const folderPath = url.searchParams.get("Path") || "";
  const search = url.searchParams.get("search");
  const fileType = url.searchParams.get("type");
  const sortBy = url.searchParams.get("sort_by") || "created_at";
  const sortOrder = url.searchParams.get("sort_order") || "desc";
  const api = createServerApi(request);

  if (folderPath !== "") {
    const d = await api.get<PathChildrenResponse>(`/files/folders/by-path?path=${encodeURIComponent(folderPath.replace(/^\//, ''))}`).catch(() => null);
    if (!d) return { folders: [], files: [], path: ["我的文件"] };
    // Map folders to include full path for navigation
    const mappedFolders = (d.folders || []).map(f => ({ ...f, id: f.id, name: f.name, path: f.path }));
    return { folders: mappedFolders, files: d.files, path: d.path };
  }
  if (search) {
    const d = await api.get(`/files/search?q=${encodeURIComponent(search)}&page=1&page_size=50`).catch(() => null);
    if (!d) return null;
    const folders = (d.items || []).filter((x: any) => x.is_folder || x.file_type === "folder");
    const fileItems = (d.items || []).filter((x: any) => !x.is_folder && x.file_type !== "folder");
    return { folders, files: fileItems, path: [`搜索: ${search}`] };
  }
  const typeParam = fileType ? `&file_type=${fileType}` : "";
  const sortParam = `&sort_by=${sortBy}&sort_order=${sortOrder}`;
  const files = await api.get(`/files?page=1&page_size=50${typeParam}${sortParam}`).catch(() => null);
  return files ? { folders: [], files: files.items, path: ["我的文件"] } : null;
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
  const currentPath = loc.url.searchParams.get("Path") || "";
  const resolvedFolderId = useSignal<number | undefined>(undefined);

  const allItems: ({ t: "f" } & { id: number; name: string; path?: string } | { t: "d" } & FileItem)[] = [];
  if (data.value) {
    for (const f of data.value.folders || []) allItems.push({ t: "f" as const, ...f });
    for (const f of data.value.files || []) allItems.push({ t: "d" as const, ...f });
    // Extract folder_id from the path-based response
    const pathData = data.value as any;
    if (pathData.folder_id !== undefined) {
      resolvedFolderId.value = pathData.folder_id;
    }
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
  const sharePermission = useSignal<"read" | "download">("read");
  const sharePassword = useSignal("");
  const shareExpiry = useSignal<number | null>(null); // hours, null=永久
  const shareCreating = useSignal(false);
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

  const refresh = () => { location.href = `/drive${currentPath ? `?Path=${encodeURIComponent(currentPath)}` : ""}`; };

  return (
    <div class="flex flex-col h-full">
      {/* Toolbar — per spec: 40px height, search 320x40, buttons */}
      <div class="flex items-center gap-4 px-6 py-3 border-b border-stora-border bg-white shrink-0">
        {/* Search — 320x40 per spec */}
        <div class="relative flex-1 max-w-[320px]">
          <span class="absolute left-3 top-1/2 -translate-y-1/2 text-stora-nav-text text-sm pointer-events-none">🔍</span>
          <input type="text" placeholder="搜索文件..."
            onKeyDown$={(e: any) => { if (e.key === "Enter" && e.target.value) nav(`/drive?search=${e.target.value}`); }}
            onFocus$={async () => { try { const h = await api.get('/files/search/history?limit=5'); if (h?.length) searchHistory.value = h; } catch {} }}
            onBlur$={() => setTimeout(() => searchHistory.value = [], 200)}
            class="w-full h-10 pl-9 pr-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary" />
          {searchHistory.value.length > 0 && (
            <div class="absolute top-full left-0 right-0 mt-1 bg-white border border-stora-border z-50 py-1">
              {searchHistory.value.map((h: any) => (
                <button key={h.keyword} onClick$={() => nav(`/drive?search=${h.keyword}`)}
                  class="w-full text-left px-3 py-2 text-sm text-stora-foreground hover:bg-stora-muted flex items-center gap-2">
                  <span class="text-stora-nav-text">🔍</span>
                  <span>{h.keyword}</span>
                  <span class="text-xs text-stora-muted-foreground ml-auto">{h.results_count} 项</span>
                </button>
              ))}
            </div>
          )}
        </div>

        <div class="flex-1" />

        {/* Upload button — primary blue */}
        <button onClick$={() => showUpload.value = !showUpload.value}
          class="hidden sm:inline-flex items-center gap-2 h-10 px-4 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] active:bg-[#1E40AF]">
          <span>⬆</span> 上传文件
        </button>

        {/* New folder button — white outline */}
        <button onClick$={() => { showNewFolder.value = true; newFolderName.value = ""; }}
          class="hidden sm:inline-flex items-center gap-2 h-10 px-4 text-sm font-medium text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-background">
          <span>➕</span> 新建文件夹
        </button>

        {/* Mobile compact actions */}
        <button onClick$={() => showUpload.value = !showUpload.value} class="sm:hidden touch-target p-2 text-stora-muted-foreground hover:bg-stora-muted" aria-label="上传">
          <span>⬆</span>
        </button>
        <button onClick$={() => { showNewFolder.value = true; newFolderName.value = ""; }} class="sm:hidden touch-target p-2 text-stora-muted-foreground hover:bg-stora-muted" aria-label="新建文件夹">
          <span>➕</span>
        </button>
      </div>

      {/* Mobile bottom action bar for batch operations */}
      {selIds.value.length > 0 && (
        <div class="fixed bottom-0 left-0 right-0 z-40 bg-white border-t border-stora-border px-4 py-3 flex items-center gap-2 lg:hidden">
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
        <div class="hidden lg:flex items-center gap-2 px-6 py-2 bg-stora-muted border-b border-stora-border shrink-0">
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
            for (const f of e.dataTransfer?.files || []) await uploadFile(f, resolvedFolderId.value).catch(() => {});
            refresh();
          }}
        >
          <div id="drop-hint" class="hidden border-2 border-dashed border-indigo-300 bg-indigo-50 rounded-xl p-4 text-center text-indigo-600 text-sm mb-3">📥 释放以上传文件</div>
          <div class="flex items-center gap-3">
            <input type="file" multiple class="text-sm" onChange$={async (e: any) => {
              for (const f of e.target.files || []) await uploadFile(f, resolvedFolderId.value).catch(() => {});
              refresh();
            }} />
            <span class="text-xs text-slate-400">或拖拽文件到上方虚线区域</span>
          </div>
        </div>
      )}

      {/* Breadcrumb */}
      <div class="flex items-center gap-1.5 px-6 py-2.5 text-sm border-b border-stora-border bg-white shrink-0 overflow-x-auto">
        {currentPath && (
          <button onClick$={() => {
            const segments = currentPath.split("/").filter(Boolean);
            segments.pop();
            const parentPath = segments.join("/");
            nav(parentPath ? `/drive?Path=${encodeURIComponent(parentPath)}` : "/drive");
          }} class="flex items-center gap-1 px-2 py-1 text-stora-muted-foreground hover:text-stora-foreground hover:bg-stora-muted mr-1 shrink-0 touch-target">
            <span>←</span> 返回
          </button>
        )}
        <a href="/drive" class="text-stora-muted-foreground hover:text-stora-foreground font-medium shrink-0">我的文件</a>
        {data.value?.path?.slice(1).map((p: string, i: number) => {
          const segments = currentPath.split("/").filter(Boolean);
          const linkPath = segments.slice(0, i + 1).join("/");
          return (
            <span class="flex items-center gap-1.5 min-w-0" key={linkPath}>
              <span class="text-stora-nav-text shrink-0">/</span>
              <a href={`/drive?Path=${encodeURIComponent(linkPath)}`} class="text-stora-muted-foreground hover:text-stora-foreground truncate">{p}</a>
            </span>
          );
        })}
        {allItems.length > 0 && <><span class="text-stora-nav-text shrink-0">/</span><span class="text-stora-nav-text shrink-0">{allItems.length} 项</span></>}
      </div>

      {/* New Folder Dialog */}
      {showNewFolder.value && (
        <div class="px-4 sm:px-6 py-3 border-b bg-white flex items-center gap-3">
          <input type="text" bind:value={newFolderName} placeholder="文件夹名称"
            class="flex-1 max-w-xs px-3 py-2 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            onKeyDown$={async (e: any) => {
              if (e.key === "Enter" && newFolderName.value) {
                try { await createFolderByPath(newFolderName.value, currentPath); showNewFolder.value = false; refresh(); } catch {}
              }
            }} />
          <Button size="sm" onClick$={async () => {
            if (newFolderName.value) { await createFolderByPath(newFolderName.value, currentPath).catch(() => {}); showNewFolder.value = false; refresh(); }
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
      <div class="flex gap-1 px-6 py-2 border-b border-stora-border bg-white shrink-0 overflow-x-auto scrollbar-thin">
        {["all", "image", "video", "audio", "document", "archive"].map(ft => (
          <button key={ft} onClick$={() => { const p = currentPath ? `?Path=${encodeURIComponent(currentPath)}${ft !== "all" ? `&type=${ft}` : ""}` : `${ft !== "all" ? `?type=${ft}` : ""}`; nav(`/drive${p}`); }}
            class={`px-3 py-1.5 text-xs font-medium whitespace-nowrap touch-target ${(!loc.url.searchParams.get("type") && ft === "all") || loc.url.searchParams.get("type") === ft ? "bg-stora-primary text-white" : "text-stora-muted-foreground hover:text-stora-foreground hover:bg-stora-muted"}`}>
            {{ all: "全部", image: "🖼 图片", video: "🎬 视频", audio: "🎵 音频", document: "📄 文档", archive: "📦 压缩包", other: "📎 其他" }[ft] || ft}
          </button>
        ))}
      </div>

      {/* Content */}
      <div class={`flex-1 overflow-auto scrollbar-thin ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {!data.value ? (
          <div class="p-6 space-y-3">{[1,2,3,4,5].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><div class="w-5 h-5 bg-stora-muted" /><div class="w-10 h-10 bg-stora-muted" /><div class="flex-1 space-y-2"><div class="h-4 w-48 bg-stora-muted" /><div class="h-3 w-24 bg-stora-muted" /></div></div>)}</div>
        ) : allItems.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground p-8 text-center">
            <div class="w-20 h-20 bg-stora-muted flex items-center justify-center text-4xl mb-5">📂</div>
            <h3 class="text-lg font-semibold text-stora-foreground mb-1">空目录</h3>
            <p class="text-sm text-stora-muted-foreground mb-6">拖拽文件到此处，或点击上传按钮</p>
            <button onClick$={() => showUpload.value = true} class="px-6 py-3 bg-stora-primary text-white text-sm font-medium">上传文件</button>
          </div>
        ) : viewMode.value === "list" ? <ListView items={allItems} selIds={selIds} renameId={renameId} renameVal={renameVal} nav={nav} currentPath={currentPath}
          onContextItem$={(item: any, e: any) => openCtx(item, e)} /> : <GridView items={allItems} selIds={selIds} nav={nav} currentPath={currentPath}
          onContextItem$={(item: any, e: any) => openCtx(item, e)} />}
      </div>

      {/* Desktop context menu overlay */}
      {ctxItem.value && !showActionSheet.value && (
        <>
          <div class="fixed inset-0 z-50" onClick$={() => ctxItem.value = null} onContextMenu$={(e: any) => { e.preventDefault(); ctxItem.value = null; }} />
          <div class="fixed z-50 min-w-[180px] bg-white border border-stora-border"
            style={{ left: `${ctxPos.value.x}px`, top: `${ctxPos.value.y}px` }}>
            {ctxItem.value.type === "file" ? (
              <>
                <button onClick$={() => { nav(`/view?id=${ctxItem.value!.id}`); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">👁 预览</button>
                <button onClick$={() => { window.open(`/api/v2/files/download/${ctxItem.value!.id}`, "_blank"); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">⬇ 下载</button>
                <div class="h-px bg-stora-border my-1" />
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">✏ 重命名</button>
                <button onClick$={async () => {
                  const id = ctxItem.value!.id;
                  ctxItem.value = null;
                  updateFile(id, { is_favorite: true }).catch(() => {});
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">⭐ 收藏</button>
                <div class="h-px bg-stora-border my-1" />
                <button onClick$={async () => {
                  if (confirm("删除?")) { await deleteFile(ctxItem.value!.id).catch(() => {}); location.reload(); }
                  ctxItem.value = null;
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-destructive hover:bg-red-50 touch-target">🗑 删除</button>
              </>
            ) : (
              <>
                <button onClick$={() => { nav(`/drive?Path=${encodeURIComponent(item.path || ctxItem.value!.name)}`); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">📂 打开</button>
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; }}
                  class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">✏ 重命名</button>
                <button onClick$={async () => {
                  const id = ctxItem.value!.id;
                  ctxItem.value = null;
                  selIds.value = [id];
                  showShareDialog.value = true;
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">🔗 分享</button>
                <div class="h-px bg-stora-border my-1" />
                <button onClick$={async () => {
                  if (confirm("删除?")) { await deleteFolder(ctxItem.value!.id).catch(() => {}); location.reload(); }
                  ctxItem.value = null;
                }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-destructive hover:bg-red-50 touch-target">🗑 删除</button>
              </>
            )}
          </div>
        </>
      )}

      {/* Mobile action sheet (replaces context menu on small screens) */}
      {ctxItem.value && showActionSheet.value && (
        <>
          <div class="fixed inset-0 z-50 bg-black/40" onClick$={() => { ctxItem.value = null; showActionSheet.value = false; }} />
          <div class="fixed bottom-0 left-0 right-0 z-51 bg-white px-4 pb-6 pt-3">
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
                <button onClick$={() => { const p = (ctxItem.value as any)?.path || ctxItem.value!.name; nav(`/drive?Path=${encodeURIComponent(p)}`); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">📂 打开</button>
                <button onClick$={() => { doRename(ctxItem.value! as any); ctxItem.value = null; showActionSheet.value = false; }}
                  class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">✏ 重命名</button>
                <button onClick$={async () => {
                  const id = ctxItem.value!.id;
                  ctxItem.value = null; showActionSheet.value = false;
                  selIds.value = [id];
                  showShareDialog.value = true;
                }} class="w-full text-left px-4 py-3 text-sm flex items-center gap-3 text-slate-700 hover:bg-slate-50 rounded-lg touch-target">🔗 分享</button>
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
          <div class="fixed z-50 bottom-0 sm:top-1/2 sm:left-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2 w-full sm:w-96 sm:max-h-[70vh] bg-white flex flex-col">
            <div class="flex items-center justify-between px-5 py-4 border-b border-stora-border">
              <h3 class="text-sm font-semibold text-stora-foreground">移动到文件夹</h3>
              <button onClick$={() => showMoveDialog.value = false} class="touch-target p-1.5 text-stora-muted-foreground hover:bg-stora-muted">
                <span>✕</span>
              </button>
            </div>
            <div class="flex-1 overflow-auto p-4 max-h-[50vh]">
              <button onClick$={async () => {
                const ids = [...selIds.value]; selIds.value = [];
                showMoveDialog.value = false;
                try { await api.post('/files/batch/move', { file_ids: ids }); refresh(); } catch {}
              }} class="w-full text-left px-3 py-2.5 text-sm text-stora-foreground hover:bg-stora-muted flex items-center gap-3 touch-target mb-1">
                <span class="text-stora-accent">📁</span>
                <span>根目录（我的文件）</span>
              </button>
              {moveTree.value.length > 0 && (
                <div class="space-y-0.5">
                  {moveTree.value.map((node: any) => (
                    <button key={node.id} onClick$={async () => {
                      const ids = [...selIds.value]; selIds.value = [];
                      showMoveDialog.value = false;
                      try { await api.post('/files/batch/move', { file_ids: ids, target_folder_id: node.id }); refresh(); } catch {}
                    }} class="w-full text-left px-3 py-2.5 text-sm text-stora-foreground hover:bg-stora-muted flex items-center gap-3 touch-target">
                      <span class="text-stora-accent">📁</span>
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
          <div class="fixed z-50 bottom-0 sm:top-1/2 sm:left-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2 w-full sm:w-96 bg-white flex flex-col p-5 max-h-[85vh] overflow-auto">
            <h3 class="text-sm font-semibold text-stora-foreground mb-3">批量分享 ({selIds.value.length} 项)</h3>
            {shareResult.value.length === 0 ? (
              <>
                {/* Permission */}
                <label class="text-xs font-medium text-slate-700 mb-1">权限</label>
                <div class="flex gap-2 mb-3">
                  <button onClick$={() => sharePermission.value = "read"}
                    class={`flex-1 touch-target px-3 py-2 text-xs font-medium rounded-lg transition-colors ${sharePermission.value === "read" ? "bg-indigo-100 text-indigo-700" : "bg-slate-100 text-slate-600 hover:bg-slate-200"}`}>仅查看</button>
                  <button onClick$={() => sharePermission.value = "download"}
                    class={`flex-1 touch-target px-3 py-2 text-xs font-medium rounded-lg transition-colors ${sharePermission.value === "download" ? "bg-indigo-100 text-indigo-700" : "bg-slate-100 text-slate-600 hover:bg-slate-200"}`}>可下载</button>
                </div>

                {/* Expiry */}
                <label class="text-xs font-medium text-slate-700 mb-1">有效期</label>
                <div class="flex gap-2 mb-3 flex-wrap">
                  {[
                    { label: "1天", value: 24 },
                    { label: "7天", value: 168 },
                    { label: "30天", value: 720 },
                    { label: "永久", value: null },
                  ].map(opt => (
                    <button key={opt.label} onClick$={() => shareExpiry.value = opt.value}
                      class={`touch-target px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${shareExpiry.value === opt.value ? "bg-indigo-100 text-indigo-700" : "bg-slate-100 text-slate-600 hover:bg-slate-200"}`}>{opt.label}</button>
                  ))}
                </div>

                {/* Password */}
                <label class="text-xs font-medium text-slate-700 mb-1">密码保护（可选）</label>
                <input type="text" bind:value={sharePassword} placeholder="设置访问密码"
                  class="w-full mb-4 px-3 py-2 text-sm rounded-lg border border-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500" />

                <button onClick$={async () => {
                  shareCreating.value = true;
                  const results: { code: string; url: string }[] = [];
                  const params: any = { permission: sharePermission.value };
                  if (sharePassword.value) params.password = sharePassword.value;
                  if (shareExpiry.value) params.expires_in_hours = shareExpiry.value;
                  for (const id of selIds.value) {
                    try {
                      // Detect if ID belongs to a folder
                      const item = allItems.find(x => x.id === id);
                      const shareParams: any = { ...params };
                      if (item && (item as any).t === "f") {
                        shareParams.folder_id = id;
                      } else {
                        shareParams.file_id = id;
                      }
                      const link = await createShare(shareParams as any);
                      results.push({ code: link.short_code, url: `${window.location.origin}/s/${link.short_code}` });
                    } catch {}
                  }
                  shareResult.value = results;
                  shareCreating.value = false;
                }} disabled={shareCreating.value} class="w-full touch-target px-4 py-3 bg-indigo-600 text-white text-sm font-medium rounded-xl hover:bg-indigo-700 transition-colors text-center disabled:opacity-50">
                  {shareCreating.value ? "创建中..." : "创建分享链接"}
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

export const ListView = component$<{ items: any[]; selIds: any; renameId: any; renameVal: any; nav: any; currentPath?: string; onContextItem$?: any }>(({ items, selIds, renameId, renameVal, nav, currentPath, onContextItem$ }) => {
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
  <table class="w-full">
    <thead>
      <tr class="text-left text-xs font-semibold text-stora-muted-foreground bg-stora-muted">
        <th class="w-10 px-4 py-3 font-semibold"><input type="checkbox" checked={selIds.value.length === items.length && items.length > 0}
          onChange$={() => selIds.value = selIds.value.length === items.length ? [] : items.map((x: any) => x.id)} class="border-stora-border" /></th>
        <th class="px-2 py-3 font-semibold cursor-pointer hover:text-stora-primary select-none" onClick$={() => nav(sortUrl("filename"))}>名称 {sortIcon("filename")}</th>
        <th class="px-2 py-3 w-[100px] font-semibold cursor-pointer hover:text-stora-primary select-none" onClick$={() => nav(sortUrl("file_size"))}>大小 {sortIcon("file_size")}</th>
        <th class="px-2 py-3 w-[100px] font-semibold">类型</th>
        <th class="px-2 py-3 w-[140px] font-semibold">修改时间</th>
        <th class="px-2 py-3 w-[80px] font-semibold">操作</th>
      </tr>
    </thead>
    <tbody class="divide-y divide-stora-muted">
      {items.map((item: any) => {
        if (item.t === "f") {
          const sel = selIds.value.includes(item.id);
          return (
            <tr key={`f-${item.id}`} draggable class={`group text-sm h-12 ${sel ? "bg-stora-muted" : "hover:bg-stora-muted"}`}
              onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
              onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "folder", name: item.name }, e); }}>
              <td class="px-4" onClick$={(e: any) => e.stopPropagation()}>
                <input type="checkbox" checked={sel}
                  onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }} class="border-stora-border" />
              </td>
              <td class="px-2 cursor-pointer" onClick$={() => nav(`/drive?Path=${encodeURIComponent(item.path || item.name)}`)}>
                <div class="flex items-center gap-3">
                  <div class="w-7 h-7 flex items-center justify-center text-sm shrink-0">📁</div>
                  <span class="text-sm font-medium text-stora-foreground">{item.name}</span>
                </div>
              </td>
              <td class="px-2 text-sm text-stora-muted-foreground">—</td>
              <td class="px-2"><span class="text-xs text-stora-muted-foreground">文件夹</span></td>
              <td class="px-2 text-sm text-stora-muted-foreground">{item.created_at?.split("T")[0] || "—"}</td>
              <td class="px-2 text-stora-muted-foreground text-base font-semibold">⋯</td>
            </tr>
          );
        }
        const sel = selIds.value.includes(item.id);
        const tc = typeMeta[item.file_type] || typeMeta.other;
        return (
          <tr key={item.id} draggable class={`group text-sm h-12 ${sel ? "bg-stora-muted" : "hover:bg-stora-muted"}`}
            onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
            onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "file", name: item.filename, fileType: item.file_type }, e); }}>
            <td class="px-4"><input type="checkbox" checked={sel}
              onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }} class="border-stora-border" /></td>
            <td class="px-2 cursor-pointer" onClick$={() => nav(`/view?id=${item.id}`)}>
              <div class="flex items-center gap-3">
                <div class={`w-7 h-7 flex items-center justify-center text-sm shrink-0`}>{tc.icon}</div>
                <span class="text-sm font-medium text-stora-foreground truncate max-w-xs">{item.filename}</span>
              </div>
            </td>
            <td class="px-2 text-sm text-stora-muted-foreground">{fmtSize(item.file_size)}</td>
            <td class="px-2 text-sm text-stora-muted-foreground">{item.file_type || "未知"}</td>
            <td class="px-2 text-sm text-stora-muted-foreground">{item.created_at?.split("T")[0] || "—"}</td>
            <td class="px-2 text-stora-muted-foreground text-base font-semibold">⋯</td>
          </tr>
        );
      })}
    </tbody>
  </table>
  </div>
  );
});

// ─── Grid View ───

export const GridView = component$<{ items: any[]; selIds: any; nav: any; currentPath?: string; onContextItem$?: any }>(({ items, selIds, nav, onContextItem$ }) => (
  <div class="card-grid p-6">
    {items.map((item: any) => {
      if (item.t === "f") return (
        <div key={`f-${item.id}`} draggable onClick$={() => nav(`/drive?Path=${encodeURIComponent(item.path || item.name)}`)}
          onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
          onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "folder", name: item.name }, e); }}
          class="bg-stora-card border border-stora-border hover:border-stora-accent cursor-pointer p-3">
          <div class="aspect-square bg-stora-muted flex items-center justify-center text-5xl mb-2.5">📁</div>
          <p class="text-xs font-medium text-stora-foreground truncate text-center">{item.name}</p>
        </div>
      );
      const sel = selIds.value.includes(item.id);
      const tc = typeMeta[item.file_type] || typeMeta.other;
      return (
        <div key={item.id} draggable onClick$={() => nav(`/view?id=${item.id}`)}
          onDragStart$={(e: DragEvent) => { e.dataTransfer?.setData('text/plain', JSON.stringify({ fileIds: [item.id] })); }}
          onContextMenu$={(e: any) => { e.preventDefault(); onContextItem$({ id: item.id, type: "file", name: item.filename, fileType: item.file_type }, e); }}
          class={`bg-stora-card border cursor-pointer p-3 relative ${sel ? "border-stora-primary" : "border-stora-border hover:border-stora-muted-foreground"}`}>
          <div class={`aspect-square flex items-center justify-center text-4xl mb-2.5 overflow-hidden`}>
            {item.file_type === "image" ? (
              <img src={`/api/v2/files/preview/${item.id}/thumbnail?size=256`} alt={item.filename}
                class="w-full h-full object-cover" loading="lazy" />
            ) : (
              <span>{tc.icon}</span>
            )}
          </div>
          <p class="text-xs font-medium text-stora-foreground truncate text-center">{item.filename}</p>
          <p class="text-xs text-stora-muted-foreground text-center mt-0.5">{fmtSize(item.file_size)}</p>
          {sel && <div class="absolute top-2 right-2 w-5 h-5 bg-stora-primary flex items-center justify-center"><span class="text-white text-xs">✓</span></div>}
        </div>
      );
    })}
  </div>
));
