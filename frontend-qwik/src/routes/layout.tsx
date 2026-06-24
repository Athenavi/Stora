/**
 * Stora App Layout — flat design sidebar + topbar
 */
import { component$, useSignal, useVisibleTask$, Slot, $ } from "@builder.io/qwik";
import { useLocation, Link } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import FolderTree from "~/components/ui/FolderTree";
import TransferQueue from "~/components/ui/TransferQueue";
import { api, isAuthenticated, getProfile, logout } from "~/lib/api";
import { ToastContainer } from "~/components/ui/index";

type NavIcon = "folder" | "share" | "trash" | "star" | "image" | "lock" | "tag" | "setting";
type NavColor = "#2563EB" | "#2563EB" | "#DC2626" | "#D97706" | "#3B82F6" | "#7C3AED" | "#059669" | "#64748B";

interface NavItem {
  href: string;
  icon: NavIcon;
  label: string;
  activeColor: NavColor;
}

const navItems: NavItem[] = [
  { href: "/drive", icon: "folder", label: "我的文件", activeColor: "#2563EB" },
  { href: "/share", icon: "share", label: "我的分享", activeColor: "#2563EB" },
  { href: "/favorites", icon: "star", label: "收藏夹", activeColor: "#D97706" },
  { href: "/photos", icon: "image", label: "照片墙", activeColor: "#3B82F6" },
  { href: "/vault", icon: "lock", label: "私密空间", activeColor: "#7C3AED" },
  { href: "/tags", icon: "tag", label: "标签", activeColor: "#059669" },
];

export default component$(() => {
  const loc = useLocation();
  const path = loc.url.pathname.replace(/\/+$/, "");
  const quota = useSignal<{ max_storage: number; used_storage: number; usage_percent: number } | null>(null);
  const quotaCalls = useSignal<number[]>([]);
  const userProfile = useSignal<{ username: string; email: string } | null>(null);
  const maintenance = useSignal<{ enabled: boolean; message: string; scheduled_start?: string; scheduled_end?: string; time_until_maintenance?: number } | null>(null);

  const isPublic =
    path === "/login" ||
    path === "/register" ||
    path.startsWith("/s/");

  // Full-screen routes that skip sidebar/topbar but keep auth
  const isFullscreen = path.startsWith("/view");

  // Auth guard
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const p = window.location.pathname.replace(/\/+$/, "");
    const pub = p === "/login" || p === "/register" || p.startsWith("/s/");
    if (!pub && !isAuthenticated()) {
      window.location.href = "/login";
    }
  });

  // Load quota and profile
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    const p = window.location.pathname.replace(/\/+$/, "");
    const pub = p === "/login" || p === "/register" || p.startsWith("/s/");
    if (pub) return;
    try {
      const [q, u] = await Promise.all([
        api.get<{ max_storage: number; used_storage: number; usage_percent: number }>("/users/me/quota"),
        getProfile(),
      ]);
      quota.value = q;
      userProfile.value = u;
    } catch {}
  });

  // Poll maintenance
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
    const timer = setInterval(poll, 30000);
    return () => clearInterval(timer);
  });

  if (isPublic) {
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

  // Full-screen preview: no sidebar/topbar, full viewport
  if (isFullscreen) {
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
        <div class="flex min-h-dvh">
          <Slot />
        </div>
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
  const isSettings = path.startsWith("/settings");
  const mobileMenuOpen = useSignal(false);
  const userMenuOpen = useSignal(false);

  const toggleSidebar = $(() => {
    mobileMenuOpen.value = !mobileMenuOpen.value;
  });

  return (
    <div class="flex min-h-dvh bg-stora-background">
      {/* Mobile overlay */}
      {mobileMenuOpen.value && (
        <div class="fixed inset-0 bg-black/50 z-30 lg:hidden" onClick$={() => mobileMenuOpen.value = false} />
      )}

      {/* Sidebar — 260px fixed */}
      <aside
        class={`${mobileMenuOpen.value ? "translate-x-0" : "-translate-x-full lg:translate-x-0"}
          w-[260px] transition-transform duration-200 bg-stora-sidebar text-white flex flex-col shrink-0
          fixed lg:sticky lg:top-0 lg:h-dvh inset-y-0 left-0 z-40 lg:z-auto`}
      >
        {/* Logo — 48px height, padding 24px */}
        <div class="flex items-center gap-3 px-6 h-12 shrink-0 mt-6">
          <div class="w-8 h-8 rounded-lg bg-stora-primary flex items-center justify-center text-white text-lg font-bold shrink-0">
            S
          </div>
          <h1 class="text-xl font-bold text-white">Stora</h1>
        </div>

        {/* Divider */}
        <div class="mx-6 my-0 h-px bg-stora-nav-divider" />

        {/* Navigation — padding 8px, gap 2px */}
        <nav class="flex-1 overflow-y-auto px-2 py-2 space-y-[2px]">
          {navItems.map((item) => {
            const isActive =
              (item.href === "/drive" && isDrive) ||
              (item.href === "/share" && isShare) ||
              (item.href === "/trash" && isTrash) ||
              (item.href === "/favorites" && isFavorites) ||
              (item.href === "/vault" && isVault) ||
              (item.href === "/tags" && isTags) ||
              (item.href === "/photos" && isPhotos) ||
              (item.href === "/admin" && isAdmin);
            return (
              <Link
                key={item.href}
                href={item.href}
                onClick$={() => mobileMenuOpen.value = false}
                class={`flex items-center gap-3 rounded-lg text-sm h-10 px-3 transition-colors ${
                  isActive
                    ? "text-white font-medium"
                    : "text-stora-nav-text hover:bg-stora-nav-hover hover:text-slate-200"
                }`}
                style={isActive ? { backgroundColor: item.activeColor } : {}}
              >
                <Icon name={item.icon} size={20} class="shrink-0" />
                <span class="truncate">{item.label}</span>
              </Link>
            );
          })}
          {/* Folder tree — shown only on drive page */}
          {isDrive && <FolderTree />}
        </nav>

        {/* Storage area — 75px, #1E293B card, padding 12px, margin 24px */}
        <div class="mx-6 mb-2 bg-stora-storage-bg rounded-lg p-3">
          <div class="flex items-center justify-between mb-1.5">
            <p class="text-xs font-medium text-stora-nav-text">存储空间</p>
            <button onClick$={async () => {
              const now = Date.now();
              const calls = quotaCalls.value.filter((t: number) => now - t < 1800000);
              if (calls.length >= 6) { return; }
              quotaCalls.value = [...calls, now];
              try { quota.value = await api.post<{ max_storage: number; used_storage: number; usage_percent: number }>("/users/me/quota/recalculate"); } catch {}
            }} class="text-stora-nav-text hover:text-white transition-colors text-xs p-0.5 touch-target"
              title="刷新存储空间 (30分钟内限6次)">
              <span>↻</span>
            </button>
          </div>
          <div class="h-1.5 bg-stora-storage-track rounded-sm overflow-hidden mb-1.5">
            <div class="h-full bg-stora-primary rounded-sm transition-all" style={{ width: `${Math.min(quota.value?.usage_percent || 0, 100)}%` }} />
          </div>
          <p class="text-xs text-stora-muted-foreground">
            {quota.value
              ? `${(quota.value.used_storage / 1073741824).toFixed(1)} GB / ${(quota.value.max_storage / 1073741824).toFixed(1)} GB`
              : '— GB / — GB'}
          </p>
        </div>

        {/* User area — clickable dropdown */}
        <div class="relative">
          <div class="flex items-center gap-3 px-6 h-[52px] mb-6 cursor-pointer hover:bg-stora-nav-hover transition-colors" onClick$={() => userMenuOpen.value = !userMenuOpen.value}>
            <div class="w-9 h-9 rounded-full bg-stora-accent flex items-center justify-center text-white text-sm font-bold shrink-0">
              {userProfile.value ? userProfile.value.username.charAt(0).toUpperCase() : "?"}
            </div>
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium text-stora-muted truncate">{userProfile.value?.username || "用户"}</p>
              <p class="text-xs text-stora-muted-foreground truncate">{userProfile.value?.email || "加载中..."}</p>
            </div>
            <Icon name="chevronUp" size={14} class={`text-stora-nav-text transition-transform ${userMenuOpen.value ? "" : "rotate-180"}`} />
          </div>

          {/* Dropdown menu — appears above user area */}
          {userMenuOpen.value && (
            <>
              <div class="fixed inset-0 z-30" onClick$={() => userMenuOpen.value = false} />
              <div class="absolute bottom-full left-3 right-3 mb-2 z-40 bg-stora-sidebar border border-stora-nav-divider rounded-lg shadow-xl overflow-hidden">
                <Link href="/settings" onClick$={() => userMenuOpen.value = false}
                  class="flex items-center gap-3 px-4 py-3 text-sm text-stora-nav-text hover:text-white hover:bg-stora-nav-hover transition-colors">
                  <Icon name="setting" size={18} />
                  账号设置
                </Link>
                <div class="mx-3 h-px bg-stora-nav-divider" />
                <Link href="/trash" onClick$={() => userMenuOpen.value = false}
                  class="flex items-center gap-3 px-4 py-3 text-sm text-stora-nav-text hover:text-white hover:bg-stora-nav-hover transition-colors">
                  <Icon name="trash" size={18} />
                  回收站
                </Link>
                <div class="mx-3 h-px bg-stora-nav-divider" />
                <button onClick$={async () => { userMenuOpen.value = false; await logout(); location.href = "/login"; }}
                  class="flex items-center gap-3 w-full px-4 py-3 text-sm text-red-400 hover:text-red-300 hover:bg-stora-nav-hover transition-colors">
                  <Icon name="close" size={18} />
                  退出登录
                </button>
              </div>
            </>
          )}
        </div>
      </aside>

      {/* Main area */}
      <div class="flex-1 flex flex-col min-w-0">
        {/* Topbar — 66px, padding 24px, flat */}
        <header class="h-[66px] flex items-center gap-4 px-6 shrink-0 border-b border-stora-border bg-stora-card">
          <button
            onClick$={toggleSidebar}
            class="p-2 -ml-2 rounded-lg text-stora-muted-foreground hover:bg-stora-muted transition-colors touch-target lg:hidden"
            aria-label="菜单"
          >
            <Icon name="menu" size={20} />
          </button>
          {/* Breadcrumb placeholder — pages override this area */}
          <div class="flex-1" />
        </header>

        {/* Maintenance banner */}
        {maintenance.value?.enabled && (
          <div class="bg-amber-50 border-b border-amber-200 px-6 py-2 text-center text-sm text-amber-800 flex items-center justify-center gap-2 shrink-0">
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
        <main class="flex-1 overflow-auto">
          <TransferQueue>
            <Slot />
          </TransferQueue>
        </main>
      </div>
      <ToastContainer />
    </div>
  );
});
