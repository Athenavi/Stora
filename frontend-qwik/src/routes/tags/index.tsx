/**
 * Stora Tags Management — create, edit, delete and assign tags
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { api } from "~/lib/api";
import { Button, Input } from "~/components/ui/Button";
import { Icon } from "~/components/ui/Icon";

interface Tag {
  id: number;
  name: string;
  color: string;
  file_count: number;
}

export const useTags = routeLoader$(async () => {
  return await api.get<Tag[]>("/files/tags").catch(() => []);
});

const TAG_COLORS = ["#6366f1", "#ec4899", "#f59e0b", "#10b981", "#06b6d4", "#8b5cf6", "#ef4444", "#14b8a6"];

export default component$(() => {
  const tags = useTags();
  const nav = useNavigate();
  const items = useSignal(tags.value);
  const showCreate = useSignal(false);
  const editId = useSignal(0);
  const name = useSignal("");
  const color = useSignal(TAG_COLORS[0]);
  const loading = useSignal(false);

  const refresh = async () => {
    try { items.value = await api.get<Tag[]>("/files/tags"); } catch {}
  };

  const createOrUpdate = async () => {
    if (!name.value.trim()) return;
    loading.value = true;
    try {
      if (editId.value > 0) {
        await api.patch(`/files/tags/${editId.value}`, { name: name.value, color: color.value });
      } else {
        await api.post("/files/tags", { name: name.value, color: color.value });
      }
      showCreate.value = false;
      editId.value = 0;
      name.value = "";
      await refresh();
    } catch {}
    loading.value = false;
  };

  const startEdit = (tag: Tag) => {
    editId.value = tag.id;
    name.value = tag.name;
    color.value = tag.color;
    showCreate.value = true;
  };

  const doDelete = async (id: number) => {
    if (!confirm("确认删除此标签？此操作不可恢复。")) return;
    try {
      await api.delete(`/files/tags/${id}`);
      await refresh();
    } catch {}
  };

  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-6 py-4 border-b border-slate-200 bg-white">
        <div>
          <h1 class="text-lg font-semibold text-slate-900">标签管理</h1>
          <p class="text-sm text-slate-500 mt-0.5">管理文件标签，快速分类和筛选</p>
        </div>
        <Button variant="primary" size="sm" onClick$={() => { showCreate.value = true; editId.value = 0; name.value = ""; color.value = TAG_COLORS[0]; }}>
          <Icon name="plus" size={16} /> 新建标签
        </Button>
      </div>

      {/* Create/Edit form */}
      {showCreate.value && (
        <div class="px-6 py-4 border-b border-slate-200 bg-slate-50/80 flex items-center gap-4 flex-wrap">
          <input type="text" bind:value={name} placeholder="标签名称"
            class="w-48 px-3 py-2 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            onKeyDown$={(e: any) => { if (e.key === "Enter") createOrUpdate(); }} />
          <div class="flex gap-1.5">
            {TAG_COLORS.map(c => (
              <button key={c} onClick$={() => color.value = c}
                class={`w-6 h-6 rounded-full border-2 transition-all ${color.value === c ? "border-slate-900 scale-110" : "border-transparent"}`}
                style={{ backgroundColor: c }} />
            ))}
          </div>
          <Button size="sm" onClick$={createOrUpdate} loading={loading.value}>
            {editId.value > 0 ? "更新" : "创建"}
          </Button>
          <Button variant="ghost" size="sm" onClick$={() => { showCreate.value = false; editId.value = 0; }}>取消</Button>
        </div>
      )}

      {/* Tag list */}
      <div class="flex-1 overflow-auto p-6">
        {items.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-16 h-16 rounded-2xl bg-slate-100 flex items-center justify-center text-3xl mb-4">🏷️</div>
            <h3 class="text-lg font-medium text-slate-500 mb-1">暂无标签</h3>
            <p class="text-sm">创建标签后，可在文件右键菜单中为文件添加标签</p>
          </div>
        ) : (
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {items.value.map(tag => (
              <div key={tag.id} class="bg-white rounded-xl border border-slate-200 hover:shadow-sm transition-all p-4">
                <div class="flex items-center gap-3">
                  <div class="w-4 h-4 rounded-full shrink-0" style={{ backgroundColor: tag.color }} />
                  <span class="text-sm font-medium text-slate-700 flex-1">{tag.name}</span>
                  <span class="text-xs text-slate-400 bg-slate-100 px-2 py-0.5 rounded-full">{tag.file_count} 文件</span>
                </div>
                <div class="flex gap-2 mt-3 pt-3 border-t border-slate-100">
                  <button onClick$={() => startEdit(tag)} class="text-xs text-slate-500 hover:text-indigo-600 transition-colors">编辑</button>
                  <span class="text-slate-200">|</span>
                  <button onClick$={() => doDelete(tag.id)} class="text-xs text-red-500 hover:text-red-700 transition-colors">删除</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
