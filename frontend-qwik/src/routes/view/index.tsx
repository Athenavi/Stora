/**
 * Stora File Preview — 文件预览（视频/音频/文档编辑 + Flyfish Viewer + 转码清晰度切换）
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$, useLocation } from "@builder.io/qwik-city";
import { createServerApi, api } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import { Button } from "~/components/ui/Button";

export const useFileDetail = routeLoader$(async ({ url, request }) => {
  const id = url.searchParams.get("id");
  if (!id) return null;
  const srv = createServerApi(request);
  return await srv.get(`/files/${id}`).catch(() => null);
});

const EDITABLE_EXTS = [".txt", ".md", ".json", ".xml", ".html", ".css", ".js", ".ts", ".py",
  ".java", ".cpp", ".h", ".c", ".rb", ".go", ".rs", ".sh", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".log", ".csv", ".sql"];

const OFFICE_EXTS = ["docx", "xlsx", "pptx", "doc", "xls", "ppt", "odt", "ods", "odp"];
const isOffice = OFFICE_EXTS.includes(ext || "");

export default component$(() => {
  const file = useFileDetail();
  const viewerRef = useSignal();
  const editing = useSignal(false);
  const editContent = useSignal("");
  const saving = useSignal(false);
  const resolutions = useSignal<{label:string;url:string}[]>([]);
  const currentRes = useSignal("original");
  const transcodeLoading = useSignal(false);
  const showEditor = useSignal(false);
  const subtitleAvailable = useSignal(false);
  const subtitleLoading = useSignal(false);

  if (!file.value) return <div class="p-6 text-slate-400">文件不存在</div>;

  const f = file.value;
  const fileUrl = `/api/v2/files/download/${f.id}`;
  const previewUrl = `/api/v2/files/preview/${f.id}/${encodeURIComponent(f.filename)}`;
  const isVideo = f.file_type === "video";
  const isAudio = f.file_type === "audio";
  const ext = f.filename?.split(".").pop()?.toLowerCase();
  const isEditable = EDITABLE_EXTS.some(e => e.endsWith(ext || ""));

  // Check for transcoded resolutions on video files
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    if (!isVideo) return;
    try {
      const tasks: any[] = await api.get(`/files/transcode/${f.id}/tasks`);
      const completed = tasks.find(t => t.status === "completed");
      if (completed?.output_files?.length) {
        const list = [{ label: "original", url: previewUrl }];
        for (const o of completed.output_files) {
          list.push({ label: o.label, url: `/api/v2/files/stream/${f.id}/${o.label}` });
        }
        resolutions.value = list;
      }
    } catch {}
  });

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    if (!viewerRef.value || isVideo || isAudio || editing.value) return;
    try {
      const { mountViewerFrame } = await import('@flyfish-group/file-viewer-web');
      mountViewerFrame(viewerRef.value, {
        url: previewUrl,
        name: f.filename,
        options: { theme: 'dark', toolbar: { download: true, print: true } },
      });
    } catch (e) { console.error('Failed to mount file viewer:', e); }
  });

  // Check subtitle availability
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    if (!isVideo) return;
    try {
      const st = await api.get<any>(`/files/transcribe/${f.id}/status`);
      if (st.available && st.status === "completed") {
        subtitleAvailable.value = true;
      }
    } catch {}
  });

  const startEditing = async () => {
    try {
      const resp = await fetch(previewUrl);
      editContent.value = await resp.text();
      editing.value = true;
    } catch {}
  };

  const saveContent = async () => {
    saving.value = true;
    try {
      await api.put(`/files/${f.id}/content`, { content: editContent.value });
      editing.value = false;
    } catch {}
    saving.value = false;
  };

  const doTranscode = async () => {
    transcodeLoading.value = true;
    try {
      await api.post(`/files/transcode/${f.id}`);
      alert("转码任务已创建，请稍后刷新页面查看");
    } catch (e: any) { alert(e.message || "转码失败"); }
    transcodeLoading.value = false;
  };

  return (
    <div class="flex flex-col h-full bg-slate-900">
      {/* Top bar */}
      <div class="flex items-center gap-3 px-6 py-3 bg-slate-800 border-b border-slate-700 shrink-0">
        <a href="/drive" class="p-1.5 rounded text-slate-400 hover:text-white hover:bg-slate-700 transition-colors">
          <Icon name="chevronLeft" size={20} />
        </a>
        <span class="text-sm text-slate-200 font-medium truncate">{f.filename}</span>
        <div class="flex-1" />
        {isEditable && !editing.value && (
          <Button variant="secondary" size="sm" onClick$={startEditing}>
            <Icon name="edit" size={16} /> 编辑
          </Button>
        )}
        {isOffice && (
          <Button variant="secondary" size="sm" onClick$={async () => {
            try {
              const res = await api.get<{ editor_url: string }>(`/wopi/access/${f.id}`);
              if (res?.editor_url) window.open(res.editor_url, "_blank");
            } catch { alert("Office 编辑服务不可用"); }
          }}>
            <Icon name="file" size={16} /> 在 Office 中编辑
          </Button>
        )}
        {f.file_type === "image" && (
          <Button variant="secondary" size="sm" onClick$={() => showEditor.value = true}>
            <Icon name="image" size={16} /> 编辑图片
          </Button>
        )}
        {isVideo && subtitleAvailable.value && (
          <Button variant="secondary" size="sm" onClick$={() => {
            const video = document.querySelector('video');
            const track = video?.querySelector('track');
            if (track) track.track.mode = track.track.mode === 'showing' ? 'hidden' : 'showing';
          }}>
            <Icon name="file" size={16} /> 字幕
          </Button>
        )}
        {isVideo && !subtitleAvailable.value && (
          <Button variant="secondary" size="sm" loading={subtitleLoading.value} onClick$={async () => {
            subtitleLoading.value = true;
            try {
              await api.post(`/files/transcribe/${f.id}`);
              const poll = setInterval(async () => {
                try {
                  const st = await api.get<any>(`/files/transcribe/${f.id}/status`);
                  if (st.status === "completed") {
                    subtitleAvailable.value = true;
                    subtitleLoading.value = false;
                    clearInterval(poll);
                    location.reload();
                  } else if (st.status === "failed") {
                    clearInterval(poll);
                    subtitleLoading.value = false;
                    alert("字幕生成失败: " + (st.error_message || "未知错误"));
                  }
                } catch { clearInterval(poll); subtitleLoading.value = false; }
              }, 3000);
            } catch (e: any) {
              subtitleLoading.value = false;
              alert(e.message || "创建转录任务失败");
            }
          }}>
            <Icon name="file" size={16} /> 生成字幕
          </Button>
        )}
        {editing.value && (
          <>
            <Button variant="secondary" size="sm" onClick$={saveContent} loading={saving.value}>
              <Icon name="check" size={16} /> 保存
            </Button>
            <Button variant="ghost" size="sm" onClick$={() => editing.value = false}>
              取消
            </Button>
          </>
        )}
        <Button variant="secondary" size="sm" onClick$={() => window.open(fileUrl, "_blank")}>
          <Icon name="download" size={16} /> 下载
        </Button>
      </div>

      {/* 预览/编辑容器 */}
      {isVideo ? (
        <div class="flex-1 flex items-center justify-center bg-black">
          <video controls autoplay class="max-w-full max-h-full" preload="metadata">
            <source src={previewUrl} type={f.mime_type || "video/mp4"} />
            {subtitleAvailable.value && <track kind="subtitles" src={`/api/v2/files/transcribe/${f.id}/subtitle`} srclang="zh" label="中文" default />}
          </video>
        </div>
      ) : isAudio ? (
        <div class="flex-1 flex flex-col items-center justify-center bg-slate-800 gap-6">
          <div class="w-32 h-32 rounded-full bg-indigo-600/20 flex items-center justify-center">
            <Icon name="music" size={48} class="text-indigo-400" />
          </div>
          <div class="w-full max-w-lg px-8">
            <audio controls class="w-full" preload="metadata" autoplay>
              <source src={previewUrl} type={f.mime_type || "audio/mpeg"} />
            </audio>
          </div>
        </div>
      ) : editing.value ? (
        <textarea class="flex-1 bg-slate-900 text-slate-100 font-mono text-sm p-6 resize-none focus:outline-none scrollbar-thin"
          bind:value={editContent}
          onKeyDown$={(e: any) => { if ((e.ctrlKey || e.metaKey) && e.key === "s") { e.preventDefault(); saveContent(); } }} />
      ) : (
        <div ref={viewerRef} class="flex-1 overflow-hidden" />
      )}

      {/* Info panel */}
      <div class="flex items-center gap-6 px-6 py-3 bg-slate-800 border-t border-slate-700 text-xs text-slate-400 shrink-0">
        <span>大小: {(f.file_size / 1024).toFixed(1)} KB</span>
        <span>类型: {f.file_type}</span>
        {f.mime_type && <span>MIME: {f.mime_type}</span>}
        {f.width && f.height && <span>尺寸: {f.width}x{f.height}</span>}
        {editing.value && <span>💡 Ctrl+S 保存</span>}
      </div>

      {/* Image editor overlay */}
      {showEditor.value && (
        <ImageEditor
          fileId={f.id}
          imageUrl={fileUrl}
          filename={f.filename}
          onClose={() => showEditor.value = false}
          onSaved={() => { location.reload(); }}
        />
      )}
    </div>
  );
});