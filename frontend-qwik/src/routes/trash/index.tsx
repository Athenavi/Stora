/**
 * Stora Trash — enterprise recycle bin
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { Button, Skeleton } from "~/components/ui/Button";
import { api } from "~/lib/api";

interface TrashItem {
  id: number;
  filename: string;
  file_size: number;
  file_type: string;
  deleted_at: string | null;
  original_filename?: string;
}

export const useTrashList = routeLoader$(async () => {
  return await api.get<{ items: TrashItem[]; total: number }>("/files/trash").catch(() => ({ items: [], total: 0 }));
});

function fmtSize(bytes: number): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

function fmtDate(s: string | null | undefined): string {
  if (!s) return "";
  const d = new Date(s);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export default component$(() => {
  const data = useTrashList();
  const items = useSignal(data.value.items);
  const selIds = useSignal<number[]>([]);
  const loading = useSignal(false);

  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <div>
          <h1 class="text-lg font-semibold text-slate-900">回收站</h1>
          <p class="text-sm text-slate-500 mt-0.5">被删除的文件会在此保留</p>
        </div>
        <div class="flex items-center gap-2">
          {selIds.value.length > 0 && (
            <>
              <Button variant="secondary" size="sm" onClick$={async () => {
                loading.value = true;
                try {
                  await api.post("/files/trash/batch-restore", { file_ids: selIds.value });
                  items.value = items.value.filter(x => !selIds.value.includes(x.id));
                  selIds.value = [];
                } catch {}
                loading.value = false;
              }} loading={loading.value}>
                <Icon name="restore" size={16} /> 恢复 ({selIds.value.length})
              </Button>
              <Button variant="danger" size="sm" onClick$={async () => {
                if (!confirm("永久删除选中的文件？")) return;
                loading.value = true;
                try {
                  for (const id of selIds.value) await api.delete(`/files/trash/${id}/destroy`);
                  items.value = items.value.filter(x => !selIds.value.includes(x.id));
                  selIds.value = [];
                } catch {}
                loading.value = false;
              }} loading={loading.value}>
                <Icon name="trash" size={16} /> 永久删除
              </Button>
            </>
          )}
          {items.value.length > 0 && (
            <Button variant="ghost" size="sm" onClick$={async () => {
              if (!confirm("清空回收站？此操作不可恢复。")) return;
              try { await api.post("/files/trash/clear"); items.value = []; } catch {}
            }}>清空回收站</Button>
          )}
        </div>
      </div>

      <div class={`flex-1 overflow-auto scrollbar-thin ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {!data.value ? (
          <div class="p-4 sm:p-6 space-y-3">
            {[1,2,3].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><Skeleton class="w-5 h-5 rounded" /><Skeleton class="w-10 h-10 rounded-lg" /><div class="flex-1 space-y-2"><Skeleton class="h-4 w-48" /><Skeleton class="h-3 w-24" /></div></div>)}
          </div>
        ) : items.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">🗑️</div>
            <h3 class="text-lg font-semibold text-slate-500 mb-1">回收站为空</h3>
            <p class="text-sm text-slate-400">删除的文件将出现在这里</p>
          </div>
        ) : (
          <>
            {/* Desktop table */}
            <div class="hidden sm:block overflow-x-auto">
              <table class="w-full min-w-[600px]">
                <thead>
                  <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 sticky top-0 bg-slate-50/95 backdrop-blur">
                    <th class="w-10 px-4 py-3">
                      <input type="checkbox" checked={selIds.value.length === items.value.length} onChange$={() => selIds.value = selIds.value.length === items.value.length ? [] : items.value.map(f => f.id)} class="rounded border-slate-300" />
                    </th>
                    <th class="px-2 py-3">文件名</th>
                    <th class="px-2 py-3 w-28">大小</th>
                    <th class="px-2 py-3 w-40">删除时间</th>
                    <th class="w-32 px-2 py-3" />
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-50">
                  {items.value.map(f => {
                    const sel = selIds.value.includes(f.id);
                    return (
                      <tr key={f.id} class={`group text-sm transition-colors ${sel ? "bg-indigo-50/50" : "hover:bg-slate-50"}`}>
                        <td class="px-4 py-3">
                          <input type="checkbox" checked={sel} onChange$={() => {
                            const i = selIds.value.indexOf(f.id);
                            if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id);
                            selIds.value = [...selIds.value];
                          }} class="rounded border-slate-300" />
                        </td>
                        <td class="px-2 py-3">
                          <div class="flex items-center gap-3">
                            <div class="w-9 h-9 rounded-lg bg-slate-100 flex items-center justify-center text-sm">📄</div>
                            <span class="text-slate-700 truncate max-w-xs">{f.filename}</span>
                          </div>
                        </td>
                        <td class="px-2 py-3 text-slate-500">{fmtSize(f.file_size)}</td>
                        <td class="px-2 py-3 text-slate-500 text-xs">{fmtDate(f.deleted_at)}</td>
                        <td class="px-2 py-3">
                          <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <Button variant="ghost" size="sm" onClick$={async () => {
                              try {
                                await api.post(`/files/trash/${f.id}/restore`);
                                items.value = items.value.filter(x => x.id !== f.id);
                              } catch {}
                            }}>恢复</Button>
                            <Button variant="ghost" size="sm" onClick$={async () => {
                              if (!confirm("永久删除此文件？")) return;
                              try { await api.delete(`/files/trash/${f.id}/destroy`); items.value = items.value.filter(x => x.id !== f.id); } catch {}
                            }} class="!text-red-500">删除</Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {/* Mobile card list */}
            <div class="sm:hidden space-y-3 p-4">
              {items.value.map(f => {
                const sel = selIds.value.includes(f.id);
                return (
                  <div key={f.id} class={`bg-white rounded-xl border transition-all p-4 ${sel ? "border-indigo-300 shadow-sm" : "border-slate-200"}`}>
                    <div class="flex items-center gap-3">
                      <input type="checkbox" checked={sel} onChange$={() => {
                        const i = selIds.value.indexOf(f.id);
                        if (i >= 0) selIds.value.splice(i, 1); else selIds.value.push(f.id);
                        selIds.value = [...selIds.value];
                      }} class="rounded border-slate-300 w-5 h-5" />
                      <div class="w-10 h-10 rounded-lg bg-slate-100 flex items-center justify-center text-lg shrink-0">📄</div>
                      <div class="flex-1 min-w-0">
                        <p class="text-sm font-medium text-slate-700 truncate">{f.filename}</p>
                        <p class="text-xs text-slate-400 mt-0.5">{fmtSize(f.file_size)} · {fmtDate(f.deleted_at)}</p>
                      </div>
                    </div>
                    <div class="flex gap-2 mt-3 pt-3 border-t border-slate-100">
                      <button onClick$={async () => {
                        try { await api.post(`/files/trash/${f.id}/restore`); items.value = items.value.filter(x => x.id !== f.id); } catch {}
                      }} class="flex-1 touch-target px-3 py-2 text-xs font-medium text-indigo-600 bg-indigo-50 rounded-lg">恢复</button>
                      <button onClick$={async () => {
                        if (!confirm("永久删除此文件？")) return;
                        try { await api.delete(`/files/trash/${f.id}/destroy`); items.value = items.value.filter(x => x.id !== f.id); } catch {}
                      }} class="flex-1 touch-target px-3 py-2 text-xs font-medium text-red-600 bg-red-50 rounded-lg">永久删除</button>
                    </div>
                  </div>
                );
              })}
            </div>
          </>
        )}
      </div>

      {/* Mobile bottom action bar for batch ops */}
      {selIds.value.length > 0 && (
        <div class="bottom-action-bar sm:hidden">
          <span class="text-sm font-medium text-slate-700">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={async () => {
            loading.value = true;
            try {
              await api.post("/files/trash/batch-restore", { file_ids: selIds.value });
              items.value = items.value.filter(x => !selIds.value.includes(x.id));
              selIds.value = [];
            } catch {}
            loading.value = false;
          }} disabled={loading.value} class="touch-target px-4 py-2 text-xs font-medium text-indigo-600 bg-indigo-50 rounded-lg">恢复</button>
          <button onClick$={async () => {
            if (!confirm("永久删除选中的文件？")) return;
            loading.value = true;
            try {
              for (const id of selIds.value) await api.delete(`/files/trash/${id}/destroy`);
              items.value = items.value.filter(x => !selIds.value.includes(x.id));
              selIds.value = [];
            } catch {}
            loading.value = false;
          }} disabled={loading.value} class="touch-target px-4 py-2 text-xs font-medium text-red-600 bg-red-50 rounded-lg">永久删除</button>
        </div>
      )}
    </div>
  );
});
