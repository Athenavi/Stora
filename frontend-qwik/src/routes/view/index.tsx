/**
 * Stora File Preview — 基于 Flyfish File Viewer 的通用文件预览
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$, useLocation } from "@builder.io/qwik-city";
import { createServerApi } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button } from "~/components/ui/Button";

export const useFileDetail = routeLoader$(async ({ url, request }) => {
  const id = url.searchParams.get("id");
  if (!id) return null;
  const api = createServerApi(request);
  return await api.get(`/files/${id}`).catch(() => null);
});

export default component$(() => {
  const file = useFileDetail();
  const viewerRef = useSignal();

  if (!file.value) return <div class="p-6 text-slate-400">文件不存在</div>;

  const f = file.value;
  const fileUrl = `/api/v2/files/download/${f.id}`;
  const previewUrl = `/api/v2/files/preview/${f.id}/${encodeURIComponent(f.filename)}`;

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    if (!viewerRef.value) return;
    try {
      const { mountViewerFrame } = await import('@flyfish-group/file-viewer-web');
      mountViewerFrame(viewerRef.value, {
        url: previewUrl,
        name: f.filename,
        options: {
          theme: 'dark',
          toolbar: { download: true, print: true },
        },
      });
    } catch (e) {
      console.error('Failed to mount file viewer:', e);
    }
  });

  return (
    <div class="flex flex-col h-full bg-slate-900">
      {/* Top bar */}
      <div class="flex items-center gap-3 px-6 py-3 bg-slate-800 border-b border-slate-700 shrink-0">
        <a href="/drive" class="p-1.5 rounded text-slate-400 hover:text-white hover:bg-slate-700 transition-colors">
          <Icon name="chevronLeft" size={20} />
        </a>
        <span class="text-sm text-slate-200 font-medium truncate">{f.filename}</span>
        <div class="flex-1" />
        <Button variant="secondary" size="sm" onClick$={() => window.open(fileUrl, "_blank")}>
          <Icon name="download" size={16} /> 下载
        </Button>
      </div>

      {/* 预览容器 */}
      <div ref={viewerRef} class="flex-1 overflow-hidden" />

      {/* Info panel */}
      <div class="flex items-center gap-6 px-6 py-3 bg-slate-800 border-t border-slate-700 text-xs text-slate-400 shrink-0">
        <span>大小: {(f.file_size / 1024).toFixed(1)} KB</span>
        <span>类型: {f.file_type}</span>
        {f.mime_type && <span>MIME: {f.mime_type}</span>}
        {f.width && f.height && <span>尺寸: {f.width}x{f.height}</span>}
      </div>
    </div>
  );
});