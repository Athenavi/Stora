/**
 * Stora App Layout — enterprise sidebar + topbar
 */
import { component$, useSignal, useVisibleTask$, Slot } from "@builder.io/qwik";
import { useLocation, Link } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { api, isAuthenticated } from "~/lib/api";

export default component$(() => {
  const loc = useLocation();
  const path = loc.url.pathname;
  const quota = useSignal<{ max_storage: number; used_storage: number; usage_percent: number } | null>(null);

  const isPublic =
    path === "/login" ||
    path === "/register" ||
    path.startsWith("/s/");

  // Auth guard — redirect unauthenticated users to /login
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const p = loc.url.pathname;
    const pub = p === "/login" || p === "/register" || p.startsWith("/s/");
    if (!pub && !isAuthenticated()) {
      window.location.href = "/login";
    }
  });

  // Load quota on mount
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    const p = loc.url.pathname;
    const pub = p === "/login" || p === "/register" || p.startsWith("/s/");
    if (pub) return;
    try { const q = await api.get<{ max_storage: number; used_storage: number; usage_percent: number }>("/users/me/quota"); quota.value = q; } catch {}
  });

  if (isPublic) {
    return <Slot />;
  }

  const isDrive = path.startsWith("/drive");
  const isShare = path.startsWith("/share");
  const isTrash = path.startsWith("/trash");
  const isFavorites = path.startsWith("/favorites");
  const isAdmin = path.startsWith("/admin");
  const darkMode = useSignal(false);
  const mobileMenuOpen = useSignal(false);
  const sidebarOpen = useSignal(true);

  const navItems = [
    { href: "/drive", icon: "folder" as const, label: "我的文件", active: isDrive },
    { href: "/share", icon: "share" as const, label: "我的分享", active: isShare },
    { href: "/trash", icon: "trash" as const, label: "回收站", active: isTrash },
    { href: "/favorites", icon: "star" as const, label: "收藏夹", active: isFavorites },
    { href: "/admin", icon: "setting" as const, label: "管理面板", active: isAdmin },
  ];

  return (
    <div class={`flex h-screen overflow-hidden ${darkMode.value ? 'dark bg-slate-900' : 'bg-slate-50'}`}>
      {/* Mobile overlay */}
      {mobileMenuOpen.value && (
        <div class="fixed inset-0 bg-black/50 z-30 lg:hidden" onClick$={() => mobileMenuOpen.value = false} />
      )}

      {/* Sidebar */}
      <aside
        class={`${
          sidebarOpen.value ? "w-[260px]" : "w-0"
        } ${mobileMenuOpen.value ? "translate-x-0" : "-translate-x-full lg:translate-x-0"}
          transition-all duration-300 bg-slate-900 text-white flex flex-col shrink-0 overflow-hidden
          fixed lg:static inset-y-0 left-0 z-40 lg:z-auto`}
      >
        <div class="flex items-center gap-3 px-6 h-16 border-b border-slate-800 shrink-0">
          <div class="w-8 h-8 rounded-lg bg-indigo-500 flex items-center justify-center text-white text-sm font-bold">
            S
          </div>
          <div>
            <h1 class="text-base font-semibold tracking-tight">Stora</h1>
            <p class="text-xs text-slate-400 -mt-0.5">Enterprise Storage</p>
          </div>
        </div>

        <nav class="flex-1 px-3 py-4 space-y-1 overflow-y-auto scrollbar-thin">
          {navItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              class={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-all duration-150 ${
                item.active
                  ? "bg-indigo-600/20 text-indigo-300 font-medium"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
              }`}
            >
              <Icon name={item.icon} size={18} class="shrink-0" />
              <span>{item.label}</span>
            </Link>
          ))}
        </nav>

        <div class="px-4 py-4 border-t border-slate-800">
          <div class="flex items-center gap-3">
            <div class="w-9 h-9 rounded-full bg-slate-700 flex items-center justify-center text-white text-sm font-medium">
              U
            </div>
            <div class="flex-1 min-w-0">
              <p class="text-sm text-slate-200 truncate font-medium">用户</p>
              <p class="text-xs text-slate-500 truncate">个人存储</p>
            </div>
          </div>
          {/* Storage bar */}
          <div class="mt-3">
            <div class="flex justify-between text-xs text-slate-500 mb-1">
              <span>存储空间</span>
              <span>{quota.value ? `${(quota.value.used_storage / 1073741824).toFixed(1)} GB / ${(quota.value.max_storage / 1073741824).toFixed(1)} GB` : '加载中...'}</span>
            </div>
            <div class="h-1.5 bg-slate-800 rounded-full overflow-hidden">
              <div class="h-full bg-indigo-500 rounded-full transition-all" style={{ width: `${Math.min(quota.value?.usage_percent || 0, 100)}%` }} />
            </div>
          </div>
        </div>
      </aside>

      {/* Main area */}
      <div class="flex-1 flex flex-col min-w-0">
        {/* Topbar */}
        <header class="h-16 bg-white border-b border-slate-200 flex items-center gap-4 px-6 shrink-0">
          <button
            onClick$={() => { if (window.innerWidth < 1024) mobileMenuOpen.value = true; else sidebarOpen.value = !sidebarOpen.value; }}
            class="p-2 -ml-2 rounded-lg text-slate-500 hover:bg-slate-100 hover:text-slate-700 transition-colors lg:hidden"
          >
            <Icon name="menu" size={20} />
          </button>
          <button
            onClick$={() => sidebarOpen.value = !sidebarOpen.value}
            class="p-2 -ml-2 rounded-lg text-slate-500 hover:bg-slate-100 hover:text-slate-700 transition-colors hidden lg:block"
          >
            <Icon name="menu" size={20} />
          </button>
          <div class="flex-1" />
          <button onClick$={() => darkMode.value = !darkMode.value} class="p-2 rounded-lg text-slate-500 hover:bg-slate-100 transition-colors" title={darkMode.value ? "亮色模式" : "暗色模式"}>
            {darkMode.value ? <Icon name="eye" size={20} /> : <Icon name="eye" size={20} />}
          </button>
          <button class="p-2 rounded-lg text-slate-500 hover:bg-slate-100 transition-colors" title="设置">
            <Icon name="setting" size={20} />
          </button>
          <div class="w-8 h-8 rounded-full bg-slate-200 flex items-center justify-center text-slate-600 text-sm font-medium">
            U
          </div>
        </header>

        {/* Page content */}
        <main class="flex-1 overflow-auto scrollbar-thin">
          <Slot />
        </main>
      </div>
    </div>
  );
});
