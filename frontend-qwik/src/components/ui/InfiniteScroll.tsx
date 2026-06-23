/**
 * Stora InfiniteScroll — reusable IntersectionObserver load-more component
 * Usage:
 *   <InfiniteScroll
 *     hasMore={items.length < total}
 *     loading={loading}
 *     onLoadMore$={() => fetchNextPage()}
 *   />
 */
import { component$, useVisibleTask$, $, Slot } from "@builder.io/qwik";

interface Props {
  hasMore: boolean;
  loading: boolean;
  onLoadMore$: () => void;
  threshold?: number;
}

export default component$<Props>(({ hasMore, loading, onLoadMore$, threshold = 200 }) => {
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(({ cleanup }) => {
    if (!hasMore || loading) return;
    const sentinel = document.getElementById("infinite-scroll-sentinel");
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          onLoadMore$();
        }
      },
      { rootMargin: `${threshold}px` },
    );
    observer.observe(sentinel);
    cleanup(() => observer.disconnect());
  });

  if (!hasMore) return null;

  return (
    <div id="infinite-scroll-sentinel" class="flex items-center justify-center py-4">
      {loading ? (
        <span class="text-xs text-stora-muted-foreground">加载中...</span>
      ) : (
        <button onClick$={onLoadMore$}
          class="text-xs font-medium text-stora-primary hover:underline">
          加载更多
        </button>
      )}
    </div>
  );
});
