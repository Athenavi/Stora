/**
 * Stora Preview Panel — floating slide-in pane with full metadata, tags, category, actions
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { api, type FileItem } from "~/lib/api";
import { listFileTags, assignFileTags, listUserTags, createTag } from "~/lib/api/files";
import { fmtSize } from "~/utils/format";

function fmtDate(iso: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function fmtDuration(s: number): string {
  if (!s || s <= 0) return "";
  const m = Math.floor(s / 60), sec = s % 60;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

const FILE_TYPES = ["image", "video", "audio", "document", "text", "archive", "other"];

export default component$<{ file: FileItem | null; onClose$: () => void }>(({ file, onClose$ }) => {
  const detail = useSignal<FileItem | null>(null);
  const textContent = useSignal<string | null>(null);
  const fileTags = useSignal<{ id: number; name: string; color: string | null }[]>([]);
  const allTags = useSignal<{ id: number; name: string; color: string | null }[]>([]);
  const editingDesc = useSignal(false);
  const descDraft = useSignal("");
  const showTagPicker = useSignal(false);
  const newTagName = useSignal("");
  const saving = useSignal(false);

  const loadTags = $((fid: number) => {
    listFileTags(fid).then(t => { fileTags.value = t; }).catch(() => {});
    listUserTags().then(t => { allTags.value = t; }).catch(() => {});
  });

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ track }) => {
    track(() => file?.id);
    if (!file) { detail.value = null; textContent.value = null; fileTags.value = []; return; }
    detail.value = file;
    loadTags(file.id);
    if (file.file_type === "text" || file.mime_type?.startsWith("text/")) {
      api.get(`/files/preview/${file.id}/content`).then((r: any) => {
        textContent.value = typeof r === "string" ? r : r?.content ?? null;
      }).catch(() => { textContent.value = null; });
    }
  });

  const toggleTag = $((tagId: number) => {
    if (!file) return;
    const has = fileTags.value.some(t => t.id === tagId);
    if (has) {
      fileTags.value = fileTags.value.filter(t => t.id !== tagId);
    } else {
      const tag = allTags.value.find(t => t.id === tagId);
      if (tag) fileTags.value = [...fileTags.value, tag];
    }
    assignFileTags(file.id, fileTags.value.map(t => t.id)).catch(() => {});
  });

  const createAndAssign = $(async () => {
    if (!newTagName.value.trim() || !file) return;
    try {
      const created = await createTag(newTagName.value.trim());
      const tag = { id: created.id, name: newTagName.value.trim(), color: null };
      allTags.value = [...allTags.value, tag];
      fileTags.value = [...fileTags.value, tag];
      await assignFileTags(file.id, fileTags.value.map(t => t.id));
      newTagName.value = "";
    } catch {}
  });

  const saveDesc = $(async () => {
    if (!file) return;
    saving.value = true;
    try {
      await api.patch(`/files/${file.id}`, { description: descDraft.value });
      if (detail.value) detail.value = { ...detail.value, description: descDraft.value };
      editingDesc.value = false;
    } catch {}
    saving.value = false;
  });

  const toggleFav = $(async () => {
    if (!file || !detail.value) return;
    const next = !detail.value.is_favorite;
    try {
      await api.patch(`/files/${file.id}`, { is_favorite: next });
      detail.value = { ...detail.value, is_favorite: next };
    } catch {}
  });

  const setFileType = $(async (ft: string) => {
    if (!file) return;
    try {
      await api.patch(`/files/${file.id}`, { file_type: ft });
      if (detail.value) detail.value = { ...detail.value, file_type: ft };
    } catch {}
  });

  if (!file) return null;

  const d = detail.value || file;
  const isMedia = d.file_type === "image" || d.file_type === "video";
  const isAudio = d.file_type === "audio";
  const isText = d.file_type === "text" || d.mime_type?.startsWith("text/");

  const panelBody = (
    <>
      {/* Preview area */}
      {isAudio ? (
        <div class="flex flex-col items-center justify-center py-6 bg-gradient-to-b from-indigo-50 to-slate-50">
          <div class="w-24 h-24 rounded-full bg-gradient-to-br from-indigo-400 to-purple-500 flex items-center justify-center mb-4 shadow-xl">
            <span class="text-4xl">🎵</span>
          </div>
          <audio controls class="w-full max-w-sm px-4" src={`/api/v2/files/download/${d.id}`} />
        </div>
      ) : isMedia ? (
        <div class="flex items-center justify-center bg-black/5 p-4 max-h-[40vh]">
          {d.file_type === "image" ? (
            <img src={`/api/v2/files/preview/${d.id}/thumbnail?size=1024`} alt={d.filename}
              class="max-w-full max-h-full object-contain rounded-lg shadow-lg" loading="lazy" />
          ) : (
            <video controls class="max-w-full max-h-full rounded-lg shadow-lg">
              <source src={`/api/v2/files/download/${d.id}`} />
            </video>
          )}
        </div>
      ) : isText && textContent.value !== null ? (
        <div class="p-4 max-h-[30vh] overflow-auto">
          <pre class="text-sm leading-relaxed font-mono whitespace-pre-wrap bg-stora-muted/50 rounded-lg p-3">{textContent.value}</pre>
        </div>
      ) : null}

      {/* Metadata section */}
      <div class="px-5 py-4 space-y-3 overflow-y-auto flex-1">
        {/* Tags */}
        <div>
          <div class="flex items-center gap-2 mb-2">
            <span class="text-xs font-semibold text-stora-muted-foreground uppercase tracking-wider">标签</span>
            <button onClick$={() => showTagPicker.value = !showTagPicker.value}
              class="text-xs text-stora-primary hover:underline">管理</button>
          </div>
          <div class="flex flex-wrap gap-1.5">
            {fileTags.value.length === 0 && <span class="text-xs text-stora-muted-foreground">无标签</span>}
            {fileTags.value.map(t => (
              <span key={t.id}
                class="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full"
                style={{ backgroundColor: t.color || "#e0e7ff", color: "#1e40af" }}>
                {t.name}
                <button onClick$={async () => { await toggleTag(t.id); }}
                  class="ml-0.5 hover:opacity-60">&times;</button>
              </span>
            ))}
          </div>
          {showTagPicker.value && (
            <div class="mt-2 p-3 bg-stora-muted rounded-lg space-y-2">
              <div class="flex flex-wrap gap-1.5">
                {allTags.value
                  .filter(t => !fileTags.value.some(ft => ft.id === t.id))
                  .map(t => (
                    <button key={t.id} onClick$={() => toggleTag(t.id)}
                      class="px-2 py-0.5 text-xs rounded-full border border-stora-border hover:bg-stora-primary hover:text-white transition-colors"
                      style={{ borderColor: t.color || undefined }}>
                      + {t.name}
                    </button>
                  ))}
              </div>
              <div class="flex gap-2">
                <input type="text" bind:value={newTagName} placeholder="新建标签..."
                  class="flex-1 px-2 py-1 text-xs border border-stora-border rounded"
                  onKeyDown$={(e: any) => { if (e.key === "Enter") createAndAssign(); }} />
                <button onClick$={createAndAssign}
                  class="px-3 py-1 text-xs font-medium text-white bg-stora-primary rounded hover:bg-[#1D4ED8]">创建</button>
              </div>
            </div>
          )}
        </div>

        {/* Category */}
        <div>
          <span class="text-xs font-semibold text-stora-muted-foreground uppercase tracking-wider block mb-1.5">分类</span>
          <select value={d.file_type}
            onChange$={(e: any) => setFileType(e.target.value)}
            class="w-full text-sm px-3 py-1.5 border border-stora-border rounded bg-white text-stora-foreground">
            {FILE_TYPES.map(ft => (
              <option key={ft} value={ft}>
                {{image:"🖼 图片",video:"🎬 视频",audio:"🎵 音频",document:"📄 文档",text:"📝 文本",archive:"📦 压缩包",other:"📎 其他"}[ft] || ft}
              </option>
            ))}
          </select>
        </div>

        {/* Attributes */}
        <div class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
          <AttrRow label="文件名" value={d.filename} />
          <AttrRow label="原始文件名" value={d.original_filename || "—"} />
          <AttrRow label="大小" value={fmtSize(d.file_size)} />
          <AttrRow label="MIME 类型" value={d.mime_type || "—"} />
          {d.width && d.height ? <AttrRow label="尺寸" value={`${d.width} × ${d.height}`} /> : null}
          {d.duration !== undefined && d.duration !== null && d.duration > 0 ? (
            <AttrRow label="时长" value={fmtDuration(d.duration)} />
          ) : null}
          <AttrRow label="下载次数" value={String(d.download_count ?? 0)} />
          <AttrRow label="加密" value={d.is_encrypted ? "是 🔒" : "否"} />
          <AttrRow label="创建时间" value={fmtDate(d.created_at!)} />
          <AttrRow label="更新时间" value={fmtDate(d.updated_at!)} />
          {d.file_hash && <div class="col-span-2"><AttrRow label="文件哈希" value={d.file_hash.substring(0, 16) + "..."} /></div>}
        </div>

        {/* Description */}
        <div>
          <div class="flex items-center gap-2 mb-1">
            <span class="text-xs font-semibold text-stora-muted-foreground uppercase tracking-wider">描述</span>
            {!editingDesc.value && (
              <button onClick$={() => { descDraft.value = d.description || ""; editingDesc.value = true; }}
                class="text-xs text-stora-primary hover:underline">编辑</button>
            )}
          </div>
          {editingDesc.value ? (
            <div class="space-y-2">
              <textarea bind:value={descDraft} rows={3}
                class="w-full text-sm px-3 py-2 border border-stora-border rounded resize-none focus:outline-none focus:ring-1 focus:ring-stora-primary"
                placeholder="添加描述..." />
              <div class="flex gap-2">
                <button onClick$={saveDesc} disabled={saving.value}
                  class="px-3 py-1 text-xs font-medium text-white bg-stora-primary rounded hover:bg-[#1D4ED8] disabled:opacity-50">
                  {saving.value ? "保存中..." : "保存"}
                </button>
                <button onClick$={() => editingDesc.value = false}
                  class="px-3 py-1 text-xs font-medium text-stora-foreground bg-stora-card border border-stora-border rounded">取消</button>
              </div>
            </div>
          ) : (
            <p class="text-sm text-stora-foreground">{d.description || <span class="text-stora-muted-foreground">无描述</span>}</p>
          )}
        </div>
      </div>

      {/* Footer actions */}
      <div class="flex items-center gap-2 px-5 py-3 border-t border-stora-border shrink-0 bg-stora-muted/30">
        <button onClick$={toggleFav}
          class={`touch-target px-3 py-2 text-sm rounded-lg transition-colors ${d.is_favorite ? "bg-amber-100 text-amber-700" : "bg-white text-stora-muted-foreground border border-stora-border hover:bg-stora-muted"}`}>
          {d.is_favorite ? "⭐ 已收藏" : "☆ 收藏"}
        </button>
        <div class="flex-1" />
        <a href={`/view?id=${d.id}`} target="_blank"
          class="touch-target px-4 py-2 text-sm font-medium text-white bg-stora-primary hover:bg-[#1D4ED8] rounded-lg transition-colors">
          全屏查看
        </a>
        <a href={`/api/v2/files/download/${d.id}`} download
          class="touch-target px-4 py-2 text-sm font-medium text-stora-foreground bg-white border border-stora-border hover:bg-stora-muted rounded-lg transition-colors">
          下载
        </a>
      </div>
    </>
  );

  return (
    <>
      {/* Backdrop */}
      <div class="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm hidden lg:block" onClick$={onClose$} />

      {/* Mobile: bottom sheet */}
      <div class="lg:hidden fixed inset-0 z-50 flex flex-col">
        <div class="flex-1 min-h-0" onClick$={onClose$} />
        <div class="bg-white rounded-t-2xl shadow-2xl flex flex-col max-h-[85vh] overflow-hidden">
          <div class="flex items-center justify-between px-5 py-3 border-b border-stora-border shrink-0">
            <div class="flex items-center gap-3 min-w-0 flex-1">
              <div class="text-2xl">{d.file_type === "image" ? "🖼️" : d.file_type === "video" ? "🎬" : d.file_type === "audio" ? "🎵" : "📄"}</div>
              <div class="min-w-0">
                <p class="text-sm font-semibold text-stora-foreground truncate">{d.filename}</p>
                <p class="text-xs text-stora-muted-foreground">{fmtSize(d.file_size)}</p>
              </div>
            </div>
            <button onClick$={onClose$} class="touch-target w-9 h-9 flex items-center justify-center text-stora-muted-foreground hover:bg-stora-muted rounded-full shrink-0"><span class="text-lg">✕</span></button>
          </div>
          <div class="flex-1 overflow-auto">{panelBody}</div>
        </div>
      </div>

      {/* Desktop: right slide-in panel */}
      <div class="hidden lg:block fixed right-0 top-0 bottom-0 z-50 w-[45vw] max-w-2xl min-w-[420px] bg-white shadow-2xl flex flex-col border-l border-stora-border animate-slide-in">
        <div class="flex items-center justify-between px-6 py-4 border-b border-stora-border shrink-0">
          <div class="flex items-center gap-3 min-w-0 flex-1">
            <div class="text-2xl">{d.file_type === "image" ? "🖼️" : d.file_type === "video" ? "🎬" : d.file_type === "audio" ? "🎵" : "📄"}</div>
            <div class="min-w-0">
              <p class="text-sm font-semibold text-stora-foreground truncate">{d.filename}</p>
              <p class="text-xs text-stora-muted-foreground">{fmtSize(d.file_size)} · {fmtDate(d.created_at!)}</p>
            </div>
          </div>
          <button onClick$={onClose$} class="touch-target w-9 h-9 flex items-center justify-center text-stora-muted-foreground hover:bg-stora-muted rounded-full shrink-0"><span class="text-lg">✕</span></button>
        </div>
        <div class="flex-1 overflow-y-auto">{panelBody}</div>
      </div>
    </>
  );
});

const AttrRow = component$<{ label: string; value: string }>(({ label, value }) => (
  <div class="flex flex-col">
    <span class="text-xs text-stora-muted-foreground">{label}</span>
    <span class="text-sm text-stora-foreground truncate">{value}</span>
  </div>
));
