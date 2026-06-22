/**
 * Stora FolderTree — collapsible folder navigation sidebar with path-based navigation
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { useNavigate, useLocation } from "@builder.io/qwik-city";
import { api } from "~/lib/api";
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

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    try {
      const data = await api.get<FolderNode[]>("/files/folders/tree");
      tree.value = data || [];
      pathMap.value = buildPathMap(tree.value);
    } catch {}
    loading.value = false;
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
          onContextMenu$={(e: any) => e.preventDefault()}
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
        {node.children?.length > 0 && expanded.value.has(node.id) && (
          <div>{renderTree(node.children, depth + 1)}</div>
        )}
      </div>
    );
  });

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
    </div>
  );
});
