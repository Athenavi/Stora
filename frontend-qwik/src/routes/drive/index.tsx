/**
 * Stora Drive — Enterprise File Manager
 *
 * Supports folder navigation via ?folder=id query param.
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { listFiles, getFolderChildren, type FileItem, type Folder, type FolderChildrenResponse } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button, Skeleton } from "~/components/ui/Button";
import UploadZone from "~/components/upload/UploadZone";

export const useFileList = routeLoader$(async ({ query, url }) => {
  const folderId = url.searchParams.get("folder");
  if (folderId) {
    const data = await getFolderChildren(Number(folderId)).catch(() => null);
    return data || { folders: [], files: [], path: [] };
  }
  const files = await listFiles({ page: 1, page_size: 50 }).catch(() => null);
  return files ? { folders: [], files: files.items, path: [{ id: 0, name: "我的文件" }] } : null;
});

function fmtSize(bytes: number): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

function fmtDate(s: string | undefined): string {
  if (!s) return "";
  const d = new Date(s);
  const diff = Date.now() - d.getTime();
  if (diff < 60000) return "刚刚";
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`;
  if (diff < 172800000) return "昨天";
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

const typeColors: Record<string, { icon: string; color: string }> = {
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
  const allItems: ({ type: "folder" } & Folder | { type: "file" } & FileItem)[] = [];
  if (data.value) {
    for (const f of data.value.folders) allItems.push({ type: "folder", ...f });
    for (const f of data.value.files) allItems.push({ type: "file", ...f });
  }
  const viewMode = useSignal<"list" | "grid">("list");
  const showUpload = useSignal(false);
  const selIds = useSignal<number[]>([]);

  return (
    <div class="flex flex-col h-full">
      {/* Toolbar */}
      <div class="flex items-center gap-3 px-6 py-3 border-b border-slate-200 bg-white shrink-0">
        <div class="flex items-center gap-2 flex-1">
          <div class="relative max-w-sm">
            <Icon name="search" size={16} class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <input type="text" placeholder="搜索文件名..." class="w-72 pl-9 pr-3 py-2 text-sm rounded-lg border border-slate-200 bg-slate-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent focus:bg-white placeholder:text-slate-400 transition-all" />
          </div>
        </div>
        <div class="flex items-center gap-1 bg-slate-100 rounded-lg p-0.5">
          <button onClick$={() => viewMode.value = "list"}
            class={`p-1.5 rounded-md transition-colors ${viewMode.value === "list" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400 hover:text-slate-600"}`}>
            <Icon name="list" size={18} />
          </button>
          <button onClick$={() => viewMode.value = "grid"}
            class={`p-1.5 rounded-md transition-colors ${viewMode.value === "grid" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400 hover:text-slate-600"}`}>
            <Icon name="grid" size={18} />
          </button>
        </div>
        <Button variant="primary" size="sm" onClick$={() => showUpload.value = !showUpload.value}>
          <Icon name="upload" size={16} /> 上传
        </Button>
        <Button variant="secondary" size="sm">
          <Icon name="plus" size={16} /> 新建文件夹
        </Button>
      </div>

      {showUpload.value && (
        <div class="px-6 py-4 border-b bg-white">
          <UploadZone folderId={undefined} />
        </div>
      )}

      <div class="flex items-center gap-1.5 px-6 py-2.5 text-sm border-b border-slate-100 bg-white/80 shrink-0">
        <span class="text-slate-500 font-medium">
          {data.value?.path?.length ? data.value.path[data.value.path.length-1]?.name : '我的文件'}
        </span>
        {data.value?.path?.slice(0,-1).map(p => (
          <a key={p.id} href={`/drive?folder=${p.id}`} class="text-slate-400 hover:text-indigo-600 transition-colors">{p.name}</a>
        ))}
        {allItems.length > 0 && <><Icon name="chevronRight" size={14} class="text-slate-300" /><span class="text-slate-400">{allItems.length} 个项目</span></>}
      </div>

      {/* Content */}
      <div class="flex-1 overflow-auto scrollbar-thin">
        {!data.value ? (
          <div class="p-6 space-y-3">
            {[1,2,3,4,5].map(i => (
              <div key={i} class="flex items-center gap-4 px-4 py-3">
                <Skeleton class="w-5 h-5 rounded" /><Skeleton class="w-10 h-10 rounded-lg" />
                <div class="flex-1 space-y-2"><Skeleton class="h-4 w-48" /><Skeleton class="h-3 w-24" /></div>
              </div>
            ))}
          </div>
        ) : allItems.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">📂</div>
            <h3 class="text-lg font-semibold text-slate-500 mb-1">空目录</h3>
            <p class="text-sm text-slate-400 mb-6">拖拽文件到此处，或点击上传按钮开始</p>
            <Button variant="primary" onClick$={() => showUpload.value = true}>
              <Icon name="upload" size={16} /> 上传文件
            </Button>
          </div>
        ) : viewMode.value === "list" ? (
          <table class="w-full">
            <thead>
              <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 sticky top-0 bg-slate-50/95 backdrop-blur">
                <th class="w-10 px-4 py-3">
                  <input type="checkbox" checked={selIds.value.length === allItems.length && allItems.length > 0}
                    onChange$={() => selIds.value = selIds.value.length === allItems.length ? [] : allItems.map(f => f.id)} class="rounded border-slate-300" />
                </th>
                <th class="px-2 py-3">文件名</th>
                <th class="px-2 py-3 w-28">大小</th>
                <th class="px-2 py-3 w-24">类型</th>
                <th class="px-2 py-3 w-32">修改时间</th>
                <th class="w-16 px-2 py-3" />
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-50">
              {allItems.map(item => {
                if (item.type === "folder") {
                  const sel = selIds.value.includes(item.id);
                  return (
                    <tr key={`f-${item.id}`} onClick$={() => nav(`/drive?folder=${item.id}`)}
                      class={`group cursor-pointer text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}>
                      <td class="px-4 py-3" onClick$={(e: any) => e.stopPropagation()}>
                        <input type="checkbox" checked={sel}
                          onChange$={() => { const i = selIds.value.indexOf(item.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(item.id); selIds.value = [...selIds.value]; }}
                          class="rounded border-slate-300" />
                      </td>
                      <td class="px-2 py-3" colspan="5">
                        <div class="flex items-center gap-3">
                          <div class="w-9 h-9 rounded-lg bg-amber-50 text-amber-600 flex items-center justify-center text-sm">📁</div>
                          <span class="text-slate-700 font-medium">{item.name}</span>
                        </div>
                      </td>
                    </tr>
                  );
                }
                const f = item as any;
                const tc = typeColors[f.file_type] || typeColors.other;
                const sel = selIds.value.includes(f.id);
                return (
                  <tr key={f.id} class={`group cursor-pointer text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}>
                    <td class="px-4 py-3">
                      <input type="checkbox" checked={sel}
                        onChange$={() => { const i = selIds.value.indexOf(f.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id); selIds.value = [...selIds.value]; }}
                        class="rounded border-slate-300" />
                    </td>
                    <td class="px-2 py-3">
                      <div class="flex items-center gap-3">
                        <div class={`w-9 h-9 rounded-lg flex items-center justify-center text-sm shrink-0 ${tc.color}`}>{tc.icon}</div>
                        <span class="text-slate-700 truncate max-w-xs font-medium">{f.filename}</span>
                      </div>
                    </td>
                    <td class="px-2 py-3 text-slate-500">{fmtSize(f.file_size)}</td>
                    <td class="px-2 py-3"><span class="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-600">{f.file_type}</span></td>
                    <td class="px-2 py-3 text-slate-500 text-xs">{fmtDate(f.updated_at || f.created_at)}</td>
                    <td class="px-2 py-3"><button class="opacity-0 group-hover:opacity-100 p-1 rounded text-slate-400 hover:bg-slate-100 transition-all"><Icon name="menu" size={16} /></button></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : (
          <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7 gap-4 p-6">
            {allItems.map(item => {
              if (item.type === "folder") {
                return (
                  <div key={`f-${item.id}`} onClick$={() => nav(`/drive?folder=${item.id}`)}
                    class="group relative bg-white rounded-xl border-2 border-slate-100 hover:border-amber-300 hover:shadow-sm transition-all cursor-pointer p-3">
                    <div class="aspect-square rounded-lg bg-amber-50 flex items-center justify-center text-5xl mb-2.5">📁</div>
                    <p class="text-xs font-medium text-slate-700 truncate text-center">{item.name}</p>
                    <div class="absolute inset-0 rounded-xl ring-1 ring-inset ring-black/5 pointer-events-none" />
                  </div>
                );
              }
              const f = item as any;
              const tc = typeColors[f.file_type] || typeColors.other;
              const sel = selIds.value.includes(f.id);
              return (
                <div key={f.id} onClick$={() => { const i = selIds.value.indexOf(f.id); if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id); selIds.value = [...selIds.value]; }}
                  class={`group relative bg-white rounded-xl border-2 transition-all cursor-pointer p-3 ${sel ? "border-indigo-500 shadow-md" : "border-slate-100 hover:border-slate-200 hover:shadow-sm"}`}>
                  <div class={`aspect-square rounded-lg flex items-center justify-center text-4xl mb-2.5 ${tc.color}`}><span>{tc.icon}</span></div>
                  <p class="text-xs font-medium text-slate-700 truncate text-center">{f.filename}</p>
                  <p class="text-xs text-slate-400 text-center mt-0.5">{fmtSize(f.file_size)}</p>
                  {sel && <div class="absolute top-2 right-2 w-5 h-5 bg-indigo-600 rounded-full flex items-center justify-center"><Icon name="check" size={12} class="text-white" /></div>}
                  <div class="absolute inset-0 rounded-xl ring-1 ring-inset ring-black/5 pointer-events-none" />
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
});
