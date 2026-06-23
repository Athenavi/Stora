/**
 * Stora Share Landing — flat centered card design
 * Route: /s/[code]
 * Features: file selection, save-to-drive with folder picker, download
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useLocation, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, isAuthenticated } from "~/lib/api";

interface FolderItem { id: number; filename: string; file_size: number; file_type: string; }
interface SubFolder { id: number; name: string; }
interface ShareInfo { id: number; short_code: string; permission: string; password_protected: boolean; is_folder?: boolean; is_batch?: boolean; file_count?: number; }
interface ShareAccess {
  share_info: ShareInfo;
  item?: { id?: number; filename?: string; file_size?: number; file_type?: string; mime_type?: string; name?: string; is_folder?: boolean };
  need_password?: boolean;
  protected?: boolean;
  folders?: SubFolder[];
  items?: any[];
}
interface FolderNode {
  id: number; name: string; parent_id?: number | null; children: FolderNode[];
}

export const useShareData = routeLoader$(async ({ params, request }) => {
  const code = params["code"];
  if (!code) return null;
  const srv = createServerApi(request);
  return await srv.get<ShareAccess>(`/files/shares/access/${code}`).catch(() => null);
});

function fmtSize(bytes: number | undefined): string {
  if (!bytes) return "";
  const k = 1024, i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

const typeIcon: Record<string, string> = { image: "🖼️", video: "🎬", audio: "🎵", document: "📄", archive: "📦", other: "📎" };

export default component$(() => {
  const data = useShareData();
  const loc = useLocation();
  const nav = useNavigate();
  const shareCode = loc.params["code"] || (data.value?.share_info?.short_code) || "";

  // Auth state
  const password = useSignal("");
  const error = useSignal("");
  const shareData = useSignal<ShareAccess | null>(data.value);
  const loading = useSignal(false);

  // Selection state
  const selFileIds = useSignal<Set<number>>(new Set());
  const selectAllChecked = useSignal(false);
  const expandedFolders = useSignal<Set<number>>(new Set());

  const toggleExpand = $((id: number) => {
    const s = new Set(expandedFolders.value);
    if (s.has(id)) s.delete(id); else s.add(id);
    expandedFolders.value = s;
  });

  // Auto-select all files on first load (except huge lists)
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ track }) => {
    track(() => shareData.value);
    const s = shareData.value;
    if (!s || s.need_password) return;
    const files = s.items || (s.item?.id ? [s.item] : []);
    if (files.length > 0 && files.length <= 100) {
      selFileIds.value = new Set(files.map((f: any) => f.id));
    }
  });

  const toggleFile = $((id: number) => {
    const s = new Set(selFileIds.value);
    if (s.has(id)) s.delete(id); else s.add(id);
    selFileIds.value = s;
  });

  const toggleAll = $((ids: number[]) => {
    if (selFileIds.value.size === ids.length) { selFileIds.value = new Set(); }
    else { selFileIds.value = new Set(ids); }
  });

  // Save-to-drive
  const showFolderPicker = useSignal(false);
  const folderTree = useSignal<FolderNode[]>([]);
  const targetFolderId = useSignal<number | undefined>(undefined);
  const targetFolderName = useSignal("根目录");
  const saving = useSignal(false);
  const saveDone = useSignal(false);

  const loadFolderTree = $(async () => {
    try {
      const res = await fetch("/api/v2/files/folders/tree");
      const json = await res.json();
      const d = json.data || json;
      folderTree.value = Array.isArray(d) ? d : [];
    } catch { folderTree.value = []; }
  });

  const renderFolderNode = (nodes: FolderNode[], depth = 0): any =>
    nodes.map(node => (
      <div key={node.id}>
        <button onClick$={() => { targetFolderId.value = node.id; targetFolderName.value = node.name; showFolderPicker.value = false; }}
          class={`w-full text-left px-3 py-2 text-sm flex items-center gap-2 hover:bg-stora-muted ${targetFolderId.value === node.id ? "bg-stora-primary/10 text-stora-primary" : "text-stora-foreground"}`}
          style={{ paddingLeft: `${12 + depth * 20}px` }}>
          <span>📁</span><span class="truncate">{node.name}</span>
        </button>
        {node.children?.length > 0 && renderFolderNode(node.children, depth + 1)}
      </div>
    ));

  const doSave = $(async (fileIds: number[]) => {
    if (!isAuthenticated()) {
      if (confirm("需要登录才能转存到你的 Stora。是否前往登录？")) {
        nav(`/login?redirect=${encodeURIComponent(loc.url.pathname)}`);
      }
      return;
    }
    if (!showFolderPicker.value) {
      showFolderPicker.value = true;
      await loadFolderTree();
      return;
    }
    saving.value = true;
    try {
      const res = await fetch(`/api/v2/share/${shareCode}/save`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_ids: fileIds, folder_id: targetFolderId.value || null }),
      });
      if (!res.ok) throw new Error("save failed");
      saveDone.value = true;
      showFolderPicker.value = false;
    } catch { error.value = "转存失败，请确认已登录"; }
    saving.value = false;
  });

  // ── Render ──

  if (!data.value) {
    return (
      <div class="min-h-screen bg-stora-background flex items-center justify-center">
        <div class="text-center text-stora-muted-foreground">
          <div class="text-6xl mb-4">🔗</div>
          <p class="text-lg font-medium text-stora-foreground">分享链接不存在或已失效</p>
        </div>
      </div>
    );
  }

  const s = shareData.value!;
  const item = s.item || {};
  const isFolder = s.share_info?.is_folder || !!item.is_folder;
  const isBatch = !!s.share_info?.is_batch;

  // Password gate
  if (s.need_password) {
    return (
      <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
        <div class="w-full max-w-sm bg-stora-card border border-stora-border p-8 text-center">
          <div class="text-5xl mb-4">🔒</div>
          <h1 class="text-xl font-semibold text-stora-foreground mb-2">此文件受密码保护</h1>
          <p class="text-sm text-stora-muted-foreground mb-6">请输入密码以访问</p>
          {error.value && <p class="text-sm text-stora-destructive mb-4">{error.value}</p>}
          <input type="password" bind:value={password} placeholder="输入密码"
            class="w-full h-12 px-4 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-primary mb-4" />
          <button onClick$={async () => {
            loading.value = true; error.value = "";
            try {
              const res = await (await fetch(`/api/v2/files/shares/access/${shareCode}?password=${encodeURIComponent(password.value)}`)).json();
              const d = res.data || res;
              if (d.need_password) { error.value = "密码错误"; return; }
              shareData.value = d;
            } catch (e: any) { error.value = "验证失败"; }
            finally { loading.value = false; }
          }} disabled={loading.value} class="w-full h-12 text-sm font-semibold text-white bg-stora-primary hover:bg-[#1D4ED8]">
            {loading.value ? "验证中..." : "确认"}
          </button>
        </div>
      </div>
    );
  }

  // Merge files from all sources
  // For folder shares, items has flat list with folder_id/is_folder — build tree
  const allFlatItems: any[] = (isFolder && s.items) || [];
  const rootFiles: FolderItem[] = !isFolder ? (s.items && s.items.length > 0 ? s.items : (s.item?.id ? [{ id: s.item.id, filename: s.item.filename || s.item.name || "", file_size: s.item.file_size || 0, file_type: s.item.file_type || "other" }] : [])) : [];
  const rootSubs: SubFolder[] = s.folders || [];

  // Build tree: group items by folder_id
  const childrenOf = new Map<number | null, any[]>();
  for (const item of allFlatItems) {
    const pid = item.folder_id ?? null;
    if (!childrenOf.has(pid)) childrenOf.set(pid, []);
    childrenOf.get(pid)!.push(item);
  }
  // Root-level items (folder_id = null or the shared folder's ID)
  const rootNodeId = s.item?.id ?? null;
  const treeItems = childrenOf.get(rootNodeId) || childrenOf.get(null) || [];

  // Recursive renderer — plain function, not $(), since it's only called during render
  const renderNode = (node: any, depth: number): any => {
    const isFolderNode = node.is_folder;
    const nodeId = node.id;
    const children = childrenOf.get(nodeId) || [];
    const isExpanded = expandedFolders.value.has(nodeId);
    const indent = depth * 24;

    return (
      <div key={nodeId}>
        <div onClick$={() => isFolderNode ? toggleExpand(nodeId) : toggleFile(nodeId)}
          class={`flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-stora-muted ${selFileIds.value.has(nodeId) ? "bg-stora-primary/5" : ""}`}
          style={{ paddingLeft: `${12 + indent}px` }}>
          {isFolderNode ? (
            <span class="w-4 shrink-0">{isExpanded ? "📂" : "📁"}</span>
          ) : (
            <>
              <input type="checkbox" checked={selFileIds.value.has(nodeId)}
                onClick$={(e: any) => e.stopPropagation()}
                onChange$={() => toggleFile(nodeId)} class="border-stora-border shrink-0" />
              <span>{typeIcon[node.file_type] || "📄"}</span>
            </>
          )}
          <span class="flex-1 truncate">{node.filename}</span>
          {!isFolderNode && <span class="text-xs text-stora-nav-text shrink-0">{fmtSize(node.file_size)}</span>}
          {isFolderNode && <span class="text-xs text-stora-muted-foreground">{children.length} 项</span>}
        </div>
        {isFolderNode && isExpanded && children.length > 0 && (
          <div>{children.map((c: any) => renderNode(c, depth + 1))}</div>
        )}
      </div>
    );
  };

  // Determine header
  const headerIcon = isBatch ? "📦" : isFolder ? "📁" : "📄";
  const headerName = isBatch ? "分享的文件" : isFolder ? (s.item?.filename || s.item?.name || "共享文件夹") : (s.item?.filename || "未命名文件");
  const totalFiles = isFolder ? allFlatItems.filter((x: any) => !x.is_folder).length : (rootFiles.length);

  return (
    <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
      <div class="w-full max-w-[560px] bg-stora-card border border-stora-border p-8">

        {/* Header */}
        <div class="flex items-center gap-3 mb-4">
          <div class="w-12 h-12 bg-stora-muted flex items-center justify-center text-2xl">{headerIcon}</div>
          <div class="flex-1 min-w-0">
            <h1 class="text-lg font-semibold text-stora-foreground truncate">{headerName}</h1>
            <p class="text-xs text-stora-muted-foreground">{totalFiles} 个文件</p>
          </div>
        </div>

        {/* Hierarchical tree view (for folder shares) */}
        {isFolder && allFlatItems.length > 0 ? (
          <div class="divide-y divide-stora-border border border-stora-border mb-4 max-h-[500px] overflow-y-auto">
            {treeItems.map((node: any) => renderNode(node, 0))}
          </div>
        ) : isFolder && allFlatItems.length === 0 ? (
          <div class="py-8 text-center text-sm text-stora-muted-foreground">此文件夹为空</div>
        ) : (
          <>
            {/* File list with checkboxes (for batch/single-file shares) */}
            {rootFiles.length > 0 ? (
              <div>
                {rootFiles.length > 1 && (
                  <div class="flex items-center gap-2 px-1 py-1.5 border-b border-stora-border mb-1">
                    <input type="checkbox"
                      checked={selFileIds.value.size === rootFiles.length}
                      onChange$={() => toggleAll(rootFiles.map(f => f.id))}
                      class="border-stora-border" />
                    <span class="text-xs text-stora-muted-foreground">
                      {selFileIds.value.size === 0 ? "全选" :
                       selFileIds.value.size === rootFiles.length ? "取消全选" :
                       `已选 ${selFileIds.value.size} 项`}
                    </span>
                  </div>
                )}
                <div class="divide-y divide-stora-border border border-stora-border mb-4 max-h-[400px] overflow-y-auto">
                  {rootFiles.map(f => (
                    <div key={f.id} onClick$={() => toggleFile(f.id)}
                      class={`flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-stora-muted ${selFileIds.value.has(f.id) ? "bg-stora-primary/5" : ""}`}>
                      <input type="checkbox" checked={selFileIds.value.has(f.id)}
                        onClick$={(e: any) => e.stopPropagation()}
                        onChange$={() => toggleFile(f.id)} class="border-stora-border shrink-0" />
                      <span>{typeIcon[f.file_type] || "📄"}</span>
                      <span class="flex-1 truncate">{f.filename}</span>
                      <span class="text-xs text-stora-nav-text shrink-0">{fmtSize(f.file_size)}</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div class="py-8 text-center text-sm text-stora-muted-foreground">暂无文件</div>
            )}
          </>
        )}

        {/* Actions */}
        {saveDone.value ? (
          <p class="text-sm text-green-600 text-center py-3">✅ 已转存到 {targetFolderName.value}</p>
        ) : (
          <div class="flex flex-col gap-3">
            <button onClick$={() => doSave([...selFileIds.value])}
              disabled={selFileIds.value.size === 0 || saving.value}
              class="w-full h-12 text-sm font-semibold text-white bg-stora-primary hover:bg-[#1D4ED8] disabled:opacity-50 disabled:cursor-not-allowed">
              {saving.value ? "转存中..." :
               showFolderPicker.value ? "选择文件夹后转存" :
               !isAuthenticated() ? "📥 登录后转存到我的 Stora" :
               `📥 转存到 ${targetFolderName.value} (${selFileIds.value.size} 项)`}
            </button>
            <button onClick$={() => { window.open(`/api/v2/share/${shareCode}/download`, "_blank"); }}
              class="w-full h-12 text-sm font-semibold text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-background">
              ⬇ 下载全部 (ZIP)
            </button>
          </div>
        )}

        {/* Folder picker dialog */}
        {showFolderPicker.value && (
          <>
            <div class="fixed inset-0 z-40 bg-black/30" onClick$={() => showFolderPicker.value = false} />
            <div class="fixed z-50 bottom-0 sm:top-1/2 sm:left-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2 w-full sm:w-96 bg-white flex flex-col max-h-[70vh]">
              <div class="flex items-center justify-between px-4 py-3 border-b border-stora-border">
                <span class="text-sm font-semibold text-stora-foreground">选择目标文件夹</span>
                <button onClick$={() => showFolderPicker.value = false} class="text-stora-muted-foreground hover:text-stora-foreground">✕</button>
              </div>
              <div class="flex-1 overflow-auto py-2">
                <button onClick$={() => { targetFolderId.value = undefined; targetFolderName.value = "根目录"; showFolderPicker.value = false; }}
                  class={`w-full text-left px-3 py-2 text-sm flex items-center gap-2 hover:bg-stora-muted ${targetFolderId.value === undefined ? "bg-stora-primary/10 text-stora-primary" : "text-stora-foreground"}`}>
                  <span>📂</span><span>根目录</span>
                </button>
                {folderTree.value.length === 0 ? (
                  <p class="text-xs text-stora-muted-foreground text-center py-4">加载中...</p>
                ) : renderFolderNode(folderTree.value)}
              </div>
              <div class="px-4 py-3 border-t border-stora-border">
                <button onClick$={() => doSave([...selFileIds.value])}
                  disabled={saving.value}
                  class="w-full h-10 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] disabled:opacity-50">
                  {saving.value ? "保存中..." : `转存到${targetFolderName.value}`}
                </button>
              </div>
            </div>
          </>
        )}

        {/* Brand trace */}
        <div class="flex items-center justify-center gap-2 mt-6 pt-4 border-t border-stora-border">
          <div class="w-6 h-6 bg-stora-primary flex items-center justify-center text-white text-sm font-bold">S</div>
          <span class="text-sm font-medium text-stora-muted-foreground">通过 Stora 安全分享</span>
        </div>
      </div>
    </div>
  );
});
