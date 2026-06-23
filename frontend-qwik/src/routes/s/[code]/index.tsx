/**
 * Stora Share Landing — flat centered card design
 * Route: /s/[code]
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useLocation } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { createServerApi } from "~/lib/api";

interface FolderItem { id: number; filename: string; file_size: number; file_type: string; }
interface SubFolder { id: number; name: string; }
interface ShareInfo { id: number; short_code: string; permission: string; password_protected: boolean; is_folder?: boolean; is_batch?: boolean; file_count?: number; }
interface ShareAccess {
  share_info: ShareInfo;
  item?: { id?: number; filename?: string; file_size?: number; file_type?: string; mime_type?: string; name?: string; is_folder?: boolean };
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
  const k = 1024, i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

const typeIcon: Record<string, string> = { image: "🖼️", video: "🎬", audio: "🎵", document: "📄", archive: "📦", other: "📎" };
const typeEmoji: Record<string, string> = { image: "🖼", video: "🎬", audio: "🎵", document: "📄", archive: "📦", other: "📎" };
const typeMeta: Record<string, string> = { image: "🖼", video: "🎬", audio: "🎵", document: "📄", archive: "📦", pptx: "📊", pdf: "📄" };

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
  const isImage = item.file_type === "image";
  const isFolder = s.share_info?.is_folder || item.is_folder;
  const isBatch = s.share_info?.is_batch;

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

  if (isFolder) {
    const subFolders = s.folders || [];
    const files = s.items || [];
    return (
      <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
        {/* Card per spec: 560px wide, but for folder a bit different */}
        <div class="w-full max-w-[560px] bg-stora-card border border-stora-border p-8">
          <div class="flex items-center gap-3 mb-4">
            <div class="w-12 h-12 bg-stora-muted flex items-center justify-center text-2xl">📁</div>
            <div>
              <h1 class="text-lg font-semibold text-stora-foreground truncate">{item.filename || "共享文件夹"}</h1>
              <p class="text-xs text-stora-muted-foreground">{subFolders.length} 个文件夹 · {files.length} 个文件</p>
            </div>
          </div>
          {subFolders.length > 0 && (
            <div class="mb-4">
              <p class="text-xs font-medium text-stora-muted-foreground mb-2">文件夹</p>
              <div class="divide-y divide-stora-border border border-stora-border">
                {subFolders.map(f => (
                  <div key={f.id} class="flex items-center gap-2 px-3 py-2 text-sm text-stora-foreground">
                    <span>📁</span>
                    <span class="truncate">{f.name}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
          {files.length > 0 && (
            <div>
              <p class="text-xs font-medium text-stora-muted-foreground mb-2">文件</p>
              <div class="divide-y divide-stora-border border border-stora-border">
                {files.map(f => (
                  <div key={f.id} class="flex items-center gap-2 px-3 py-2 text-sm text-stora-foreground">
                    <span>{typeIcon[f.file_type] || "📄"}</span>
                    <span class="flex-1 truncate">{f.filename}</span>
                    <span class="text-xs text-stora-nav-text">{fmtSize(f.file_size)}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
          <button onClick$={() => { window.open(`/api/v2/share/${shareCode}/download`, "_blank"); }}
            class="w-full h-12 mt-6 text-sm font-semibold text-white bg-stora-primary hover:bg-[#1D4ED8]">
            ⬇ 下载全部 (ZIP)
          </button>
        </div>
      </div>
    );
  }

  // Batch share — multiple files
  if (isBatch) {
    const files = s.items || [];
    return (
      <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
        <div class="w-full max-w-[560px] bg-stora-card border border-stora-border p-8">
          <div class="flex items-center gap-3 mb-4">
            <div class="w-12 h-12 bg-stora-muted flex items-center justify-center text-2xl">📦</div>
            <div>
              <h1 class="text-lg font-semibold text-stora-foreground">分享的文件</h1>
              <p class="text-xs text-stora-muted-foreground">{files.length} 个文件</p>
            </div>
          </div>
          {files.length > 0 ? (
            <div class="divide-y divide-stora-border border border-stora-border mb-4">
              {files.map(f => (
                <div key={f.id} class="flex items-center gap-2 px-3 py-2 text-sm text-stora-foreground">
                  <span>{typeIcon[f.file_type] || "📄"}</span>
                  <span class="flex-1 truncate">{f.filename}</span>
                  <span class="text-xs text-stora-nav-text">{fmtSize(f.file_size)}</span>
                </div>
              ))}
            </div>
          ) : (
            <p class="text-sm text-stora-muted-foreground py-4 text-center">暂无文件</p>
          )}
          <button onClick$={() => { window.open(`/api/v2/share/${shareCode}/download`, "_blank"); }}
            class="w-full h-12 text-sm font-semibold text-white bg-stora-primary hover:bg-[#1D4ED8]">
            ⬇ 下载全部 (ZIP)
          </button>
        </div>
      </div>
    );
  }

  // Single file view — per spec centered card
  const emoji = typeMeta[item.file_type || ""] || "📄";
  const fileExt = (item.filename || "").split(".").pop()?.toUpperCase() || (item.file_type || "").toUpperCase() || "文件";
  return (
    <div class="min-h-screen bg-stora-background flex items-center justify-center p-4">
      {/* Share card — 560px wide per spec */}
      <div class="w-full max-w-[560px] bg-stora-card border border-stora-border p-12 flex flex-col items-center gap-7">
        {/* Share badge — capsule per spec */}
        <div class="inline-flex items-center h-[30px] px-3.5 bg-stora-tag-work text-stora-primary text-xs font-medium">
          📎 分享文件
        </div>

        {/* File icon 96x96 per spec */}
        <div class="w-24 h-24 bg-[#FEF3C7] flex items-center justify-center text-[44px]">
          {emoji}
        </div>

        {/* Filename 24px Bold per spec */}
        <h1 class="text-2xl font-bold text-stora-foreground text-center truncate w-full">{item.filename || item.name || "未命名文件"}</h1>

        {/* Meta row per spec */}
        <div class="flex items-center gap-6 text-sm font-medium text-stora-muted-foreground">
          <span>📦 {fmtSize(item.file_size)}</span>
          <span>📄 {fileExt} 文件</span>
        </div>

        {/* Divider */}
        <div class="w-full h-px bg-stora-border" />

        {/* Download button 464x56 per spec */}
        {item.id && (
          <button onClick$={() => { window.open(`/api/v2/share/${shareCode}/download`, "_blank"); }}
            class="w-[464px] max-w-full h-14 text-base font-semibold text-white bg-stora-primary hover:bg-[#1D4ED8] flex items-center justify-center gap-2">
            ⬇ 下载文件 ({fmtSize(item.file_size)})
          </button>
        )}

        {/* Preview area 464x200 per spec */}
        <div class="w-[464px] max-w-full h-[200px] bg-stora-background flex flex-col items-center justify-center gap-3">
          <div class="w-16 h-16 bg-stora-tag-work flex items-center justify-center text-[28px]">👁</div>
          <p class="text-sm text-stora-nav-text">点击下载以查看完整文件</p>
        </div>

        {/* Save button 464x48 per spec */}
        <button onClick$={async () => {
          try { await fetch(`/api/v2/share/${shareCode}/save`, { method: "POST" }); alert("已转存到你的 Stora"); } catch { alert("转存失败，请先登录"); }
        }} class="w-[464px] max-w-full h-12 text-[15px] font-medium text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-background flex items-center justify-center gap-2">
          📥 转存到我的 Stora
        </button>

        {/* Secondary actions per spec */}
        <div class="flex items-center gap-8 text-sm text-stora-nav-text">
          <span class="cursor-pointer hover:text-stora-foreground">🚩 举报</span>
          <span class="cursor-pointer hover:text-stora-foreground">👁 在线预览</span>
          <span class="text-base font-semibold cursor-pointer hover:text-stora-foreground">⋯ 更多</span>
        </div>

        {/* Brand trace per spec */}
        <div class="flex items-center gap-2">
          <div class="w-6 h-6 bg-stora-primary flex items-center justify-center text-white text-sm font-bold">S</div>
          <span class="text-sm font-medium text-stora-muted-foreground">通过 Stora 安全分享</span>
        </div>

        {/* Security notice per spec */}
        <div class="flex items-center gap-1.5 px-2.5 py-2">
          <span class="text-xs text-stora-nav-text">🔒 此链接仅用于查看和下载，文件内容受加密保护</span>
        </div>
      </div>
    </div>
  );
});
