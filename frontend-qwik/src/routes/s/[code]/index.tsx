/**
 * Stora Share Receiving Page — public file access
 * Route: /s/[code]
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useLocation } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { Button, Input } from "~/components/ui/Button";
import { api } from "~/lib/api";

interface ShareAccess {
  share_info: { short_code: string; permission: string; password_protected: boolean; download_count: number; max_downloads: number };
  item: { id?: number; filename?: string; file_size?: number; file_type?: string; mime_type?: string; name?: string; thumbnail_url?: string };
  need_password?: boolean;
  protected?: boolean;
}

export const useShareData = routeLoader$(async ({ params }) => {
  const code = params["code"];
  if (!code) return null;
  return await api.get<ShareAccess>(`/files/shares/access/${code}`).catch(() => null);
});

function fmtSize(bytes: number | undefined): string {
  if (!bytes) return "";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

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
              const res = await api.get<ShareAccess>(`/files/shares/access/${data.value.share_info.short_code}?password=${encodeURIComponent(password.value)}`);
              if ((res as any).need_password) { error.value = "密码错误"; return; }
              shareData.value = res;
            } catch (e: any) { error.value = e.message || "验证失败"; }
            finally { loading.value = false; }
          }} loading={loading.value}>确认</Button>
        </div>
      </div>
    );
  }

  return (
    <div class="min-h-screen bg-gradient-to-br from-slate-50 to-slate-100 flex items-center justify-center p-4">
      <div class="w-full max-w-lg bg-white rounded-2xl shadow-xl overflow-hidden">
        {/* Preview */}
        <div class="aspect-video bg-slate-100 flex items-center justify-center text-7xl">
          {isImage && item.thumbnail_url ? (
            <img src={item.thumbnail_url} alt="" class="w-full h-full object-cover" />
          ) : item.file_type === "image" ? "🖼️" : item.file_type === "video" ? "🎬" : item.file_type === "audio" ? "🎵" : item.file_type === "document" ? "📄" : "📦"}
        </div>
        {/* Info */}
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
            <span class="text-xs text-slate-400">已下载 {s.share_info.download_count} 次</span>
          </div>
        </div>
      </div>
    </div>
  );
});
