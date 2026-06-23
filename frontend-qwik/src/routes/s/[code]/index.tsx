/**
 * Stora Share Landing — hierarchical tree with pagination, auto-expand, lazy load
 * Route: /s/[code]
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useLocation, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, isAuthenticated } from "~/lib/api";

interface ShareInfo {
  id: number; short_code: string; permission: string;
  password_protected: boolean; is_folder?: boolean; is_batch?: boolean; file_count?: number;
}
interface FolderEntry {
  id: number; name: string; file_count: number; child_folder_count: number;
}
interface ShareAccess {
  share_info: ShareInfo;
  item?: any;
  items?: any[];
  folders?: FolderEntry[];
  total?: number; page?: number; per_page?: number;
  need_password?: boolean; protected?: boolean;
}
interface FolderNode {
  id: number; name: string; parent_id?: number | null; children: FolderNode[];
}

export const useShareData = routeLoader$(async ({ params, request }) => {
  const code = params["code"];
  if (!code) return null;
  const srv = createServerApi(request);
  return await srv.get<ShareAccess>(`/files/shares/access/${code}?page=1&per_page=50`).catch(() => null);
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
  const expandedFolders = useSignal<Set<number>>(new Set());

  const toggleFile = $((id: number) => {
    const s = new Set(selFileIds.value);
    if (s.has(id)) s.delete(id); else s.add(id);
    selFileIds.value = s;
  });

  const toggleAll = $((ids: number[]) => {
    if (selFileIds.value.size === ids.length) { selFileIds.value = new Set(); }
    else { selFileIds.value = new Set(ids); }
  });

  // ── Folder content cache ──
  // Key: folderId, Value: { folders, items, total, page, perPage, loading }
  const folderContents = useSignal<Map<number, {
    folders: FolderEntry[]; items: any[]; total: number; page: number; perPage: number; loading: boolean; loadedRootItems?: any[]; rootTotal?: number; rootPage?: number;
  }>>(new Map());

  const PER_PAGE = 50;
  const FOLDER_PAGE = 50;

  const loadFolder = $(async (folderId: number, pg: number, pPg: number) => {
    const key = folderId;
    const cached = folderContents.value.get(key) || { folders: [], items: [], total: 0, page: 1, perPage: FOLDER_PAGE, loading: false };
    if (cached.loading) return;
    cached.loading = true;
    folderContents.value = new Map(folderContents.value).set(key, { ...cached });
    try {
      const res = await fetch(`/api/v2/share/${shareCode}/folder/${folderId}?page=${pg || cached.page}&per_page=${pPg || cached.perPage}`);
      const json = await res.json();
      const d = json.data || json;
      const merged = {
        folders: d.folders || [],
        items: [...(cached.items || []), ...(d.items || [])],
        total: d.total || 0,
        page: pg || d.page || 1,
        perPage: pPg || d.per_page || FOLDER_PAGE,
        loading: false,
      };
      folderContents.value = new Map(folderContents.value).set(key, merged);
    } catch {
      cached.loading = false;
      folderContents.value = new Map(folderContents.value).set(key, { ...cached });
    }
  });

  const loadMore = $((folderId: number) => {
    const cached = folderContents.value.get(folderId);
    if (!cached) return;
    loadFolder(folderId, cached.page + 1, cached.perPage);
  });

  const displayLimit = useSignal<Map<number, number>>(new Map());

  const showMore = $((folderId: number, total: number) => {
    displayLimit.value = new Map(displayLimit.value).set(folderId, Math.min(total, (displayLimit.value.get(folderId) || 3) + 50));
  });

  const toggleExpand = $((folderId: number) => {
    const s = new Set(expandedFolders.value);
    if (s.has(folderId)) {
      s.delete(folderId);
      expandedFolders.value = s;
      return;
    }
    s.add(folderId);
    expandedFolders.value = s;
    // Lazy load if not already cached
    if (!folderContents.value.has(folderId)) {
      loadFolder(folderId, 1, FOLDER_PAGE);
    }
  });

  // Auto-expand first 3 folders on load (using preloaded data from API)
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ track }) => {
    track(() => shareData.value);
    const s = shareData.value;
    if (!s || s.need_password || !s.folders) return;
    const preloaded: any[] = (s as any).preloaded_folders || [];
    const folders = s.folders || [];
    for (let i = 0; i < Math.min(3, folders.length); i++) {
      const fid = folders[i].id;
      expandedFolders.value = new Set(expandedFolders.value).add(fid);
      // Use preloaded data if available, show first 3 initially
      const pl = preloaded.find((p: any) => p.id === fid);
      if (pl) {
        folderContents.value = new Map(folderContents.value).set(fid, {
          folders: pl.folders || [],
          items: pl.items || [],
          total: pl.file_count || 0,
          page: 1,
          perPage: FOLDER_PAGE,
          loading: false,
        });
        displayLimit.value = new Map(displayLimit.value).set(fid, 3);
      } else {
        loadFolder(fid, 1, FOLDER_PAGE);
        displayLimit.value = new Map(displayLimit.value).set(fid, 3);
      }
    }
  });

  // Auto-select files
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

  // ── Save-to-drive ──
  const showFolderPicker = useSignal(false);
  const folderTree = useSignal<FolderNode[]>([]);
  const targetFolderId = useSignal<number | undefined>(undefined);
  const targetFolderName = useSignal("根目录");
  const saving = useSignal(false);
  const saveDone = useSignal(false);
  const rootPage = useSignal(1);

  const loadFolderTreeFn = $(async () => {
    try {
      const res = await fetch("/api/v2/files/folders/tree");
      const json = await res.json();
      const d = json.data || json;
      folderTree.value = Array.isArray(d) ? d : [];
    } catch { folderTree.value = []; }
  });

  const renderFolderNode = (nodes: FolderNode[], d = 0): any =>
    nodes.map(node => (
      <div key={node.id}>
        <button onClick$={() => { targetFolderId.value = node.id; targetFolderName.value = node.name; showFolderPicker.value = false; }}
          class={`w-full text-left px-3 py-2 text-sm flex items-center gap-2 hover:bg-stora-muted ${targetFolderId.value === node.id ? "bg-stora-primary/10 text-stora-primary" : "text-stora-foreground"}`}
          style={{ paddingLeft: `${12 + d * 20}px` }}>
          <span>📁</span><span class="truncate">{node.name}</span>
        </button>
        {node.children?.length > 0 && renderFolderNode(node.children, d + 1)}
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
      await loadFolderTreeFn();
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

  // ── Collect all selected file IDs from expanded folders ──
  const collectSelected = $((): number[] => {
    return [...selFileIds.value];
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

  if (s.need_password) {
    return (
      <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
        <div class="w-full max-w-sm bg-stora-card border border-stora-border p-8 text-center">
          <div class="text-5xl mb-4">🔒</div>
          <h1 class="text-xl font-semibold text-stora-foreground mb-2">此文件受密码保护</h1>
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

  // Data for root level
  const rootFolders: FolderEntry[] = (isFolder || isBatch) ? (s.folders || []) : [];
  const rootItems: any[] = (isFolder || isBatch) ? (s.items || []) : (s.items && s.items.length > 0 ? s.items : (s.item?.id ? [s.item] : []));
  const rootTotal = s.total || rootItems.length;
  const hasMoreRoot = rootItems.length < rootTotal;
  const isMixed = isFolder || isBatch;

  // Header
  const headerIcon = isBatch ? "📦" : isFolder ? "📁" : "📄";
  const headerName = isBatch ? "分享的文件" : isFolder ? (item.filename || "共享文件夹") : (item.filename || "未命名文件");

  // Determine which files are available for selection (root files + expanded folder files)
  const allSelectable = new Set<number>();
  for (const f of rootItems) { if (!f.is_folder) allSelectable.add(f.id); }
  for (const [, content] of folderContents.value) {
    for (const f of content.items) { allSelectable.add(f.id); }
  }

  return (
    <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
      <div class="w-full max-w-[560px] bg-stora-card border border-stora-border p-8">
        {/* Header */}
        <div class="flex items-center gap-3 mb-4">
          <div class="w-12 h-12 bg-stora-muted flex items-center justify-center text-2xl">{headerIcon}</div>
          <div class="flex-1 min-w-0">
            <h1 class="text-lg font-semibold text-stora-foreground truncate">{headerName}</h1>
            <p class="text-xs text-stora-muted-foreground">{rootTotal + [...folderContents.value.values()].reduce((a, c) => a + c.total, 0)} 个文件</p>
          </div>
        </div>

        {/* Folder list (root level) */}
        {rootFolders.length > 0 && (
          <div class="mb-3">
            <p class="text-xs font-medium text-stora-muted-foreground mb-1">文件夹</p>
            <div class="divide-y divide-stora-border border border-stora-border">
              {rootFolders.map(f => {
                const isExp = expandedFolders.value.has(f.id);
                const content = folderContents.value.get(f.id);
                return (
                  <div key={f.id}>
                    {/* Folder row */}
                    <button onClick$={() => toggleExpand(f.id)}
                      class="w-full flex items-center gap-2 px-3 py-2 text-sm text-stora-foreground hover:bg-stora-muted text-left">
                      <span>{isExp ? "📂" : "📁"}</span>
                      <span class="truncate flex-1">{f.name}</span>
                      <span class="text-xs text-stora-muted-foreground">{f.file_count} 文件{f.child_folder_count > 0 ? ` · ${f.child_folder_count} 子文件夹` : ""}</span>
                    </button>
                    {/* Expanded content */}
                    {isExp && (
                      <div class="border-t border-stora-border bg-stora-muted/20">
                        {content?.loading && (
                          <div class="px-6 py-3 text-xs text-stora-muted-foreground">加载中...</div>
                        )}
                        {content && !content.loading && (
                          <>
                            {/* Sub-folders */}
                            {content.folders.map((sf: FolderEntry) => (
                              <div key={sf.id} class="flex items-center gap-2 px-6 py-2 text-sm text-stora-foreground">
                                <span>📁</span>
                                <span class="truncate flex-1">{sf.name}</span>
                                <span class="text-xs text-stora-muted-foreground">{sf.file_count} 项</span>
                              </div>
                            ))}
                            {/* Files (limited to display limit, then show more) */}
                            {(content.items.slice(0, displayLimit.value.get(f.id) || content.items.length) as any[]).map((fi: any) => (
                              <div key={fi.id} onClick$={() => toggleFile(fi.id)}
                                class={`flex items-center gap-2 px-6 py-2 text-sm cursor-pointer hover:bg-stora-muted ${selFileIds.value.has(fi.id) ? "bg-stora-primary/5" : ""}`}>
                                <input type="checkbox" checked={selFileIds.value.has(fi.id)}
                                  onClick$={(e: any) => e.stopPropagation()}
                                  onChange$={() => toggleFile(fi.id)} class="border-stora-border shrink-0" />
                                <span>{typeIcon[fi.file_type] || "📄"}</span>
                                <span class="flex-1 truncate">{fi.filename}</span>
                                <span class="text-xs text-stora-nav-text shrink-0">{fmtSize(fi.file_size)}</span>
                              </div>
                            ))}
                            {/* Show more / load more */}
                            {(displayLimit.value.get(f.id) || 3) < content.total && (
                              <button onClick$={() => showMore(f.id, content.total)}
                                class="w-full text-left px-6 py-2 text-xs text-stora-primary hover:bg-stora-muted/40">
                                显示全部 {content.total} 项（已显示 {(displayLimit.value.get(f.id) || 3)} 项）→
                              </button>
                            )}
                            {content.items.length < content.total && (displayLimit.value.get(f.id) || 3) >= content.items.length && (
                              <button onClick$={() => loadMore(f.id)}
                                class="w-full text-left px-6 py-2 text-xs text-stora-primary hover:bg-stora-muted/40">
                                加载更多（已显示 {content.items.length}，共 {content.total}）→
                              </button>
                            )}
                          </>
                        )}
                        {!content && (
                          <div class="px-6 py-3 text-xs text-stora-muted-foreground">点击加载</div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Root-level file list */}
        {rootItems.length > 0 ? (
          <div>
            {rootItems.length > 1 && (
              <div class="flex items-center gap-2 px-1 py-1.5 border-b border-stora-border mb-1">
                <input type="checkbox"
                  checked={[...allSelectable].every(id => selFileIds.value.has(id)) && [...allSelectable].length > 0}
                  onChange$={() => {
                    if ([...allSelectable].every(id => selFileIds.value.has(id))) {
                      selFileIds.value = new Set();
                    } else {
                      selFileIds.value = new Set(allSelectable);
                    }
                  }}
                  class="border-stora-border" />
                <span class="text-xs text-stora-muted-foreground">
                  {selFileIds.value.size === 0 ? "全选" : `已选 ${selFileIds.value.size} 项`}
                </span>
              </div>
            )}
            <div class="divide-y divide-stora-border border border-stora-border mb-4 max-h-[500px] overflow-y-auto">
              {rootItems.map((f: any) => (
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
            {hasMoreRoot && (
              <button onClick$={async () => {
                const nextPage = (s.page || 1) + 1;
                try {
                  const res = await fetch(`/api/v2/files/shares/access/${shareCode}?page=${nextPage}&per_page=${PER_PAGE}`);
                  const json = await res.json();
                  const d = json.data || json;
                  if (d.items) {
                    shareData.value = {
                      ...shareData.value!,
                      items: [...(s.items || []), ...d.items],
                      total: d.total || 0,
                      page: nextPage,
                    };
                  }
                } catch {}
              }} class="w-full py-3 text-xs font-medium text-stora-primary hover:bg-stora-muted text-center border border-stora-border">
                加载更多文件（已显示 {rootItems.length}，共 {rootTotal}）
              </button>
            )}
          </div>
        ) : rootFolders.length === 0 ? (
          <div class="py-8 text-center text-sm text-stora-muted-foreground">暂无文件</div>
        ) : null}

        {/* Actions */}
        {saveDone.value ? (
          <p class="text-sm text-green-600 text-center py-3">✅ 已转存到 {targetFolderName.value}</p>
        ) : (
          <div class="flex flex-col gap-3 mt-4">
            <button onClick$={async () => {
              const ids = [...selFileIds.value];
              if (ids.length === 0) { error.value = "请先选择文件"; return; }
              await doSave(ids);
            }} disabled={selFileIds.value.size === 0 || saving.value}
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
