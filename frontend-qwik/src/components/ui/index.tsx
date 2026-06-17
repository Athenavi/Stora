/**
 * Stora UI Components — enterprise-grade components
 *
 * Skeleton loaders, lazy image, toast, empty state, FAB
 */
import { component$, Slot, useSignal, useVisibleTask$ } from "@builder.io/qwik";

// ─── Skeleton Loaders ───

export const Skeleton = component$<{ class?: string }>(({ class: cls = "" }) => (
  <div class={`animate-pulse rounded-md bg-slate-200 ${cls}`} />
));

export const TableSkeleton = component$<{ rows?: number }>(({ rows = 5 }) => (
  <div class="space-y-3 p-4">
    {Array.from({ length: rows }).map((_, i) => (
      <div key={i} class="flex items-center gap-4">
        <Skeleton class="w-5 h-5 rounded" />
        <Skeleton class="w-10 h-10 rounded-lg" />
        <div class="flex-1 space-y-2">
          <Skeleton class="h-4 w-3/5" />
          <Skeleton class="h-3 w-1/5" />
        </div>
      </div>
    ))}
  </div>
));

export const CardGridSkeleton = component$<{ count?: number }>(({ count = 6 }) => (
  <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6">
    {Array.from({ length: count }).map((_, i) => (
      <div key={i} class="space-y-3">
        <Skeleton class="aspect-square rounded-xl" />
        <Skeleton class="h-4 w-3/4 mx-auto" />
        <Skeleton class="h-3 w-1/2 mx-auto" />
      </div>
    ))}
  </div>
));

// ─── Lazy Image ───

export const LazyImage = component$<{ src: string; alt: string; class?: string }>(({ src, alt, class: cls = "" }) => {
  const loaded = useSignal(false);
  return (
    <div class={`relative overflow-hidden ${cls}`}>
      {!loaded.value && <div class="absolute inset-0 bg-slate-100 animate-pulse" />}
      <img
        src={src}
        alt={alt}
        loading="lazy"
        onLoad$={() => { loaded.value = true; }}
        class={`w-full h-full object-cover transition-opacity duration-300 ${loaded.value ? "opacity-100" : "opacity-0"}`}
      />
    </div>
  );
});

// ─── Toast Notification ───

let toastId = 0;
const toasts: Array<{ id: number; message: string; type: string }> = [];

export const showToast = (message: string, type: "success" | "error" | "info" = "info") => {
  const id = ++toastId;
  toasts.push({ id, message, type });
  // Auto-remove after 3s
  setTimeout(() => {
    const idx = toasts.findIndex(t => t.id === id);
    if (idx >= 0) toasts.splice(idx, 1);
  }, 3000);
};

export const ToastContainer = component$(() => {
  const items = useSignal<typeof toasts>([]);
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const interval = setInterval(() => { items.value = [...toasts]; }, 200);
    return () => clearInterval(interval);
  });
  const colors = { success: "bg-green-50 border-green-200 text-green-700", error: "bg-red-50 border-red-200 text-red-700", info: "bg-blue-50 border-blue-200 text-blue-700" };
  return (
    <div class="fixed bottom-4 right-4 z-50 space-y-2">
      {items.value.map(t => (
        <div key={t.id} class={`px-4 py-3 rounded-lg border shadow-lg text-sm transition-all ${colors[t.type] || colors.info}`}>
          {t.message}
        </div>
      ))}
    </div>
  );
});

// ─── Empty State ───

export const EmptyState = component$<{ icon: string; title: string; description: string }>(
  ({ icon, title, description }) => (
    <div class="flex flex-col items-center justify-center h-full text-slate-400">
      <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">{icon}</div>
      <h3 class="text-lg font-semibold text-slate-500 mb-1">{title}</h3>
      <p class="text-sm text-slate-400 mb-6">{description}</p>
      <Slot />
    </div>
  )
);

// ─── Floating Action Button (P2.5 mobile) ───

export const FAB = component$<{ onClick$: () => void }>(({ onClick$ }) => (
  <button
    onClick$={onClick$}
    class="lg:hidden fixed bottom-6 right-6 w-14 h-14 bg-indigo-600 text-white rounded-full shadow-lg
      flex items-center justify-center text-2xl hover:bg-indigo-700 active:scale-95 transition-all z-40"
    aria-label="上传"
  >
    +
  </button>
));
