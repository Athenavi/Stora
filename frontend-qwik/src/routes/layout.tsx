/**
 * Stora App Layout - enterprise sidebar + topbar
 */
import { component$, useSignal, useVisibleTask$, Slot, $ } from "@builder.io/qwik";
import { useLocation, Link } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import FolderTree from "~/components/ui/FolderTree";
import TransferQueue from "~/components/ui/TransferQueue";
import { api, isAuthenticated } from "~/lib/api";
import { ToastContainer } from "~/components/ui/index";

export default component$(() => {
  const loc = useLocation();
  const path = loc.url.pathname.replace(/\/+$/, "");
  const quota = useSignal<{ max_storage: number; used_storage: number; usage_percent: number } | null>(null);
  const maintenance = useSignal<{ enabled: boolean; message: string; scheduled_start?: string; scheduled_end?: string; time_until_maintenance?: number } | null>(null);

  const isPublic =
    path === "/login" ||
    path === "/register" ||
    path.startsWith("/s");

  // Auth guard - redirect unauthenticated users to /login
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const p = window.location.pathname.replace(/\/+$/, "");
    const pub = p === "/login" || p === "/register" || p.startsWith("/s");
    if (!pub && !isAuthenticated()) {
      window.location.href = "/login";
    }
  });

  // Load quota on mount
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    const p = window.location.pathname.replace(/\/+$/, "");
    const pub = p === "/login" || p === "/register" || p.startsWith("/s");
    if (pub) return;
    try { const q = await api.get<{ max_storage: number; used_storage: number; usage_percent: number }>("/users/me/quota"); quota.value = q; } catch {}
  });

  // Poll maintenance status
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const poll = async () => {
      try {
        const res = await fetch('/api/v2/system/maintenance/public-status');
        const json = await res.json();
        if (json.success && json.data) {
          maintenance.value = json.data;
        }
      } catch {}
    };
    poll();
    const timer = setInterval(poll, 30000); // 30s polling
    return () => clearInterval(timer);
  });

  if (isPublic) {
    // Show maintenance banner on public pages too
    return (
      <>
        {maintenance.value?.enabled && (
          <div class="bg-amber-50 border-b border-amber-200 px-4 py-2 text-center text-sm text-amber-800 flex items-center justify-center gap-2">
            <Icon name="setting" size={16} class="shrink-0" />
            <span>{maintenance.value.message || '系统正在维护中'}</span>
            {maintenance.value.time_until_maintenance !== undefined && maintenance.value.time_until_maintenance > 0 && (
              <span class="text-amber-600 font-mono">
                即将于 {Math.ceil(maintenance.value.time_until_maintenance / 60)} 分钟后开始
              </span>
            )}
          </div>
        )}
        <Slot />
      </>
    );
  }

  const isDrive = path.startsWith("/drive");
  const isShare = path.startsWith("/share");
  const isTrash = path.startsWith("/trash");
  const isFavorites = path.startsWith("/favorites");
  const isVault = path.startsWith("/vault");
  const isTags = path.startsWith("/tags");
  const isPhotos = path.startsWith("/photos");
  const isAdmin = path.startsWith("/admin");
  const darkMode = useSignal(false);
  const mobileMenuOpen = useSignal(false);
  const sidebarOpen = useSignal(true);
  const sidebarHover = useSignal(false);

  // Restore dark mode and sidebar preferences
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const saved = localStorage.getItem("stora-dark-mode");
    if (saved === "true") darkMode.value = true;
    const sidebar = localStorage.getItem("stora-sidebar-collapsed");
    sidebarOpen.value = sidebar !== "true";
  });

  const navItems = [
    { href: "/drive", icon: "folder" as const, label: "我的文件", active: isDrive },
    { href: "/share", icon: "share" as const, label: "我的分享", active: isShare },
    { href: "/trash", icon: "trash" as const, label: "回收站", active: isTrash },
    { href: "/favorites", icon: "star" as const, label: "收藏夹", active: isFavorites },
    { href: "/photos", icon: "image" as const, label: "照片墙", active: isPhotos },
    { href: "/vault", icon: "lock" as const, label: "私密空间", active: isVault },
    { href: "/tags", icon: "tag" as const, label: "标签", active: isTags },
    { href: "/admin", icon: "setting" as const, label: "管理面板", active: isAdmin },
  ];

  const toggleSidebar = $(() => {
    sidebarOpen.value = !sidebarOpen.value;
    localStorage.setItem("stora-sidebar-collapsed", String(!sidebarOpen.value));
  });

  const isCollapsed = !sidebarOpen.value;

  return (
    <div class={`flex min-h-dvh overflow-hidden ${darkMode.value ? 'dark bg-slate-900' : 'bg-slate-50'}`}>
      {/* Mobile overlay */}
      {mobileMenuOpen.value && (
        <div class="fixed inset-0 bg-black/50 z-30 lg:hidden animate-fade-in" onClick$={() => mobileMenuOpen.value = false} />
      )}

      {/* Sidebar */}
      <aside
        onMouseEnter$={() => { if (isCollapsed) sidebarHover.value = true; }}
        onMouseLeave$={() => sidebarHover.value = false}
        class={`${
          sidebarHover.value && isCollapsed ? "w-[260px]" : isCollapsed ? "w-[64px]" : "w-[260px]"
        } ${mobileMenuOpen.value ? "translate-x-0" : "-translate-x-full lg:translate-x-0"}
          transition-all duration-300 bg-slate-900 text-white flex flex-col shrink-0 overflow-hidden noise-bg
          fixed lg:static inset-y-0 left-0 z-40 lg:z-auto`}
      >
        {/* Logo */}
        <div class={`flex items-center h-16 border-b border-slate-800 shrink-0 ${isCollapsed && !sidebarHover.value ? "justify-center px-0" : "gap-3 px-6"}`}>
          <div class="w-8 h-8 rounded-lg bg-indigo-500 flex items-center justify-center text-white text-sm font-bold shrink-0">
            S
          </div>
          {(!isCollapsed || sidebarHover.value) && (
            <div class="overflow-hidden whitespace-nowrap">
              <h1 class="text-base font-semibold tracking-tight text-white">Stora</h1>
              <p class="text-xs text-slate-400 -mt-0.5">Enterprise Storage</p>
            </div>
          )}
        </div>

        <nav class="flex-1 overflow-y-auto scrollbar-thin">
          <div class={`${isCollapsed && !sidebarHover.value ? "px-1.5 py-4" : "px-3 py-4"} space-y-1`}>
          {navItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              onClick$={() => mobileMenuOpen.value = false}
              title={isCollapsed && !sidebarHover.value ? item.label : undefined}
              class={`flex items-center gap-3 rounded-lg text-sm transition-all duration-150 ${
                isCollapsed && !sidebarHover.value ? "px-2.5 py-2.5 justify-center" : "px-3 py-2.5"
              } ${
                item.active
                  ? "bg-indigo-600/20 text-indigo-300 font-medium"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
              }`}
            >
              <Icon name={item.icon} size={20} class="shrink-0" />
              {(!isCollapsed || sidebarHover.value) && <span class="truncate">{item.label}</span>}
            </Link>
          ))}
          </div>
          {/* Folder tree - only shown on drive page when sidebar is not collapsed */}
          {isDrive && (!isCollapsed || sidebarHover.value) && <FolderTree />}
        </nav>

        {/* Bottom user area - collapsed mode shows only avatar */}
        {(!isCollapsed || sidebarHover.value) ? (
          <div class="px-4 py-4 border-t border-slate-800">
            <div class="flex items-center gap-3">
              <div class="w-9 h-9 rounded-full bg-slate-700 flex items-center justify-center text-white text-sm font-medium shrink-0">
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
        ) : (
          <div class="py-4 border-t border-slate-800 flex justify-center">
            <div class="w-8 h-8 rounded-full bg-slate-700 flex items-center justify-center text-white text-xs font-medium">
              U
            </div>
          </div>
        )}
      </aside>

      {/* Main area */}
      <div class="flex-1 flex flex-col min-w-0">
        {/* Topbar */}
        <header class="h-16 bg-white border-b border-slate-200 flex items-center gap-2 sm:gap-4 px-4 sm:px-6 shrink-0">
          <button
            onClick$={() => { if (window.innerWidth < 1024) mobileMenuOpen.value = true; else toggleSidebar(); }}
            class="p-2 -ml-2 rounded-lg text-slate-500 hover:bg-slate-100 hover:text-slate-700 transition-colors"
            aria-label="菜单"
          >
            <Icon name="menu" size={20} />
          </button>
          <div class="flex-1" />
          <button onClick$={() => { darkMode.value = !darkMode.value; localStorage.setItem("stora-dark-mode", String(darkMode.value)); }} class="p-2 rounded-lg text-slate-500 hover:bg-slate-100 transition-colors" title={darkMode.value ? "亮色模式" : "暗色模式"}>
            {darkMode.value ? <Icon name="sun" size={20} /> : <Icon name="moon" size={20} />}
          </button>
          <button class="p-2 rounded-lg text-slate-500 hover:bg-slate-100 transition-colors hidden sm:block" title="设置">
            <Icon name="setting" size={20} />
          </button>
          <div class="w-8 h-8 rounded-full bg-slate-200 flex items-center justify-center text-slate-600 text-sm font-medium shrink-0">
            U
          </div>
        </header>

        {/* Maintenance banner */}
        {maintenance.value?.enabled && (
          <div class="bg-amber-50 border-b border-amber-200 px-3 sm:px-4 py-2 text-center text-sm text-amber-800 flex items-center justify-center gap-2 shrink-0">
            <Icon name="setting" size={16} class="shrink-0" />
            <span class="truncate">{maintenance.value.message || '系统正在维护中'}</span>
            {maintenance.value.time_until_maintenance !== undefined && maintenance.value.time_until_maintenance > 0 && (
              <span class="text-amber-600 font-mono text-xs shrink-0">
                {Math.ceil(maintenance.value.time_until_maintenance / 60)} 分钟后
              </span>
            )}
          </div>
        )}

        {/* Page content */}
        <main class="flex-1 overflow-auto scrollbar-thin">
          <TransferQueue>
            <Slot />
          </TransferQueue>
        </main>

        {/* Mobile FAB — visible on action pages */}
        {(isDrive || isVault || isPhotos) && (
          <button
            onClick$={() => {
              // Dispatch a custom event that pages can listen for
              window.dispatchEvent(new CustomEvent('stora:fab-click'));
            }}
            class="lg:hidden fixed bottom-6 right-6 w-14 h-14 bg-indigo-600 text-white rounded-full shadow-lg
              flex items-center justify-center text-2xl hover:bg-indigo-700 active:scale-95 transition-all z-40 animate-scale-in"
            aria-label="新建"
          >
            <Icon name="plus" size={24} />
          </button>
        )}
      </div>
      <ToastContainer />
    </div>
  );
});
