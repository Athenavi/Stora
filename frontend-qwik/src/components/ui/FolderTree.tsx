/**
 * Stora FolderTree — collapsible folder navigation sidebar component
 * Fetches folder tree from API and renders recursively.
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { useNavigate, useLocation } from "@builder.io/qwik-city";
import { api, type Folder } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";

interface FolderNode extends Folder {
  children: FolderNode[];
}

export default component$(() => {
  const nav = useNavigate();
  const loc = useLocation();
  const tree = useSignal<FolderNode[]>([]);
  const loading = useSignal(true);
  const expanded = useSignal<Set<number>>(new Set());
  const currentFolderId = Number(loc.url.searchParams.get("folder")) || 0;

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    try {
      const data = await api.get<FolderNode[]>("/files/folders/tree");
      tree.value = data || [];
    } catch {}
    loading.value = false;
  });

  const toggle = (id: number) => {
    const s = new Set(expanded.value);
    if (s.has(id)) s.delete(id); else s.add(id);
    expanded.value = s;
  };

  const renderTree = (nodes: FolderNode[], depth = 0) => nodes.map(node => (
    <div key={node.id}>
      <div
        onClick$={() => nav(`/drive?folder=${node.id}`)}
        onContextMenu$={(e: any) => e.preventDefault()}
        preventdefault:drop
        preventdefault:dragover
        onDragOver$={(e: any) => { e.currentTarget.classList.add('bg-indigo-600/30'); }}
        onDragLeave$={(e: any) => { e.currentTarget.classList.remove('bg-indigo-600/30'); }}
        onDrop$={async (e: DragEvent) => {
          e.currentTarget.classList.remove('bg-indigo-600/30');
          const data = e.dataTransfer?.getData('text/plain');
          if (!data) return;
          try {
            const { fileIds } = JSON.parse(data);
            if (fileIds?.length) {
              await api.post('/files/batch/move', { file_ids: fileIds, target_folder_id: node.id });
              // Refresh the current page
              if (currentFolderId) nav(`/drive?folder=${currentFolderId}`);
              else nav('/drive');
            }
          } catch {}
        }}
        class={`flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm cursor-pointer transition-colors group ${
          currentFolderId === node.id
            ? "bg-indigo-600/20 text-indigo-300"
            : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
        }`}
        style={{ paddingLeft: `${12 + depth * 16}px` }}
      >
        <button onClick$={(e: any) => { e.stopPropagation(); toggle(node.id); }}
          class="w-4 h-4 flex items-center justify-center shrink-0 text-slate-500 hover:text-slate-300">
          {node.children?.length > 0 ? (
            <Icon name={expanded.value.has(node.id) ? "chevronDown" : "chevronRight"} size={12} />
          ) : <span class="w-4" />}
        </button>
        <Icon name="folder" size={16} class="shrink-0 text-amber-400" />
        <span class="truncate flex-1">{node.name}</span>
      </div>
      {node.children?.length > 0 && expanded.value.has(node.id) && (
        <div>{renderTree(node.children, depth + 1)}</div>
      )}
    </div>
  ));

  if (loading.value) {
    return <div class="px-3 py-2 space-y-2">{Array.from({length:4}).map((_,i) => (
      <div key={i} class="h-6 bg-slate-800 rounded animate-pulse" style={{width:`${60+Math.random()*30}%`}} />
    ))}</div>;
  }

  return (
    <div class="px-2 py-2">
      <div class="flex items-center justify-between px-2 mb-2">
        <span class="text-xs font-medium text-slate-500 uppercase tracking-wider">文件夹</span>
      </div>
      <div class="space-y-0.5 max-h-[40vh] overflow-y-auto scrollbar-thin">
        {renderTree(tree.value)}
      </div>
    </div>
  );
});
