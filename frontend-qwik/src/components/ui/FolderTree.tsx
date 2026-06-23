/**
 * Stora FolderTree — collapsible folder navigation sidebar with context menu
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { useNavigate, useLocation } from "@builder.io/qwik-city";
import { api, updateFolder, deleteFolder } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";

interface FolderNode {
  id: number;
  name: string;
  parent_id?: number | null;
  children: FolderNode[];
}

function buildPathMap(nodes: FolderNode[], parentPath = ""): Map<number, string> {
  const map = new Map<number, string>();
  for (const node of nodes) {
    const fullPath = parentPath ? `${parentPath}/${node.name}` : node.name;
    map.set(node.id, fullPath);
    if (node.children?.length) {
      const childMap = buildPathMap(node.children, fullPath);
      childMap.forEach((v, k) => map.set(k, v));
    }
  }
  return map;
}

export default component$(() => {
  const nav = useNavigate();
  const loc = useLocation();
  const tree = useSignal<FolderNode[]>([]);
  const loading = useSignal(true);
  const expanded = useSignal<Set<number>>(new Set());
  const currentPath = loc.url.searchParams.get("Path") || "";
  const pathMap = useSignal<Map<number, string>>(new Map());

  // Context menu state
  const ctxFolder = useSignal<FolderNode | null>(null);
  const ctxPos = useSignal({ x: 0, y: 0 });
  const renamingId = useSignal(0);
  const renameVal = useSignal("");

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    try {
      const data = await api.get<FolderNode[]>("/files/folders/tree");
      tree.value = data || [];
      pathMap.value = buildPathMap(tree.value);
    } catch {}
    loading.value = false;
  });

  const refreshTree = $(async () => {
    try {
      const data = await api.get<FolderNode[]>("/files/folders/tree");
      tree.value = data || [];
      pathMap.value = buildPathMap(tree.value);
    } catch {}
  });

  const toggle = (id: number) => {
    const s = new Set(expanded.value);
    if (s.has(id)) s.delete(id); else s.add(id);
    expanded.value = s;
  };

  const renderTree = (nodes: FolderNode[], depth = 0) => nodes.map(node => {
    const nodePath = pathMap.value.get(node.id) || node.name;
    const isActive = currentPath === nodePath;

    return (
      <div key={node.id}>
        <div
          onClick$={() => nav(`/drive?Path=${encodeURIComponent(nodePath)}`)}
          onContextMenu$={(e: any) => {
            ctxFolder.value = node;
            ctxPos.value = { x: e.clientX, y: e.clientY };
          }}
          preventdefault:contextmenu
          preventdefault:drop
          preventdefault:dragover
          onDragOver$={(e: any) => { (e.currentTarget as HTMLElement).style.backgroundColor = 'rgba(37,99,235,0.3)'; }}
          onDragLeave$={(e: any) => { (e.currentTarget as HTMLElement).style.backgroundColor = ''; }}
          onDrop$={async (e: DragEvent) => {
            (e.currentTarget as HTMLElement).style.backgroundColor = '';
            const data = e.dataTransfer?.getData('text/plain');
            if (!data) return;
            try {
              const { fileIds } = JSON.parse(data);
              if (fileIds?.length) {
                await api.post('/files/batch/move', { file_ids: fileIds, target_folder_id: node.id });
                nav(currentPath ? `/drive?Path=${encodeURIComponent(currentPath)}` : '/drive');
              }
            } catch {}
          }}
          class={`flex items-center gap-2 px-2 py-1.5 text-sm cursor-pointer group ${
            isActive
              ? "bg-stora-primary text-white"
              : "text-stora-nav-text hover:text-slate-200 hover:bg-stora-nav-hover"
          }`}
          style={{ paddingLeft: `${12 + depth * 16}px` }}
        >
          <button onClick$={(e: any) => { e.stopPropagation(); toggle(node.id); }}
            class="w-4 h-4 flex items-center justify-center shrink-0 text-stora-nav-text hover:text-slate-300">
            {node.children?.length > 0 ? (
              <Icon name={expanded.value.has(node.id) ? "chevronDown" : "chevronRight"} size={12} />
            ) : <span class="w-4" />}
          </button>
          <Icon name="folder" size={16} class="shrink-0 text-stora-accent" />
          <span class="truncate flex-1">{node.name}</span>
        </div>

        {/* Inline rename input */}
        {renamingId.value === node.id && (
          <div class="flex items-center gap-1 px-2 py-1" style={{ paddingLeft: `${28 + depth * 16}px` }}>
            <input type="text" bind:value={renameVal}
              class="flex-1 px-2 py-0.5 text-xs border border-stora-border rounded bg-white text-stora-foreground"
              onKeyDown$={async (e: any) => {
                if (e.key === "Enter" && renameVal.value.trim()) {
                  try { await updateFolder(node.id, renameVal.value.trim()); renamingId.value = 0; refreshTree(); } catch {}
                }
                if (e.key === "Escape") renamingId.value = 0;
              }}
              onBlur$={async () => {
                if (renameVal.value.trim()) {
                  try { await updateFolder(node.id, renameVal.value.trim()); } catch {}
                }
                renamingId.value = 0;
              }} />
          </div>
        )}

        {node.children?.length > 0 && expanded.value.has(node.id) && (
          <div>{renderTree(node.children, depth + 1)}</div>
        )}
      </div>
    );
  });

  // Context menu backdrop
  const closeCtx = $(() => { ctxFolder.value = null; });

  if (loading.value) {
    return <div class="px-3 py-2 space-y-2">{Array.from({length:4}).map((_,i) => (
      <div key={i} class="h-6 bg-stora-storage-bg" style={{width:`${60+Math.random()*30}%`}} />
    ))}</div>;
  }

  return (
    <div class="px-2 py-2">
      <div class="flex items-center justify-between px-2 mb-2">
        <span class="text-xs font-medium text-stora-nav-text tracking-wider">文件夹</span>
      </div>
      <div class="space-y-0.5 max-h-[40vh] overflow-y-auto scrollbar-thin">
        {renderTree(tree.value)}
      </div>

      {/* Context menu */}
      {ctxFolder.value && (
        <>
          <div class="fixed inset-0 z-50" onClick$={closeCtx}
            preventdefault:contextmenu
            onContextMenu$={closeCtx} />
          <div class="fixed z-50 min-w-[150px] bg-white border border-stora-border shadow-xl"
            style={{ left: `${ctxPos.value.x}px`, top: `${ctxPos.value.y}px` }}>
            <button onClick$={() => { const p = pathMap.value.get(ctxFolder.value!.id); if (p) nav(`/drive?Path=${encodeURIComponent(p)}`); ctxFolder.value = null; }}
              class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">📂 打开</button>
            <button onClick$={() => { renameVal.value = ctxFolder.value!.name; renamingId.value = ctxFolder.value!.id; ctxFolder.value = null; }}
              class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-foreground hover:bg-stora-muted touch-target">✏ 重命名</button>
            <div class="h-px bg-stora-border my-1" />
            <button onClick$={async () => {
              const id = ctxFolder.value!.id;
              const name = ctxFolder.value!.name;
              ctxFolder.value = null;
              if (confirm(`确认删除文件夹「${name}」及其中所有文件？`)) {
                try { await deleteFolder(id, true); refreshTree(); } catch { alert("删除失败"); }
              }
            }} class="w-full text-left px-4 py-2.5 text-sm flex items-center gap-3 text-stora-destructive hover:bg-red-50 touch-target">🗑 删除</button>
          </div>
        </>
      )}
    </div>
  );
});
