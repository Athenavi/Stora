/**
 * Stora Drive — core file manager with rename, folder create, batch ops, drag-drop
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { listFiles, getFolderChildren, createFolder, updateFile, deleteFile, moveFiles, uploadFile, type FileItem, type Folder } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button, Skeleton, Input } from "~/components/ui/Button";

export const useFileList = routeLoader$(async ({ url }) => {
  const folderId = url.searchParams.get("folder");
  if (folderId) {
    const d = await getFolderChildren(Number(folderId)).catch(() => null);
    return d || { folders: [], files: [], path: [{ id: 0, name: "我的文件" }] };
  }
  const files = await listFiles({ page: 1, page_size: 50 }).catch(() => null);
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
    for (const f of data.value.folders) allItems.push({ t: "f" as const, ...f });
    for (const f of data.value.files) allItems.push({ t: "d" as const, ...f });
  }

  const viewMode = useSignal<"list" | "grid">("list");
  const selIds = useSignal<number[]>([]);
  const showUpload = useSignal(false);
  const showNewFolder = useSignal(false);
  const newFolderName = useSignal("");
  const renameId = useSignal(0);
  const renameVal = useSignal("");

  const refresh = () => { location.href = `/drive${folderId ? `?folder=${folderId}` : ""}`; };

  return (
    <div class="flex flex-col h-full">
      {/* Toolbar */}
      <div class="flex items-center gap-3 px-6 py-3 border-b border-slate-200 bg-white shrink-0 flex-wrap">
        <div class="flex items-center gap-2 flex-1">
          <div class="relative max-w-sm">
            <Icon name="search" size={16} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <input type="text" placeholder="搜索文件名..."
              onInput$={(e: any) => { if (e.target.value) nav(`/drive?search=${e.target.value}`); }}
              class="w-64 pl-9 pr-3 py-2 text-sm rounded-lg border border-slate-200 bg-slate-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent focus:bg-white placeholder:text-slate-400 transition-all" />
          </div>
        </div>

        <div class="flex items-center gap-1 bg-slate-100 rounded-lg p-0.5">
          <button onClick$={() => viewMode.value = "list"}
            class={`p-1.5 rounded-md ${viewMode.value === "list" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400"}`}><Icon name="list" size={18} /></button>
          <button onClick$={() => viewMode.value = "grid"}
            class={`p-1.5 rounded-md ${viewMode.value === "grid" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400"}`}><Icon name="grid" size={18} /></button>
        </div>

        <Button variant="primary" size="sm" onClick$={() => showUpload.value = !showUpload.value}>
          <Icon name="upload" size={16} /> 上传
        </Button>
        <Button variant="secondary" size="sm" onClick$={() => { showNewFolder.value = true; newFolderName.value = ""; }}>
          <Icon name="plus" size={16} /> 新建文件夹
        </Button>

        {/* Batch operations */}
        {selIds.value.length > 0 && (
          <div class="flex items-center gap-2 ml-2 pl-3 border-l border-slate-200">
            <span class="text-xs text-slate-500">{selIds.value.length} 项</span>
            <Button variant="ghost" size="sm" onClick$={async () => {
              const ids = [...selIds.value]; selIds.value = [];
              try { await moveFiles(ids, folderId); refresh(); } catch {}
            }}>移动</Button>
            <Button variant="ghost" size="sm" class="!text-red-500" onClick$={async () => {
              if (!confirm("确认删除？")) return;
              for (const id of selIds.value) await deleteFile(id).catch(() => {});
              selIds.value = []; refresh();
            }}>删除</Button>
          </div>
        )}
      </div>

      {/* Upload zone (P0.5: integrated drag-drop) */}
      {showUpload.value && (
        <div class="px-6 py-3 border-b bg-white"
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
      <div class="flex items-center gap-1.5 px-6 py-2.5 text-sm border-b border-slate-100 bg-white/80 shrink-0">
        <a href="/drive" class="text-slate-500 hover:text-indigo-600 font-medium">我的文件</a>
        {data.value?.path?.slice(1).map(p => (
          <><Icon name="chevronRight" size={14} class="text-slate-300" /><a key={p.id} href={`/drive?folder=${p.id}`} class="text-slate-500 hover:text-indigo-600">{p.name}</a></>
        ))}
        {allItems.length > 0 && <><Icon name="chevronRight" size={14} class="text-slate-300" /><span class="text-slate-400">{allItems.length} 项</span></>}
      </div>

      {/* New Folder Dialog (P0.3) */}
      {showNewFolder.value && (
        <div class="px-6 py-3 border-b bg-white flex items-center gap-3">
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

      {/* Rename Dialog (P0.2) */}
      {renameId.value > 0 && (
        <div class="px-6 py-3 border-b bg-white flex items-center gap-3">
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

      {/* Content */}
      <div class="flex-1 overflow-auto scrollbar-thin">
        {!data.value ? (
          <div class="p-6 space-y-3">{[1,2,3,4,5].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><Skeleton class="w-5 h-5 rounded" /><Skeleton class="w-10 h-10 rounded-lg" /><div class="flex-1 space-y-2"><Skeleton class="h-4 w-48" /><Skeleton class="h-3 w-24" /></div></div>)}</div>
        ) : allItems.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">📂</div>
            <h3 class="text-lg font-semibold text-slate-500 mb-1">空目录</h3>
            <p class="text-sm text-slate-400 mb-6">拖拽文件到此处，或点击上传按钮</p>
            <Button variant="primary" onClick$={() => showUpload.value = true}><Icon name="upload" size={16} /> 上传文件</Button>
          </div>
        ) : viewMode.value === "list" ? <ListView items={allItems} selIds={selIds} renameId={renameId} renameVal={renameVal} nav={nav} folderId={folderId} /> : <GridView items={allItems} selIds={selIds} nav={nav} folderId={folderId} />}
      </div>
    </div>
  );
});

// ─── List View ───

export const ListView = component$<{ items: any[]; selIds: any; renameId: any; renameVal: any; nav: any; folderId?: number }>(({ items, selIds, renameId, renameVal, nav, folderId }) => (
  <table class="w-full">
    <thead>
      <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 sticky top-0 bg-slate-50/95 backdrop-blur">
        <th class="w-10 px-4 py-3"><input type="checkbox" checked={selIds.value.length === items.length && items.length > 0}
          onChange$={() => selIds.value = selIds.value.length === items.length ? [] : items.map((x: any) => x.id)} class="rounded border-slate-300" /></th>
        <th class="px-2 py-3">文件名</th>
        <th class="px-2 py-3 w-28">大小</th>
        <th class="px-2 py-3 w-24">类型</th>
        <th class="px-2 py-3 w-32">操作</th>
      </tr>
    </thead>
    <tbody class="divide-y divide-slate-50">
      {items.map((item: any) => {
        if (item.t === "f") {
          const sel = selIds.value.includes(item.id);
          return (
            <tr key={`f-${item.id}`} class={`group text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}>
              <td class="px-4 py-3" onClick$={(e: any) => e.stopPropagation()}>
                <input type="checkbox" checked={sel}
                  onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }} class="rounded border-slate-300" />
              </td>
              <td class="px-2 py-3 cursor-pointer" onClick$={() => nav(`/drive?folder=${item.id}`)}>
                <div class="flex items-center gap-3"><div class="w-9 h-9 rounded-lg bg-amber-50 text-amber-600 flex items-center justify-center text-sm">📁</div><span class="text-slate-700 font-medium">{item.name}</span></div>
              </td>
              <td class="px-2 py-3 text-slate-500">—</td>
              <td class="px-2 py-3"><span class="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-600">folder</span></td>
              <td class="px-2 py-3"><div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity"><Button variant="ghost" size="sm">重命名</Button><Button variant="ghost" size="sm" class="!text-red-500">删除</Button></div></td>
            </tr>
          );
        }
        const sel = selIds.value.includes(item.id);
        const tc = typeMeta[item.file_type] || typeMeta.other;
        return (
          <tr key={item.id} class={`group text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}>
            <td class="px-4 py-3"><input type="checkbox" checked={sel}
              onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }} class="rounded border-slate-300" /></td>
            <td class="px-2 py-3">
              <div class="flex items-center gap-3">
                <div class={`w-9 h-9 rounded-lg flex items-center justify-center text-sm shrink-0 ${tc.color}`}>{tc.icon}</div>
                <span class="text-slate-700 truncate max-w-xs font-medium">{item.filename}</span>
              </div>
            </td>
            <td class="px-2 py-3 text-slate-500">{fmtSize(item.file_size)}</td>
            <td class="px-2 py-3"><span class="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-600">{item.file_type}</span></td>
            <td class="px-2 py-3">
              <div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <Button variant="ghost" size="sm" onClick$={() => { renameId.value = item.id; renameVal.value = item.filename; }}>重命名</Button>
                <Button variant="ghost" size="sm" class="!text-red-500" onClick$={async () => { if (confirm("删除?")) { await deleteFile(item.id).catch(() => {}); location.reload(); } }}>删除</Button>
              </div>
            </td>
          </tr>
        );
      })}
    </tbody>
  </table>
));

// ─── Grid View ───

export const GridView = component$<{ items: any[]; selIds: any; nav: any; folderId?: number }>(({ items, selIds, nav }) => (
  <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6">
    {items.map((item: any) => {
      if (item.t === "f") return (
        <div key={`f-${item.id}`} onClick$={() => nav(`/drive?folder=${item.id}`)}
          class="bg-white rounded-xl border-2 border-slate-100 hover:border-amber-300 hover:shadow-sm transition-all cursor-pointer p-3">
          <div class="aspect-square rounded-lg bg-amber-50 flex items-center justify-center text-5xl mb-2.5">📁</div>
          <p class="text-xs font-medium text-slate-700 truncate text-center">{item.name}</p>
        </div>
      );
      const sel = selIds.value.includes(item.id);
      const tc = typeMeta[item.file_type] || typeMeta.other;
      return (
        <div key={item.id} onClick$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }}
          class={`bg-white rounded-xl border-2 transition-all cursor-pointer p-3 ${sel ? "border-indigo-500 shadow-md" : "border-slate-100 hover:border-slate-200 hover:shadow-sm"}`}>
          <div class={`aspect-square rounded-lg flex items-center justify-center text-4xl mb-2.5 ${tc.color}`}><span>{tc.icon}</span></div>
          <p class="text-xs font-medium text-slate-700 truncate text-center">{item.filename}</p>
          <p class="text-xs text-slate-400 text-center mt-0.5">{fmtSize(item.file_size)}</p>
          {sel && <div class="absolute top-2 right-2 w-5 h-5 bg-indigo-600 rounded-full flex items-center justify-center"><Icon name="check" size={12} class="text-white" /></div>}
        </div>
      );
    })}
  </div>
));
