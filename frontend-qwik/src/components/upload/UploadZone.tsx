/**
 * Stora 拖拽上传组件
 *
 * 所有事件处理逻辑完全内联在 `$` 函数中，满足 Qwik 序列化要求。
 */
import { component$, useSignal } from "@builder.io/qwik";
import { TaskState, storeFile, processUploadTask } from "./upload-utils";

function fmtSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

export default component$<{ folderId?: number | null }>(({ folderId }) => {
  const isDragging = useSignal(false);
  const tasks = useSignal<TaskState[]>([]);

  return (
    <div>
      {/* Drop zone — also acts as file picker on click */}
      <div
        preventdefault:dragover
        preventdefault:drop
        onDragOver$={() => { isDragging.value = true; }}
        onDragLeave$={() => { isDragging.value = false; }}
        onDrop$={(e: DragEvent) => {
          isDragging.value = false;
          const list = e.dataTransfer?.files;
          if (!list?.length) return;
          const next: TaskState[] = [];
          for (let i = 0; i < list.length; i++) {
            const id = `${Date.now()}-${i}`;
            storeFile(id, list[i]);
            next.push({ id, filename: list[i].name, size: list[i].size, progress: 0, status: "pending" });
          }
          tasks.value = [...tasks.value, ...next];
          next.forEach(t => processUploadTask(t, folderId, () => { tasks.value = [...tasks.value]; }));
        }}
        onClick$={() => {
          const inp = document.createElement("input");
          inp.type = "file"; inp.multiple = true;
          inp.onchange = () => {
            if (!inp.files?.length) return;
            const list = inp.files;
            const next: TaskState[] = [];
            for (let i = 0; i < list.length; i++) {
              const id = `${Date.now()}-${i}`;
              storeFile(id, list[i]);
              next.push({ id, filename: list[i].name, size: list[i].size, progress: 0, status: "pending" });
            }
            tasks.value = [...tasks.value, ...next];
            next.forEach(t => processUploadTask(t, folderId, () => { tasks.value = [...tasks.value]; }));
          };
          inp.click();
        }}
        class={`relative border-2 border-dashed rounded-xl p-8 text-center transition-all cursor-pointer ${
          isDragging.value ? "border-indigo-500 bg-indigo-50 scale-[1.02]" : "border-slate-300 hover:border-indigo-400 bg-slate-50"
        }`}
      >
        {isDragging.value ? (
          <div class="text-indigo-600">
            <div class="text-5xl mb-3">📥</div>
            <p class="text-lg font-medium">释放以上传文件</p>
          </div>
        ) : (
          <div class="text-slate-400">
            <div class="text-5xl mb-3">📤</div>
            <p class="text-lg font-medium text-slate-600 mb-1">拖拽文件到此处，或 <span class="text-indigo-600">点击选择</span></p>
            <p class="text-sm">所有文件类型 · 大文件自动分片 · 秒传检测</p>
          </div>
        )}
      </div>

      {/* Upload task list */}
      {tasks.value.length > 0 && (
        <div class="mt-4 space-y-2 max-h-64 overflow-auto">
          {tasks.value.map((t) => (
            <div key={t.id} class="flex items-center gap-3 px-4 py-2.5 bg-white rounded-lg border border-slate-200 shadow-sm">
              <span class="text-base shrink-0">
                {t.status === "completed" ? "✅" : t.status === "error" ? "❌" : t.status === "checking" ? "🔍" : "📄"}
              </span>
              <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between gap-2">
                  <span class="text-sm font-medium text-slate-700 truncate">{t.filename}</span>
                  <span class="text-xs text-slate-400 shrink-0">
                    {t.status === "completed" ? "完成" : t.status === "error" ? t.error : fmtSize(t.size)}
                  </span>
                </div>
                {(t.status === "uploading" || t.status === "checking") && (
                  <div class="mt-1 w-full bg-slate-100 rounded-full h-1.5 overflow-hidden">
                    <div
                      class="h-full bg-indigo-500 rounded-full transition-all duration-300"
                      style={{ width: `${t.progress}%` }}
                    />
                  </div>
                )}
              </div>
              {t.status === "error" && (
                <button onClick$={() => { tasks.value = tasks.value.filter(x => x.id !== t.id); }} class="text-slate-400 hover:text-red-500">✕</button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
});
