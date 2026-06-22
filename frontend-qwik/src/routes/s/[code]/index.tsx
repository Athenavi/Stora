/**
 * Stora Share Receiving Page — public file/folder access
 * Route: /s/[code]
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useLocation } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { Button } from "~/components/ui/Button";
import { createServerApi } from "~/lib/api";

interface FolderItem {
  id: number;
  filename: string;
  file_size: number;
  file_type: string;
}

interface SubFolder {
  id: number;
  name: string;
}

interface ShareInfo {
  id: number;
  short_code: string;
  permission: string;
  password_protected: boolean;
  is_folder?: boolean;
}

interface ShareAccess {
  share_info: ShareInfo;
  item: { id?: number; filename?: string; file_size?: number; file_type?: string; mime_type?: string; name?: string; is_folder?: boolean };
  need_password?: boolean;
  protected?: boolean;
  folders?: SubFolder[];
  items?: FolderItem[];
}

export const useShareData = routeLoader$(async ({ params, request }) => {
  const code = params["code"];
  if (!code) return null;
  const srv = createServerApi(request);
  return await srv.get<ShareAccess>(`/files/shares/access/${code}`).catch(() => null);
});

function fmtSize(bytes: number | undefined): string {
  if (!bytes) return "";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

function fmtDate(s: string | undefined | null): string {
  if (!s) return "";
  return new Date(s).toLocaleString("zh-CN");
}

const typeIcon: Record<string, string> = {
  image: "🖼️", video: "🎬", audio: "🎵", document: "📄", archive: "📦", other: "📎",
};

export default component$(() => {
  const data = useShareData();
  const loc = useLocation();
  const shareCode = loc.params["code"] || (data.value?.share_info?.short_code) || "";
  const password = useSignal("");
  const error = useSignal("");
  const shareData = useSignal<ShareAccess | null>(data.value);
  const loading = useSignal(false);

  if (!data.value) {
    return (
      <div class="min-h-screen bg-slate-50 flex items-center justify-center">
        <div class="text-center text-slate-400">
          <div class="text-6xl mb-4">🔗</div>
          <p class="text-lg font-medium text-slate-500">分享链接不存在或已失效</p>
        </div>
      </div>
    );
  }

  const s = shareData.value!;
  const item = s.item || {};
  const isImage = item.file_type === "image";
  const isFolder = s.share_info?.is_folder || item.is_folder;

  if (s.need_password) {
    return (
      <div class="min-h-screen bg-slate-50 flex items-center justify-center p-4">
        <div class="w-full max-w-sm bg-white rounded-2xl shadow-lg p-8 text-center">
          <div class="text-5xl mb-4">🔒</div>
          <h1 class="text-xl font-semibold text-slate-900 mb-2">此文件受密码保护</h1>
          <p class="text-sm text-slate-500 mb-6">请输入密码以访问</p>
          {error.value && <p class="text-sm text-red-600 mb-4">{error.value}</p>}
          <input type="password" bind:value={password} placeholder="输入密码"
            class="w-full px-4 py-2.5 rounded-lg border border-slate-300 text-sm mb-4 focus:outline-none focus:ring-2 focus:ring-indigo-500" />
          <Button class="w-full" onClick$={async () => {
            loading.value = true; error.value = "";
            try {
              const res = await (await fetch(`/api/v2/files/shares/access/${shareCode}?password=${encodeURIComponent(password.value)}`)).json();
              const d = res.data || res;
              if (d.need_password) { error.value = "密码错误"; return; }
              shareData.value = d;
            } catch (e: any) { error.value = "验证失败"; }
            finally { loading.value = false; }
          }} loading={loading.value}>确认</Button>
        </div>
      </div>
    );
  }

  if (isFolder) {
    // Folder view
    const subFolders = s.folders || [];
    const files = s.items || [];
    return (
      <div class="min-h-screen bg-gradient-to-br from-slate-50 to-slate-100 flex items-center justify-center p-4">
        <div class="w-full max-w-lg bg-white rounded-2xl shadow-xl overflow-hidden">
          <div class="p-6">
            <div class="flex items-center gap-3 mb-4">
              <div class="w-12 h-12 rounded-xl bg-amber-50 flex items-center justify-center text-2xl">📁</div>
              <div>
                <h1 class="text-lg font-semibold text-slate-900 truncate">{item.filename || "共享文件夹"}</h1>
                <p class="text-xs text-slate-400">{subFolders.length} 个文件夹 · {files.length} 个文件</p>
              </div>
            </div>

            {subFolders.length > 0 && (
              <div class="mb-4">
                <p class="text-xs font-medium text-slate-500 mb-2">文件夹</p>
                <div class="space-y-1">
                  {subFolders.map(f => (
                    <div key={f.id} class="flex items-center gap-2 px-3 py-2 bg-slate-50 rounded-lg text-sm text-slate-700">
                      <span>📁</span>
                      <span class="truncate">{f.name}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {files.length > 0 && (
              <div>
                <p class="text-xs font-medium text-slate-500 mb-2">文件</p>
                <div class="space-y-1">
                  {files.map(f => (
                    <div key={f.id} class="flex items-center gap-2 px-3 py-2 bg-slate-50 rounded-lg text-sm text-slate-700">
                      <span>{typeIcon[f.file_type] || "📄"}</span>
                      <span class="flex-1 truncate">{f.filename}</span>
                      <span class="text-xs text-slate-400">{fmtSize(f.file_size)}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div class="mt-6">
              <Button class="w-full" variant="primary" onClick$={() => {
                window.open(`/api/v2/share/${shareCode}/download`, "_blank");
              }}>
                <Icon name="download" size={16} /> 下载全部 (ZIP)
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Single file view
  return (
    <div class="min-h-screen bg-gradient-to-br from-slate-50 to-slate-100 flex items-center justify-center p-4">
      <div class="w-full max-w-lg bg-white rounded-2xl shadow-xl overflow-hidden">
        <div class="aspect-video bg-slate-100 flex items-center justify-center text-7xl">
          {isImage && item.thumbnail_url ? (
            <img src={item.thumbnail_url} alt="" class="w-full h-full object-cover" />
          ) : typeIcon[item.file_type || ""] || "📄"}
        </div>
        <div class="p-6">
          <h1 class="text-lg font-semibold text-slate-900 truncate">{item.filename || item.name || "未命名文件"}</h1>
          <div class="flex items-center gap-4 mt-2 text-sm text-slate-500">
            <span>{fmtSize(item.file_size)}</span>
            <span>·</span>
            <span class="capitalize">{item.file_type || "unknown"}</span>
          </div>
          <div class="flex items-center gap-3 mt-6">
            {item.id && (
              <Button class="flex-1" variant="primary" onClick$={() => {
                const pw = s.share_info.password_protected ? password.value : "";
                window.open(`/api/v2/share/${shareCode}/download?password=${encodeURIComponent(pw)}`, "_blank");
              }}>
                <Icon name="download" size={16} /> 下载文件
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
});
