import { component$ } from "@builder.io/qwik";
import { useLocation, Link } from "@builder.io/qwik-city";

export default component$(() => {
  const location = useLocation();
  const isDrive = location.url.pathname.startsWith("/drive");
  const isShare = location.url.pathname.startsWith("/share");

  return (
    <div class="flex h-screen">
      {/* Sidebar */}
      <aside class="w-64 bg-indigo-950 text-white flex flex-col shrink-0">
        <div class="p-5 border-b border-indigo-800">
          <Link href="/" class="flex items-center gap-2 text-xl font-bold">
            <span class="text-indigo-400">◆</span>
            <span>Stora</span>
          </Link>
        </div>

        <nav class="flex-1 p-3 space-y-1">
          <SidebarLink href="/drive" icon="📁" label="我的文件" active={isDrive} />
          <SidebarLink href="/share" icon="🔗" label="我的分享" active={isShare} />
          <SidebarLink href="/trash" icon="🗑️" label="回收站" />
          <SidebarLink href="/favorites" icon="⭐" label="收藏夹" />
        </nav>

        <div class="p-4 border-t border-indigo-800">
          <div class="text-sm text-indigo-300">存储空间</div>
          <div class="mt-1 h-2 bg-indigo-900 rounded-full overflow-hidden">
            <div class="h-full w-1/3 bg-indigo-500 rounded-full" />
          </div>
          <div class="mt-1 text-xs text-indigo-400">1.2 GB / 10 GB</div>
        </div>
      </aside>

      {/* Main Content */}
      <main class="flex-1 flex flex-col overflow-hidden">
        <slot />
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
      <span>{props.icon}</span>
      <span>{props.label}</span>
    </Link>
  );
}
