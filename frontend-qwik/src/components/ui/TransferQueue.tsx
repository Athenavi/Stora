/**
 * Stora TransferQueue — global upload/download progress floating panel
 * Tracks active transfers, shows progress bars, supports cancel/retry.
 */
import { component$, useSignal, useVisibleTask$, Slot } from "@builder.io/qwik";

export interface TransferTask {
  id: string;
  filename: string;
  type: "upload" | "download";
  size: number;
  progress: number;
  status: "pending" | "active" | "completed" | "error";
  error?: string;
}

// Global task registry (module-level so all components share it)
const _tasks = new Map<string, TransferTask>();
const _listeners = new Set<() => void>();

function notify() { _listeners.forEach(fn => fn()); }

export function addTask(task: TransferTask) {
  _tasks.set(task.id, task);
  notify();
}

export function updateTask(id: string, patch: Partial<TransferTask>) {
  const t = _tasks.get(id);
  if (t) Object.assign(t, patch);
  notify();
}

export function removeTask(id: string) {
  _tasks.delete(id);
  notify();
}

function fmtSize(b: number): string {
  if (!b) return "0 B";
  const k = 1024, i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

export default component$(() => {
  const open = useSignal(false);
  const tasks = useSignal<TransferTask[]>([]);
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const update = () => {
      tasks.value = Array.from(_tasks.values()).filter(t => t.status !== "completed");
    };
    _listeners.add(update);
    return () => _listeners.delete(update);
  });

  const activeCount = tasks.value.filter(t => t.status === "active" || t.status === "pending").length;

  return (
    <div class="h-full">
      {/* Trigger button — floating in bottom-right corner */}
      {activeCount > 0 && (
        <button onClick$={() => open.value = !open.value}
          class="fixed bottom-6 right-6 z-40 w-12 h-12 rounded-full bg-indigo-600 text-white shadow-lg hover:bg-indigo-700 transition-all flex items-center justify-center text-sm font-bold">
          {activeCount > 9 ? "9+" : activeCount}
        </button>
      )}

      {/* Panel */}
      {open.value && tasks.value.length > 0 && (
        <div class="fixed bottom-20 right-6 z-40 w-80 bg-white rounded-xl shadow-xl border border-slate-200 overflow-hidden">
          <div class="flex items-center justify-between px-4 py-3 border-b border-slate-100 bg-slate-50">
            <span class="text-sm font-medium text-slate-700">传输队列</span>
            <button onClick$={() => open.value = false} class="text-slate-400 hover:text-slate-600"><Icon name="x" size={16} /></button>
          </div>
          <div class="max-h-80 overflow-y-auto divide-y divide-slate-50">
            {tasks.value.map(t => (
              <div key={t.id} class="px-4 py-3">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-xs font-medium text-slate-700 truncate flex-1">{t.filename}</span>
                  <span class="text-xs text-slate-400">{fmtSize(t.size)}</span>
                </div>
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-1.5 bg-slate-100 rounded-full overflow-hidden">
                    <div class={`h-full rounded-full transition-all duration-300 ${
                      t.status === "error" ? "bg-red-500" : "bg-indigo-500"
                    }`} style={{ width: `${t.progress}%` }} />
                  </div>
                  <span class="text-xs text-slate-500 w-8 text-right">
                    {t.status === "error" ? "!" : `${t.progress}%`}
                  </span>
                </div>
                {t.error && <p class="text-xs text-red-500 mt-1">{t.error}</p>}
              </div>
            ))}
          </div>
        </div>
      )}
      <Slot />
    </div>
  );
});
