/**
 * Stora Favorites — bookmarked files
 */
import { component$ } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { listFiles, type FileItem } from "~/lib/api";
import { Button, Skeleton } from "~/components/ui/Button";

export const useFavList = routeLoader$(async () => {
  return await listFiles({ page: 1, page_size: 50 }).catch(() => null);
});

function fmtSize(b: number): string {
  if (!b) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

export default component$(() => {
  const data = useFavList();
  const files = data.value?.items?.filter(f => f.is_favorite) || [];

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">收藏夹</h1>
        <p class="text-sm text-slate-500 mt-0.5">{files.length} 个收藏文件</p>
      </div>
      <div class="flex-1 overflow-auto scrollbar-thin">
        {!data.value ? (
          <div class="p-6 space-y-3">{[1,2,3].map(i => <div key={i} class="flex items-center gap-4 px-4 py-3"><Skeleton class="w-10 h-10 rounded-lg" /><div class="flex-1 space-y-2"><Skeleton class="h-4 w-48" /><Skeleton class="h-3 w-24" /></div></div>)}</div>
        ) : files.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="text-6xl mb-4">⭐</div>
            <h3 class="text-lg font-medium text-slate-500">暂无收藏</h3>
            <p class="text-sm mt-1">在文件列表中点击星标收藏文件</p>
          </div>
        ) : (
          <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6">
            {files.map(f => (
              <div key={f.id} class="bg-white rounded-xl border border-slate-200 hover:shadow-sm transition-all p-3">
                <div class="aspect-square rounded-lg bg-amber-50 flex items-center justify-center text-4xl mb-2">⭐</div>
                <p class="text-xs font-medium text-slate-700 truncate text-center">{f.filename}</p>
                <p class="text-xs text-slate-400 text-center mt-0.5">{fmtSize(f.file_size)}</p>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
