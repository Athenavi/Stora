/**
 * Stora ContextMenu — right-click context menu for files/folders
 * Lightweight: no dependencies, pure positioned dropdown.
 */
import { component$, useSignal, useVisibleTask$, Slot } from "@builder.io/qwik";

export interface MenuItem {
  label: string;
  icon?: string;
  danger?: boolean;
  divider?: boolean;
  onClick$: () => void;
}

interface ContextMenuProps {
  items: MenuItem[];
}

export default component$<ContextMenuProps>(({ items }) => {
  const visible = useSignal(false);
  const pos = useSignal({ x: 0, y: 0 });

  const show = (e: MouseEvent) => {
    e.preventDefault();
    pos.value = { x: e.clientX, y: e.clientY };
    visible.value = true;
  };

  const hide = () => { visible.value = false; };

  return (
    <div onContextMenu$={show}>
      <Slot />
      {visible.value && (
        <>
          <div class="fixed inset-0 z-50" onClick$={hide} onContextMenu$={(e: any) => { e.preventDefault(); hide(); }} />
          <div
            class="fixed z-50 min-w-[180px] bg-white rounded-xl shadow-lg border border-slate-200 py-1.5"
            style={{ left: `${pos.value.x}px`, top: `${pos.value.y}px` }}
          >
            {items.map((item, i) => (
              item.divider ? (
                <div key={i} class="h-px bg-slate-100 my-1" />
              ) : (
                <button
                  key={i}
                  onClick$={() => { hide(); item.onClick$(); }}
                  class={`w-full text-left px-4 py-2 text-sm flex items-center gap-3 transition-colors ${
                    item.danger
                      ? "text-red-600 hover:bg-red-50"
                      : "text-slate-700 hover:bg-slate-50"
                  }`}
                >
                  {item.icon && <span class="w-4 text-center text-base">{item.icon}</span>}
                  <span>{item.label}</span>
                </button>
              )
            ))}
          </div>
        </>
      )}
    </div>
  );
});
