/**
 * Stora Preview Panel — floating slide-in pane adapting to file type
 *
 *  layout per type:
 *   image / video   → media-focused right panel (45vw)
 *   audio           → player card with album-art vibe
 *   document / text → text viewer, wider panel
 *   other           → file info card + download / open in full view
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { api, type FileItem } from "~/lib/api";
import { fmtSize } from "~/utils/format";

function fmtDate(iso: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export default component$<{ file: FileItem | null; onClose$: () => void }>(({ file, onClose$ }) => {
  const detail = useSignal<FileItem | null>(null);
  const textContent = useSignal<string | null>(null);

  // Fetch full detail when file changes
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ track }) => {
    track(() => file?.id);
    if (!file) { detail.value = null; textContent.value = null; return; }
    // Use what we have; fetch full detail if needed
    detail.value = file;
    if (file.file_type === "text" || file.mime_type?.startsWith("text/")) {
      api.get(`/files/preview/${file.id}/content`).then((r: any) => {
        textContent.value = typeof r === "string" ? r : r?.content ?? null;
      }).catch(() => { textContent.value = null; });
    }
  });

  if (!file) return null;

  const isMedia = file.file_type === "image" || file.file_type === "video";
  const isAudio = file.file_type === "audio";
  const isText = file.file_type === "text" || file.mime_type?.startsWith("text/");

  return (
    <>
      {/* Backdrop */}
      <div class="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm hidden lg:block"
        onClick$={onClose$} />

      {/* Mobile: bottom sheet */}
      <div class="lg:hidden fixed inset-0 z-50 flex flex-col">
        <div class="flex-1 min-h-0" onClick$={onClose$} />
        <div class="bg-white rounded-t-2xl shadow-2xl flex flex-col max-h-[85vh] overflow-hidden">
          {/* Handle + header */}
          <div class="flex items-center justify-between px-5 py-3 border-b border-stora-border shrink-0">
            <div class="flex items-center gap-3 min-w-0 flex-1">
              <div class="flex items-center justify-center w-9 h-9 shrink-0">
                <TypeIcon type={file.file_type} />
              </div>
              <div class="min-w-0">
                <p class="text-sm font-semibold text-stora-foreground truncate">{file.filename}</p>
                <p class="text-xs text-stora-muted-foreground">{fmtSize(file.file_size)} · {fmtDate(file.created_at!)}</p>
              </div>
            </div>
            <button onClick$={onClose$}
              class="touch-target w-9 h-9 flex items-center justify-center text-stora-muted-foreground hover:bg-stora-muted rounded-full shrink-0">
              <span class="text-lg">✕</span>
            </button>
          </div>
          <div class="flex-1 overflow-auto p-4">
            <MobilePreview file={file} detail={detail.value} textContent={textContent.value} />
          </div>
          <div class="p-4 border-t border-stora-border shrink-0">
            <ActionButtons file={file} onClose$={onClose$} />
          </div>
        </div>
      </div>

      {/* Desktop: right slide-in panel */}
      <div class="hidden lg:block fixed right-0 top-0 bottom-0 z-50 w-[45vw] max-w-2xl min-w-[420px] bg-white shadow-2xl flex flex-col border-l border-stora-border animate-slide-in">
        {/* Header */}
        <div class="flex items-center justify-between px-6 py-4 border-b border-stora-border shrink-0">
          <div class="flex items-center gap-3 min-w-0 flex-1">
            <div class="flex items-center justify-center w-10 h-10 shrink-0">
              <TypeIcon type={file.file_type} />
            </div>
            <div class="min-w-0">
              <p class="text-sm font-semibold text-stora-foreground truncate">{file.filename}</p>
              <p class="text-xs text-stora-muted-foreground">{fmtSize(file.file_size)} · {fmtDate(file.created_at!)}</p>
            </div>
          </div>
          <button onClick$={onClose$}
            class="touch-target w-9 h-9 flex items-center justify-center text-stora-muted-foreground hover:bg-stora-muted rounded-full shrink-0">
            <span class="text-lg">✕</span>
          </button>
        </div>

        {/* Preview body */}
        <div class="flex-1 overflow-auto">
          {isAudio ? (
            <AudioPreview file={file} />
          ) : isMedia ? (
            <MediaPreview file={file} />
          ) : isText && textContent.value !== null ? (
            <TextViewer content={textContent.value} />
          ) : (
            <InfoFallback file={file} />
          )}
        </div>

        {/* Footer actions */}
        <div class="flex items-center gap-2 px-6 py-4 border-t border-stora-border shrink-0 bg-stora-muted/30">
          <ActionButtons file={file} onClose$={onClose$} />
        </div>
      </div>
    </>
  );
});

/** Type icon by file type */
function TypeIcon({ type }: { type?: string }) {
  const icons: Record<string, string> = {
    image: "🖼️", video: "🎬", audio: "🎵", document: "📄",
    text: "📝", archive: "📦", other: "📎",
  };
  return <span class="text-2xl">{icons[type || "other"] || icons.other}</span>;
}

/** Media (image/video) — large preview in right panel */
const MediaPreview = component$<{ file: FileItem }>(({ file }) => (
  <div class="flex items-center justify-center h-full bg-black/5 p-4">
    {file.file_type === "image" ? (
      <img src={`/api/v2/files/preview/${file.id}/thumbnail?size=1024`}
        alt={file.filename}
        class="max-w-full max-h-full object-contain rounded-lg shadow-lg"
        loading="lazy" />
    ) : (
      <video controls class="max-w-full max-h-full rounded-lg shadow-lg"
        onError$={(e: any) => { e.target.style.display = 'none'; }}>
        <source src={`/api/v2/files/download/${file.id}`} />
      </video>
    )}
  </div>
));

/** Audio player with visual flair */
const AudioPreview = component$<{ file: FileItem }>(({ file }) => (
  <div class="flex flex-col items-center justify-center h-full p-8 bg-gradient-to-b from-indigo-50 to-slate-50">
    <div class="w-32 h-32 rounded-full bg-gradient-to-br from-indigo-400 to-purple-500 flex items-center justify-center mb-6 shadow-xl">
      <span class="text-5xl">🎵</span>
    </div>
    <p class="text-lg font-semibold text-stora-foreground mb-1 text-center">{file.filename}</p>
    <p class="text-sm text-stora-muted-foreground mb-6">{fmtSize(file.file_size)}</p>
    <audio controls class="w-full max-w-sm" src={`/api/v2/files/download/${file.id}`} />
  </div>
));

/** Text / code viewer */
const TextViewer = component$<{ content: string }>(({ content }) => (
  <div class="p-4">
    <pre class="text-sm leading-relaxed text-stora-foreground bg-stora-muted/50 rounded-lg p-4 overflow-auto whitespace-pre-wrap font-mono max-h-[calc(100vh-220px)]">
      {content}
    </pre>
  </div>
));

/** Fallback info card for unsupported types */
const InfoFallback = component$<{ file: FileItem }>(({ file }) => (
  <div class="flex flex-col items-center justify-center h-full p-8 text-center">
    <div class="w-24 h-24 bg-stora-muted rounded-2xl flex items-center justify-center mb-5">
      <span class="text-5xl">📄</span>
    </div>
    <p class="text-sm text-stora-muted-foreground mb-6 max-w-xs">
      该文件类型暂不支持在线预览
    </p>
    <div class="w-full max-w-sm space-y-3 text-left">
      <InfoRow label="文件名" value={file.filename} />
      <InfoRow label="大小" value={fmtSize(file.file_size)} />
      <InfoRow label="类型" value={file.mime_type || "未知"} />
      <InfoRow label="修改时间" value={fmtDate(file.created_at!)} />
      {file.width && file.height && <InfoRow label="尺寸" value={`${file.width} × ${file.height}`} />}
      {file.duration !== undefined && file.duration !== null && file.duration > 0 && (
        <InfoRow label="时长" value={`${Math.floor(file.duration / 60)}:${String(file.duration % 60).padStart(2, "0")}`} />
      )}
    </div>
  </div>
));

/** Mobile preview — simpler, space-efficient */
const MobilePreview = component$<{ file: FileItem; detail: FileItem | null; textContent: string | null }>(({ file, detail, textContent }) => {
  if (file.file_type === "image") {
    return <img src={`/api/v2/files/preview/${file.id}/thumbnail?size=1024`} alt={file.filename}
      class="w-full rounded-lg" loading="lazy" />;
  }
  if (file.file_type === "video") {
    return <video controls class="w-full rounded-lg"><source src={`/api/v2/files/download/${file.id}`} /></video>;
  }
  if (file.file_type === "audio") {
    return (
      <div class="flex flex-col items-center py-4">
        <div class="w-20 h-20 rounded-full bg-gradient-to-br from-indigo-400 to-purple-500 flex items-center justify-center mb-4">
          <span class="text-3xl">🎵</span>
        </div>
        <audio controls class="w-full" src={`/api/v2/files/download/${file.id}`} />
      </div>
    );
  }
  if (file.file_type === "text" && textContent) {
    return <pre class="text-sm font-mono whitespace-pre-wrap bg-stora-muted/50 rounded-lg p-3 max-h-64 overflow-auto">{textContent}</pre>;
  }
  return (
    <div class="space-y-2 text-sm">
      <InfoRow label="文件名" value={file.filename} />
      <InfoRow label="大小" value={fmtSize(file.file_size)} />
      <InfoRow label="类型" value={file.mime_type || "未知"} />
      <InfoRow label="修改时间" value={fmtDate(file.created_at!)} />
    </div>
  );
});

/** Info row label-value pair */
const InfoRow = component$<{ label: string; value: string }>(({ label, value }) => (
  <div class="flex items-center gap-3 text-sm">
    <span class="text-stora-muted-foreground w-20 shrink-0">{label}</span>
    <span class="text-stora-foreground truncate">{value}</span>
  </div>
));

/** Bottom action buttons */
const ActionButtons = component$<{ file: FileItem; onClose$: () => void }>(({ file, onClose$ }) => (
  <>
    <a href={`/view?id=${file.id}`}
      class="flex-1 touch-target px-4 py-2.5 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] text-center rounded-lg transition-colors"
      target="_blank">
      全屏查看
    </a>
    <a href={`/api/v2/files/download/${file.id}`} download
      class="flex-1 touch-target px-4 py-2.5 text-sm font-medium text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-muted text-center rounded-lg transition-colors">
      下载
    </a>
  </>
));
