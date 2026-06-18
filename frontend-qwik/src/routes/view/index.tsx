/**
 * Stora File Preview �?view files in the browser
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useLocation } from "@builder.io/qwik-city";
import { getFile, type FileItem } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button } from "~/components/ui/Button";

export const useFileDetail = routeLoader$(async ({ url }) => {
  const id = url.searchParams.get("id");
  if (!id) return null;
  return await getFile(Number(id)).catch(() => null);
});

export default component$(() => {
  const file = useFileDetail();
  if (!file.value) return <div class="p-6 text-slate-400">文件不存�?/div>;

  const f = file.value;
  const isImage = f.file_type === "image";
  const isVideo = f.file_type === "video";
  const isAudio = f.file_type === "audio";
  const isPdf = f.mime_type === "application/pdf" || f.filename?.endsWith(".pdf");
  const isText = [".txt", ".md", ".json", ".xml", ".html", ".css", ".js", ".ts", ".py", ".md"].some(e => f.filename?.endsWith(e));

  return (
    <div class="flex flex-col h-full bg-slate-900">
      {/* Top bar */}
      <div class="flex items-center gap-3 px-6 py-3 bg-slate-800 border-b border-slate-700 shrink-0">
        <a href="/drive" class="p-1.5 rounded text-slate-400 hover:text-white hover:bg-slate-700 transition-colors">
          <Icon name="chevronLeft" size={20} />
        </a>
        <span class="text-sm text-slate-200 font-medium truncate">{f.filename}</span>
        <div class="flex-1" />
        <Button variant="secondary" size="sm" onClick$={() => window.open(`/api/v2/files/download/${f.id}`, "_blank")}>
          <Icon name="download" size={16} /> 下载
        </Button>
      </div>

      {/* Preview area */}
      <div class="flex-1 flex items-center justify-center p-8 overflow-auto">
        {isImage && <img src={`/api/v2/files/preview/${f.id}`} alt={f.filename} class="max-w-full max-h-full object-contain rounded-lg shadow-2xl" />}
        {isVideo && <video controls class="max-w-full max-h-full rounded-lg" src={`/api/v2/files/preview/${f.id}?t=stream`} />}
        {isAudio && <audio controls class="w-full max-w-md" src={`/api/v2/files/preview/${f.id}?t=stream`} />}
        {isPdf && <iframe src={`/api/v2/files/preview/${f.id}`} class="w-full h-full rounded-lg" />}
        {isText && <div class="w-full max-w-4xl">加载�?..</div>}
        {!isImage && !isVideo && !isAudio && !isPdf && !isText && (
          <div class="text-center text-slate-500">
            <div class="text-6xl mb-4">📄</div>
            <p class="text-lg">不支持预览此文件类型</p>
            <Button variant="primary" size="md" class="mt-4" onClick$={() => window.open(`/api/v2/files/download/${f.id}`, "_blank")}>
              <Icon name="download" size={16} /> 下载文件
            </Button>
          </div>
        )}
      </div>

      {/* Info panel */}
      <div class="flex items-center gap-6 px-6 py-3 bg-slate-800 border-t border-slate-700 text-xs text-slate-400 shrink-0">
        <span>大小: {(f.file_size / 1024).toFixed(1)} KB</span>
        <span>类型: {f.file_type}</span>
        {f.mime_type && <span>MIME: {f.mime_type}</span>}
        {f.width && f.height && <span>尺寸: {f.width}×{f.height}</span>}
      </div>
    </div>
  );
});
