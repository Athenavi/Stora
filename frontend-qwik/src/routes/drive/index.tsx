import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { listFiles, type FileItem } from "~/lib/api";
import UploadZone from "~/components/upload/UploadZone";

// ─── Loaders ───

export const useFileList = routeLoader$(async ({ query }) => {
  const folderId = query.get("folder") ? Number(query.get("folder")) : undefined;
  const page = query.get("page") ? Number(query.get("page")) : 1;
  const data = await listFiles({ folder_id: folderId ?? null, page, page_size: 50 }).catch(() => null);
  return data || { items: [], total: 0, page: 1, page_size: 50 };
});

// ─── Helpers ───

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return "";
  const d = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - d.getTime();
  if (diff < 60000) return "刚刚";
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`;
  if (diff < 172800000) return "昨天";
  if (diff < 604800000) return `${Math.floor(diff / 86400000)} 天前`;
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

const FILE_ICONS: Record<string, string> = {
  image: "🖼️",
  video: "🎬",
  audio: "🎵",
  document: "📄",
  archive: "📦",
  other: "📎",
};

function fileIcon(type: string): string {
  return FILE_ICONS[type] || FILE_ICONS.other;
}

// ─── Page ───

export default component$(() => {
  const fileList = useFileList();
  const viewMode = useSignal<"grid" | "list">("list");
  const showUpload = useSignal(false);

  const files = fileList.value.items;
  const total = fileList.value.total;

  return (
    <div class="flex flex-col h-full">
      {/* Top Bar */}
      <div class="flex items-center gap-4 p-4 border-b bg-white shrink-0">
        <div class="flex-1 relative">
          <input
            type="text"
            placeholder="搜索文件..."
            class="w-full max-w-md pl-10 pr-4 py-2 rounded-lg border border-slate-200 bg-slate-50 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          />
          <span class="absolute left-3 top-2.5 text-slate-400 text-sm">🔍</span>
        </div>

        <div class="flex items-center gap-1 bg-slate-100 rounded-lg p-1">
          <button
            onClick$={() => (viewMode.value = "list")}
            class={`p-1.5 rounded ${viewMode.value === "list" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400 hover:text-slate-600"}`}
            title="列表视图"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"/></svg>
          </button>
          <button
            onClick$={() => (viewMode.value = "grid")}
            class={`p-1.5 rounded ${viewMode.value === "grid" ? "bg-white shadow-sm text-indigo-600" : "text-slate-400 hover:text-slate-600"}`}
            title="网格视图"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4h6v6H4V4zM14 4h6v6h-6V4zM4 14h6v6H4v-6zM14 14h6v6h-6v-6z"/></svg>
          </button>
        </div>

        <button
          onClick$={() => (showUpload.value = !showUpload.value)}
          class={`px-4 py-2 rounded-lg text-sm font-medium transition-colors flex items-center gap-1.5 ${
            showUpload.value
              ? "bg-indigo-100 text-indigo-700 border border-indigo-200"
              : "bg-indigo-600 text-white hover:bg-indigo-700"
          }`}
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M7 10l5 5 5-5M12 15V3"/></svg>
          {showUpload.value ? "关闭" : "上传"}
        </button>
        <button class="px-4 py-2 border border-slate-300 text-slate-700 rounded-lg text-sm font-medium hover:bg-slate-50 transition-colors flex items-center gap-1.5">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/></svg>
          新建文件夹
        </button>
      </div>

      {/* Breadcrumb */}
      <div class="flex items-center gap-2 px-4 py-2 text-sm text-slate-500 border-b bg-white/50 shrink-0">
        <span class="text-indigo-600 hover:text-indigo-800 cursor-pointer font-medium">我的文件</span>
        <svg class="w-3 h-3 text-slate-300" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
        <span class="text-slate-400 text-xs">{total} 个文件</span>
      </div>

      {showUpload.value && (
        <div class="px-4 py-3 border-b bg-white">
          <UploadZone folderId={undefined} />
        </div>
      )}

      {/* File Content */}
      <div class="flex-1 overflow-auto">
        {files.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="text-6xl mb-4">📂</div>
            <h3 class="text-lg font-medium text-slate-500 mb-1">还没有文件</h3>
            <p class="text-sm">点击"上传"按钮或拖拽文件到此处开始使用</p>
          </div>
        ) : viewMode.value === "list" ? (
          <ListView files={files} />
        ) : (
          <GridView files={files} />
        )}
      </div>
    </div>
  );
});

// ─── List View ───

export const ListView = component$<{ files: FileItem[] }>(({ files }) => (
  <table class="w-full">
    <thead>
      <tr class="text-left text-xs font-medium text-slate-500 uppercase tracking-wider border-b">
        <th class="px-4 py-3 w-10"><input type="checkbox" class="rounded" /></th>
        <th class="px-2 py-3">文件名</th>
        <th class="px-2 py-3 w-24">大小</th>
        <th class="px-2 py-3 w-24">类型</th>
        <th class="px-2 py-3 w-32">修改时间</th>
      </tr>
    </thead>
    <tbody class="divide-y divide-slate-100">
      {files.map((f) => (
        <tr key={f.id} class="hover:bg-slate-50 cursor-pointer text-sm">
          <td class="px-4 py-3"><input type="checkbox" class="rounded" /></td>
          <td class="px-2 py-3">
            <div class="flex items-center gap-2.5">
              <span class="text-lg">{fileIcon(f.file_type)}</span>
              <span class="text-slate-700 truncate max-w-xs">{f.filename}</span>
            </div>
          </td>
          <td class="px-2 py-3 text-slate-500">{formatSize(f.file_size)}</td>
          <td class="px-2 py-3 text-slate-500">{f.file_type}</td>
          <td class="px-2 py-3 text-slate-500">{formatDate(f.updated_at || f.created_at)}</td>
        </tr>
      ))}
    </tbody>
  </table>
));

// ─── Grid View ───

export const GridView = component$<{ files: FileItem[] }>(({ files }) => (
  <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-4">
    {files.map((f) => (
      <div key={f.id} class="group relative bg-white rounded-xl border border-slate-200 hover:border-indigo-300 hover:shadow-md transition-all cursor-pointer p-3">
        {/* Thumbnail */}
        <div class="aspect-square rounded-lg bg-gradient-to-br from-slate-100 to-slate-50 flex items-center justify-center text-4xl mb-2">
          {f.thumbnail_url ? (
            <img src={f.thumbnail_url} alt={f.filename} class="w-full h-full object-cover rounded-lg" />
          ) : (
            fileIcon(f.file_type)
          )}
        </div>
        {/* Info */}
        <div class="text-center">
          <p class="text-xs font-medium text-slate-700 truncate">{f.filename}</p>
          <p class="text-xs text-slate-400 mt-0.5">{formatSize(f.file_size)}</p>
        </div>
        {/* Hover actions */}
        <div class="absolute top-2 right-2 hidden group-hover:flex gap-1">
          <button class="p-1.5 bg-white/90 rounded-lg shadow-sm hover:bg-white text-slate-600" title="下载">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M7 10l5 5 5-5M12 15V3"/></svg>
          </button>
          <button class="p-1.5 bg-white/90 rounded-lg shadow-sm hover:bg-white text-slate-600" title="更多">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z"/></svg>
          </button>
        </div>
      </div>
    ))}
  </div>
));
