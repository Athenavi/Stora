/**
 * Stora Search — file search results page
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { createServerApi, type FileItem } from "~/lib/api";
import { Button, Skeleton } from "~/components/ui/Button";
import { Icon } from "~/components/ui/Icon";

export const useSearch = routeLoader$(async ({ url, request }) => {
  const q = url.searchParams.get("q");
  if (!q) return null;
  const srv = createServerApi(request);
  const qs = new URLSearchParams({ q, page: "1", page_size: "50" });
  return await srv.get<{ items: FileItem[]; total: number; page: number; page_size: number }>("/files/search?" + qs.toString()).catch(() => null);
});

function fmtSize(b: number): string { if (!b) return "0 B"; const k = 1024; const i = Math.floor(Math.log(b) / Math.log(k)); return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i]; }

export default component$(() => {
  const data = useSearch();
  const nav = useNavigate();
  const q = useLocation().url.searchParams.get("q") || "";

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">搜索：{q}</h1>
        <p class="text-sm text-slate-500 mt-0.5">{data.value?.total || 0} 个结果</p>
      </div>
      <div class="flex-1 overflow-auto scrollbar-thin p-6">
        {!data.value ? (
          <div class="text-center text-slate-400 mt-20"><p>输入关键词开始搜索</p></div>
        ) : data.value.items.length === 0 ? (
          <div class="text-center text-slate-400 mt-20"><div class="text-5xl mb-4">🔍</div><p>未找到匹配的文件</p></div>
        ) : (
          <div class="space-y-2">
            {data.value.items.map((f: FileItem) => (
              <div key={f.id} onClick$={() => nav(`/view?id=${f.id}`)}
                class="flex items-center gap-4 px-4 py-3 bg-white rounded-lg border border-slate-200 hover:shadow-sm transition-all cursor-pointer">
                <div class="w-10 h-10 rounded-lg bg-slate-100 flex items-center justify-center text-lg">📄</div>
                <div class="flex-1 min-w-0">
                  <p class="text-sm font-medium text-slate-700 truncate">{f.filename}</p>
                  <p class="text-xs text-slate-400">{fmtSize(f.file_size)} · {f.file_type}</p>
                </div>
                <Icon name="chevronRight" size={16} class="text-slate-300" />
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
