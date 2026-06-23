/**
 * Stora Photo Wall — flat design timeline grid
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi, batchDownload, api } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import InfiniteScroll from "~/components/ui/InfiniteScroll";

interface Photo {
  id: number;
  filename: string;
  file_size: number;
  width?: number;
  height?: number;
  created_at: string;
  thumbnail_url?: string;
}

interface DayGroup {
  date: string;
  photos: Photo[];
}

export const usePhotos = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  try {
    const data = await srv.get<{ items: Photo[] }>("/files?file_type=image&sort_by=created_at&sort_order=desc&page_size=200");
    const groups: Record<string, Photo[]> = {};
    for (const p of data.items) {
      const d = p.created_at ? p.created_at.split("T")[0] : "未知";
      if (!groups[d]) groups[d] = [];
      groups[d].push(p);
    }
    return Object.entries(groups).map(([date, photos]) => ({ date, photos }));
  } catch {
    return [];
  }
});

function fmtDate(dateStr: string): string {
  const d = new Date(dateStr);
  const weekdays = ["日", "一", "二", "三", "四", "五", "六"];
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 星期${weekdays[d.getDay()]}`;
}

export default component$(() => {
  const groups = usePhotos();
  const nav = useNavigate();
  const lightbox = useSignal<Photo | null>(null);
  const touchStartX = useSignal(0);
  const currentPhotoIndex = useSignal(0);
  const selIds = useSignal<number[]>([]);
  const photoPage = useSignal(1);
  const photoTotal = useSignal(0);
  const photoLoading = useSignal(false);
  const allPhotosAccum = useSignal(groups.value.flatMap(g => g.photos));
  const allGroups = useSignal(groups.value);

  const loadMorePhotos = $(async () => {
    if (photoLoading.value) return;
    photoLoading.value = true;
    try {
      const next = photoPage.value + 1;
      const res = await fetch(`/api/v2/files?file_type=image&sort_by=created_at&sort_order=desc&page=${next}&page_size=200`);
      const json = await res.json();
      const d = json.data || json;
      if (d.items?.length) {
        const newPhotos: Photo[] = d.items;
        allPhotosAccum.value = [...allPhotosAccum.value, ...newPhotos];
        // Re-group by day
        const groupsMap: Record<string, Photo[]> = {};
        for (const p of allPhotosAccum.value) {
          const dt = p.created_at ? p.created_at.split("T")[0] : "未知";
          if (!groupsMap[dt]) groupsMap[dt] = [];
          groupsMap[dt].push(p);
        }
        allGroups.value = Object.entries(groupsMap).map(([date, ph]) => ({ date, photos: ph }));
        photoTotal.value = d.total || 0;
        photoPage.value = next;
      }
    } catch {}
    photoLoading.value = false;
  });

  const toggleSel = (id: number) => {
    const i = selIds.value.indexOf(id);
    if (i >= 0) selIds.value.splice(i, 1);
    else selIds.value.push(id);
    selIds.value = [...selIds.value];
  };

  return (
    <div class="flex flex-col h-full">
      {/* Title area per spec */}
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <h1 class="text-[28px] font-bold text-stora-foreground">照片墙</h1>
        <p class="text-sm text-stora-muted-foreground mt-1">以时间线浏览你的照片回忆</p>
      </div>

      {/* Batch action bar */}
      {selIds.value.length > 0 && (
        <div class="flex items-center gap-2 px-6 py-2 bg-stora-muted border-b border-stora-border shrink-0">
          <span class="text-sm font-medium text-stora-foreground">{selIds.value.length} 项已选</span>
          <div class="flex-1" />
          <button onClick$={() => selIds.value = []} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-muted-foreground hover:bg-stora-muted">取消选择</button>
          <button onClick$={() => batchDownload([...selIds.value])}
            class="touch-target px-3 py-1.5 text-xs font-medium text-stora-primary">下载</button>
          <button onClick$={async () => {
            if (!confirm(`确认删除 ${selIds.value.length} 张照片？`)) return;
            await api.post("/files/batch/delete", { file_ids: [...selIds.value] }).catch(() => {});
            selIds.value = [];
            location.reload();
          }} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-destructive">删除</button>
        </div>
      )}

      <div class={`flex-1 overflow-auto scrollbar-thin p-6 ${selIds.value.length > 0 ? 'pb-20 lg:pb-0' : ''}`}>
        {allGroups.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
            <div class="w-20 h-20 bg-stora-muted flex items-center justify-center text-4xl mb-5">📷</div>
            <h3 class="text-lg font-semibold text-stora-foreground mb-1">暂无照片</h3>
            <p class="text-sm text-stora-muted-foreground mb-6">上传图片文件后，它们将出现在这里</p>
          </div>
        ) : (
          <div class="space-y-8">
            {allGroups.value.map(group => (
              <div key={group.date}>
                <h2 class="text-sm font-medium text-stora-muted-foreground mb-3 sticky top-0 bg-stora-background py-2 z-10">
                  {fmtDate(group.date)} · {group.photos.length} 张
                </h2>
                {/* Photo grid: 220x220px per spec */}
                <div class="flex flex-wrap gap-3">
                  {group.photos.map(photo => {
                    const sel = selIds.value.includes(photo.id);
                    return (
                      <div key={photo.id}
                        onClick$={() => {
                          if (selIds.value.length > 0) { toggleSel(photo.id); }
                          else { lightbox.value = photo; currentPhotoIndex.value = allPhotosAccum.value.findIndex(p => p.id === photo.id); }
                        }}
                        onContextMenu$={(e: any) => { e.preventDefault(); toggleSel(photo.id); }}
                        class={`w-[220px] h-[220px] bg-stora-muted cursor-pointer relative overflow-hidden ${sel ? 'ring-2 ring-stora-primary' : 'hover:ring-2 hover:ring-stora-secondary'}`}>
                        <img
                          src={`/api/v2/files/preview/${photo.id}/thumbnail?size=400`}
                          alt={photo.filename}
                          class="w-full h-full object-cover"
                          loading="lazy"
                        />
                        <div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/60 to-transparent p-2 opacity-0 hover:opacity-100">
                          <p class="text-xs text-white truncate">{photo.filename}</p>
                        </div>
                        {sel && (
                          <div class="absolute top-2 right-2 w-6 h-6 bg-stora-primary flex items-center justify-center">
                            <Icon name="check" size={14} class="text-white" />
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        )}
        <InfiniteScroll
          hasMore={allPhotosAccum.value.length < (photoTotal.value || allPhotosAccum.value.length + 1)}
          loading={photoLoading.value}
          onLoadMore$={loadMorePhotos}
        />
      </div>

      {/* Lightbox */}
      {lightbox.value && (
        <div class="fixed inset-0 z-50 bg-black/95 flex items-center justify-center"
          onClick$={() => lightbox.value = null}
          onTouchStart$={(e: any) => touchStartX.value = e.touches[0].clientX}
          onTouchEnd$={(e: any) => {
            const dx = e.changedTouches[0].clientX - touchStartX.value;
            if (Math.abs(dx) > 60) {
              const dir = dx > 0 ? -1 : 1;
              const newIdx = currentPhotoIndex.value + dir;
              if (newIdx >= 0 && newIdx < allPhotosAccum.value.length) { currentPhotoIndex.value = newIdx; lightbox.value = allPhotosAccum.value[newIdx]; }
            }
          }}>
          <button onClick$={() => lightbox.value = null}
            class="absolute top-4 right-4 w-12 h-12 bg-white/10 text-white flex items-center justify-center hover:bg-white/20 z-10 touch-target"
            aria-label="关闭">
            <Icon name="close" size={24} />
          </button>
          {currentPhotoIndex.value > 0 && (
            <button onClick$={(e: any) => { e.stopPropagation(); currentPhotoIndex.value--; lightbox.value = allPhotosAccum.value[currentPhotoIndex.value]; }}
              class="absolute left-4 top-1/2 -translate-y-1/2 w-12 h-12 bg-white/10 text-white flex items-center justify-center hover:bg-white/20 touch-target"
              aria-label="上一张">
              <Icon name="chevronLeft" size={24} />
            </button>
          )}
          <img src={`/api/v2/files/preview/${lightbox.value.id}/${encodeURIComponent(lightbox.value.filename)}`}
            alt={lightbox.value.filename}
            class="max-w-full max-h-full object-contain px-16"
            onClick$={(e: any) => e.stopPropagation()} />
          {currentPhotoIndex.value < allPhotosAccum.value.length - 1 && (
            <button onClick$={(e: any) => { e.stopPropagation(); currentPhotoIndex.value++; lightbox.value = allPhotosAccum.value[currentPhotoIndex.value]; }}
              class="absolute right-4 top-1/2 -translate-y-1/2 w-12 h-12 bg-white/10 text-white flex items-center justify-center hover:bg-white/20 touch-target"
              aria-label="下一张">
              <Icon name="chevronRight" size={24} />
            </button>
          )}
          <div class="absolute bottom-6 left-1/2 -translate-x-1/2 bg-black/60 text-white text-sm px-4 py-2 max-w-[90vw] truncate">
            {lightbox.value.filename}
            {lightbox.value.width && lightbox.value.height ? ` · ${lightbox.value.width}×${lightbox.value.height}` : ""}
            <span class="hidden sm:inline"> · {currentPhotoIndex.value + 1}/{allPhotosAccum.value.length}</span>
          </div>
        </div>
      )}
    </div>
  );
});
