/**
 * Stora Preview Modal — 响应式全屏预览模态框
 * 在当前位置弹出，不打开新页面/新窗口
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { api } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";

interface Props {
  fileId: number;
  onClose$: () => void;
}

export default component$<Props>(({ fileId, onClose$ }) => {
  const file = useSignal<any>(null);
  const viewerRef = useSignal<HTMLDivElement>();

  // 获取文件详情
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    try {
      file.value = await api.get(`/files/${fileId}`);
    } catch {}
  });

  // 挂载 Flyfish viewer（非视频/音频文件）
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    if (!viewerRef.value || !file.value) return;
    const type = file.value.file_type;
    if (type === "video" || type === "audio") return;
    try {
      const { mountViewerFrame } = await import('@flyfish-group/file-viewer-web');
      const previewUrl = `/api/v2/files/preview/${file.value.id}/${encodeURIComponent(file.value.filename)}`;
      mountViewerFrame(viewerRef.value, {
        url: previewUrl,
        name: file.value.filename,
        options: { theme: 'dark', toolbar: { download: true, print: true } },
      });
    } catch (e) { console.error('Failed to mount file viewer:', e); }
  });

  // ESC 关闭
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose$();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  });

  const downloadUrl = `/api/v2/files/download/${fileId}`;

  // 加载中
  if (!file.value) {
    return (
      <div class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center"
        onClick$={onClose$}>
        <div class="text-slate-400 text-sm" onClick$={(e) => e.stopPropagation()}>加载中...</div>
      </div>
    );
  }

  const f = file.value;
  const previewUrl = `/api/v2/files/preview/${f.id}/${encodeURIComponent(f.filename)}`;
  const isVideo = f.file_type === "video";
  const isAudio = f.file_type === "audio";

  return (
    <div class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-0 sm:p-4"
      onClick$={onClose$}>

      <div class="bg-slate-900 w-full h-full sm:rounded-2xl sm:max-w-6xl sm:max-h-[90vh] flex flex-col sm:shadow-2xl overflow-hidden"
        onClick$={(e) => e.stopPropagation()}>

        {/* 顶栏 */}
        <div class="flex items-center gap-3 px-4 py-3 bg-slate-800 border-b border-slate-700 shrink-0">
          <button onClick$={onClose$}
            class="p-1.5 rounded text-slate-400 hover:text-white hover:bg-slate-700 transition-colors">
            <Icon name="close" size={20} />
          </button>
          <span class="text-sm text-slate-200 font-medium truncate">{f.filename}</span>
          <div class="flex-1" />
          <button onClick$={() => window.open(downloadUrl, "_blank")}
            class="flex items-center gap-1 px-2.5 py-1.5 text-xs rounded text-slate-400 hover:text-white hover:bg-slate-700 transition-colors">
            <Icon name="download" size={14} /> 下载
          </button>
        </div>

        {/* 预览内容 */}
        {isVideo ? (
          <div class="flex-1 flex items-center justify-center bg-black min-h-0">
            <video controls autoplay class="max-w-full max-h-full" preload="metadata">
              <source src={previewUrl} type={f.mime_type || "video/mp4"} />
            </video>
          </div>
        ) : isAudio ? (
          <div class="flex-1 flex flex-col items-center justify-center bg-slate-800 gap-6 min-h-0">
            <div class="w-32 h-32 rounded-full bg-indigo-600/20 flex items-center justify-center">
              <Icon name="music" size={48} class="text-indigo-400" />
            </div>
            <div class="w-full max-w-lg px-8">
              <audio controls class="w-full" preload="metadata" autoplay>
                <source src={previewUrl} type={f.mime_type || "audio/mpeg"} />
              </audio>
            </div>
          </div>
        ) : (
          <div ref={viewerRef} class="flex-1 overflow-hidden min-h-0" />
        )}

        {/* 信息栏 */}
        <div class="flex items-center gap-3 md:gap-6 px-3 md:px-6 py-3 bg-slate-800 border-t border-slate-700 text-xs text-slate-400 shrink-0">
          <span>大小: {(f.file_size / 1024).toFixed(1)} KB</span>
          <span>类型: {f.file_type}</span>
          {f.mime_type && <span>MIME: {f.mime_type}</span>}
          {f.width && f.height && <span>尺寸: {f.width}x{f.height}</span>}
        </div>
      </div>
    </div>
  );
});
