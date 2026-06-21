/**
 * Stora Photo Wall — timeline-based photo album view
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$, useNavigate } from "@builder.io/qwik-city";
import { createServerApi } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";

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
    // Group by date
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
  // Flatten all photos for swipe navigation
  const allPhotos = groups.value.flatMap(g => g.photos);

  return (
    <div class="flex flex-col h-full">
      <div class="px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">照片墙</h1>
        <p class="text-sm text-slate-500 mt-0.5">{allPhotos.length} 张照片</p>
      </div>

      <div class="flex-1 overflow-auto scrollbar-thin p-4 sm:p-6">
        {groups.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">📷</div>
            <h3 class="text-lg font-semibold text-slate-500 mb-1">暂无照片</h3>
            <p class="text-sm text-slate-400 mb-6">上传图片文件后，它们将出现在这里</p>
          </div>
        ) : (
          <div class="space-y-8">
            {groups.value.map(group => (
              <div key={group.date}>
                <h2 class="text-sm font-medium text-slate-700 mb-3 sticky top-0 bg-slate-50/95 backdrop-blur py-2 z-10">
                  {fmtDate(group.date)} · {group.photos.length} 张
                </h2>
                <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
                  {group.photos.map(photo => (
                    <div key={photo.id}
                      onClick$={() => {
                        lightbox.value = photo;
                        currentPhotoIndex.value = allPhotos.findIndex(p => p.id === photo.id);
                      }}
                      class="aspect-square rounded-xl overflow-hidden bg-slate-100 cursor-pointer hover:ring-2 hover:ring-indigo-400 transition-all group relative">
                      <img
                        src={`/api/v2/files/preview/${photo.id}/thumbnail?size=400`}
                        alt={photo.filename}
                        class="w-full h-full object-cover"
                        loading="lazy"
                      />
                      <div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/60 to-transparent p-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <p class="text-xs text-white truncate">{photo.filename}</p>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Enhanced Lightbox with swipe support */}
      {lightbox.value && (
        <div class="fixed inset-0 z-50 bg-black/95 flex items-center justify-center"
          onClick$={() => lightbox.value = null}
          onTouchStart$={(e: any) => touchStartX.value = e.touches[0].clientX}
          onTouchEnd$={(e: any) => {
            const dx = e.changedTouches[0].clientX - touchStartX.value;
            if (Math.abs(dx) > 60) {
              const dir = dx > 0 ? -1 : 1;
              const newIdx = currentPhotoIndex.value + dir;
              if (newIdx >= 0 && newIdx < allPhotos.length) {
                currentPhotoIndex.value = newIdx;
                lightbox.value = allPhotos[newIdx];
              }
            }
          }}>
          {/* Close button - touch friendly */}
          <button onClick$={() => lightbox.value = null}
            class="absolute top-4 right-4 w-12 h-12 rounded-full bg-white/10 text-white flex items-center justify-center hover:bg-white/20 transition-colors z-10 touch-target"
            aria-label="关闭">
            <Icon name="close" size={24} />
          </button>

          {/* Previous - desktop hover, always visible on mobile */}
          {currentPhotoIndex.value > 0 && (
            <button onClick$={(e: any) => { e.stopPropagation(); currentPhotoIndex.value--; lightbox.value = allPhotos[currentPhotoIndex.value]; }}
              class="absolute left-2 sm:left-4 top-1/2 -translate-y-1/2 w-12 h-12 rounded-full bg-white/10 text-white flex items-center justify-center hover:bg-white/20 transition-colors touch-target"
              aria-label="上一张">
              <Icon name="chevronLeft" size={24} />
            </button>
          )}

          <img
            src={`/api/v2/files/preview/${lightbox.value.id}/${encodeURIComponent(lightbox.value.filename)}`}
            alt={lightbox.value.filename}
            class="max-w-full max-h-full object-contain rounded-lg px-12 sm:px-16"
            onClick$={(e: any) => e.stopPropagation()}
          />

          {/* Next */}
          {currentPhotoIndex.value < allPhotos.length - 1 && (
            <button onClick$={(e: any) => { e.stopPropagation(); currentPhotoIndex.value++; lightbox.value = allPhotos[currentPhotoIndex.value]; }}
              class="absolute right-2 sm:right-4 top-1/2 -translate-y-1/2 w-12 h-12 rounded-full bg-white/10 text-white flex items-center justify-center hover:bg-white/20 transition-colors touch-target"
              aria-label="下一张">
              <Icon name="chevronRight" size={24} />
            </button>
          )}

          {/* Bottom info bar */}
          <div class="absolute bottom-4 sm:bottom-6 left-1/2 -translate-x-1/2 bg-black/60 text-white text-xs sm:text-sm px-4 py-2 rounded-full whitespace-nowrap max-w-[90vw] truncate">
            {lightbox.value.filename}
            {lightbox.value.width && lightbox.value.height ? ` · ${lightbox.value.width}×${lightbox.value.height}` : ""}
            <span class="hidden sm:inline"> · {currentPhotoIndex.value + 1}/{allPhotos.length}</span>
          </div>
        </div>
      )}
    </div>
  );
});
