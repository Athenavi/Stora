/**
 * Stora Tags — flat design tag cloud + file list
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { api, createServerApi } from "~/lib/api";

interface Tag {
  id: number;
  name: string;
  color: string;
  file_count: number;
}

export const useTags = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  return await srv.get<Tag[]>("/files/tags").catch(() => []);
});

const TAG_COLORS = ["#2563EB", "#D97706", "#BE185D", "#059669", "#DC2626", "#7C3AED", "#0891B2"];
const TAG_BG: Record<string, string> = {
  "工作": "#DBEAFE", "设计": "#FEF3C7", "个人": "#FCE7F3", "项目": "#D1FAE5", "重要": "#FEE2E2",
};

export default component$(() => {
  const tags = useTags();
  const nav = useNavigate();
  const items = useSignal(tags.value);
  const showCreate = useSignal(false);
  const editId = useSignal(0);
  const name = useSignal("");
  const color = useSignal(TAG_COLORS[0]);
  const loading = useSignal(false);

  const refresh = async () => { try { items.value = await api.get<Tag[]>("/files/tags"); } catch {} };

  const createOrUpdate = async () => {
    if (!name.value.trim()) return;
    loading.value = true;
    try {
      if (editId.value > 0) { await api.patch(`/files/tags/${editId.value}`, { name: name.value, color: color.value }); }
      else { await api.post("/files/tags", { name: name.value, color: color.value }); }
      showCreate.value = false; editId.value = 0; name.value = ""; await refresh();
    } catch {}
    loading.value = false;
  };

  const startEdit = (tag: Tag) => { editId.value = tag.id; name.value = tag.name; color.value = tag.color; showCreate.value = true; };

  const doDelete = async (id: number) => {
    if (!confirm("确认删除此标签？此操作不可恢复。")) return;
    try { await api.delete(`/files/tags/${id}`); await refresh(); } catch {}
  };

  // Compute tag display color
  const tagStyle = (tag: Tag) => ({
    backgroundColor: TAG_BG[tag.name] || `${tag.color}20`,
    color: tag.color,
  });

  return (
    <div class="flex flex-col h-full">
      {/* Title area per spec */}
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <h1 class="text-[28px] font-bold text-stora-foreground">标签管理</h1>
        <p class="text-sm text-stora-muted-foreground mt-1">通过标签快速组织和查找文件</p>
      </div>

      {/* Create/Edit form */}
      {showCreate.value && (
        <div class="px-6 py-4 border-b border-stora-border bg-stora-muted flex flex-col sm:flex-row items-start sm:items-center gap-3 sm:gap-4">
          <input type="text" bind:value={name} placeholder="标签名称"
            class="w-full sm:w-48 h-10 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-tag"
            onKeyDown$={(e: any) => { if (e.key === "Enter") createOrUpdate(); }} />
          <div class="flex gap-1.5 overflow-x-auto scrollbar-thin pb-1 w-full sm:w-auto">
            {TAG_COLORS.map(c => (
              <button key={c} onClick$={() => color.value = c}
                class={`w-8 h-8 border-2 shrink-0 touch-target ${color.value === c ? "border-stora-foreground scale-110" : "border-transparent"}`}
                style={{ backgroundColor: c }} aria-label={`颜色 ${c}`} />
            ))}
          </div>
          <div class="flex gap-2">
            <button onClick$={createOrUpdate} disabled={loading.value} class="h-9 px-4 text-sm font-medium text-white bg-stora-tag hover:bg-[#047857]">{loading.value ? "..." : (editId.value > 0 ? "更新" : "创建")}</button>
            <button onClick$={() => { showCreate.value = false; editId.value = 0; }} class="h-9 px-4 text-sm font-medium text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-muted">取消</button>
          </div>
        </div>
      )}

      {/* Content */}
      <div class="flex-1 overflow-auto p-6">
        {items.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
            <div class="w-16 h-16 bg-stora-muted flex items-center justify-center text-3xl mb-4">🏷️</div>
            <h3 class="text-lg font-medium text-stora-foreground mb-1">暂无标签</h3>
            <p class="text-sm text-stora-muted-foreground">创建标签后，可在文件右键菜单中为文件添加标签</p>
            <button onClick$={() => { showCreate.value = true; editId.value = 0; name.value = ""; color.value = TAG_COLORS[0]; }}
              class="mt-4 px-4 py-2 bg-stora-tag text-white text-sm font-medium">新建标签</button>
          </div>
        ) : (
          <>
            {/* Tag cloud — pill/capsule per spec: 34px h, radius 17px */}
            <div class="flex flex-wrap gap-2.5 mb-8">
              {items.value.map(tag => (
                <div key={tag.id}
                  class="inline-flex items-center h-[34px] px-[14px] cursor-pointer hover:opacity-80"
                  style={tagStyle(tag)}
                  onClick$={() => startEdit(tag)}
                  onContextMenu$={(e: any) => { e.preventDefault(); if (confirm("删除标签?")) doDelete(tag.id); }}>
                  <span class="text-sm font-medium">{tag.name} {tag.file_count}</span>
                </div>
              ))}
            </div>

            {/* Tag file list — 48px rows per spec */}
            <div class="divide-y divide-stora-border">
              {items.value.map(tag => (
                <div key={tag.id} class="flex items-center gap-4 px-4 h-12 hover:bg-stora-muted">
                  <span class="text-sm">📄</span>
                  <span class="text-sm font-medium text-stora-foreground flex-1 truncate">{tag.name}</span>
                  {/* Tag badge per spec */}
                  <span class="h-[26px] inline-flex items-center px-3 text-xs font-medium" style={tagStyle(tag)}>{tag.name}</span>
                  <span class="text-xs text-stora-nav-text">{tag.file_count} 个文件</span>
                  <div class="flex gap-2">
                    <button onClick$={() => startEdit(tag)} class="text-xs text-stora-muted-foreground hover:text-stora-foreground">编辑</button>
                    <button onClick$={() => doDelete(tag.id)} class="text-xs text-stora-destructive hover:text-red-700">删除</button>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  );
});
