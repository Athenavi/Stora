import { component$, Slot } from "@builder.io/qwik";
import { useLocation, Link } from "@builder.io/qwik-city";

export default component$(() => {
  const loc = useLocation();
  const path = loc.url.pathname;

  // Auth pages: no sidebar
  if (path === "/login" || path === "/register") {
    return <Slot />;
  }

  // Main app: sidebar
  const isDrive = path.startsWith("/drive");
  const isShare = path.startsWith("/share");
  const isTrash = path.startsWith("/trash");
  const isFavorites = path.startsWith("/favorites");

  return (
    <div class="flex h-screen">
      <aside class="w-64 bg-indigo-950 text-white flex flex-col shrink-0">
        <div class="p-5 border-b border-indigo-800">
          <Link href="/drive" class="flex items-center gap-2 text-xl font-bold">
            <span class="text-indigo-400">◆</span>
            <span>Stora</span>
          </Link>
        </div>

        <nav class="flex-1 p-3 space-y-1 overflow-auto">
          <SidebarLink href="/drive" icon="📁" label="我的文件" active={isDrive} />
          <SidebarLink href="/share" icon="🔗" label="我的分享" active={isShare} />
          <SidebarLink href="/trash" icon="🗑️" label="回收站" active={isTrash} />
          <SidebarLink href="/favorites" icon="⭐" label="收藏夹" active={isFavorites} />
        </nav>

        <div class="p-4 border-t border-indigo-800">
          <div class="flex items-center gap-3 text-sm text-indigo-200">
            <div class="w-8 h-8 rounded-full bg-indigo-700 flex items-center justify-center text-white text-xs font-bold">
              U
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-white truncate font-medium">用户</div>
              <div class="text-xs text-indigo-300 truncate">设置</div>
            </div>
          </div>
        </div>
      </aside>

      <main class="flex-1 flex flex-col overflow-hidden bg-slate-50">
        <Slot />
      </main>
    </div>
  );
});

function SidebarLink(props: { href: string; icon: string; label: string; active?: boolean }) {
  return (
    <Link
      href={props.href}
      class={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors ${
        props.active
          ? "bg-indigo-800 text-white font-medium"
          : "text-indigo-200 hover:bg-indigo-900 hover:text-white"
      }`}
    >
      <span class="text-lg">{props.icon}</span>
      <span>{props.label}</span>
    </Link>
  );
}
