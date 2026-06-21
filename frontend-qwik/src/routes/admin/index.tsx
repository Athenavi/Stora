/**
 * Stora Admin Page — user management, storage plans, audit logs
 * Full implementation with real backend data.
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$ } from "@builder.io/qwik-city";
import { Icon } from "~/components/ui/Icon";
import { Button, Skeleton, Input, Badge } from "~/components/ui/Button";
import { api } from "~/lib/api";

// ─── Types ───

interface DashboardStats {
  total_users: number;
  total_files: number;
  total_shares: number;
  total_storage: number;
  used_storage: number;
  storage_percent: number;
}

interface AdminUser {
  id: number;
  username: string;
  email: string;
  is_active: boolean;
  is_superuser: boolean;
  is_staff: boolean;
  date_joined: string;
  last_login_at: string | null;
  total_storage: number;
  used_storage: number;
}

interface AuditLog {
  id: number;
  user_id: number;
  action: string;
  resource: string | null;
  detail: string | null;
  ip_address: string | null;
  created_at: string;
}

interface AdminSetting {
  key: string;
  value: string;
}

const tabs = [
  { id: "overview", label: "概览", icon: "eye" as const },
  { id: "users", label: "用户管理", icon: "user" as const },
  { id: "audit", label: "审计日志", icon: "search" as const },
  { id: "settings", label: "系统设置", icon: "setting" as const },
];

export const useStats = routeLoader$(async () => {
  try { return await api.get<DashboardStats>("/admin/dashboard"); } catch { return null; }
});

export const useUsers = routeLoader$(async () => {
  try { return await api.get<AdminUser[]>("/admin/users"); } catch { return []; }
});

export const useLogs = routeLoader$(async () => {
  try { return await api.get<{ items: AuditLog[]; total: number }>("/admin/audit-logs?page=1&per_page=50"); } catch { return { items: [], total: 0 }; }
});

export const useSettings = routeLoader$(async () => {
  try { return await api.get<Record<string, string>>("/admin/settings"); } catch { return {} as Record<string, string>; }
});

function fmtSize(bytes: number): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

function fmtDate(s: string | null | undefined): string {
  if (!s) return "-";
  const d = new Date(s);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export default component$(() => {
  const activeTab = useSignal("overview");

  return (
    <div class="flex flex-col h-full">
      <div class="px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">管理面板</h1>
        <p class="text-sm text-slate-500 mt-0.5">系统管理和监控</p>
      </div>

      {/* Tabs — scrollable on mobile */}
      <div class="flex gap-1 px-4 sm:px-6 py-2 border-b border-slate-200 bg-white/80 shrink-0 overflow-x-auto scrollbar-thin">
        {tabs.map(t => (
          <button key={t.id} onClick$={() => activeTab.value = t.id}
            class={`flex items-center gap-2 px-3 sm:px-4 py-2 text-sm font-medium rounded-lg transition-colors whitespace-nowrap touch-target
              ${activeTab.value === t.id ? "bg-indigo-50 text-indigo-700" : "text-slate-500 hover:text-slate-700 hover:bg-slate-50"}`}>
            <Icon name={t.icon} size={16} />
            <span class="hidden xs:inline sm:inline">{t.label}</span>
          </button>
        ))}
      </div>

      <div class="flex-1 overflow-auto p-4 sm:p-6">
        {activeTab.value === "overview" && <OverviewTab />}
        {activeTab.value === "users" && <UsersTab />}
        {activeTab.value === "audit" && <AuditTab />}
        {activeTab.value === "settings" && <SettingsTab />}
      </div>
    </div>
  );
});

// ─── Overview Tab ───

function OverviewTab() {
  const stats = useStats();
  const s = stats.value;

  if (!s) {
    return <div class="flex items-center justify-center h-40 text-slate-400 text-sm">加载统计数据中...</div>;
  }

  return (
    <div class="space-y-6">
      <div class="grid grid-cols-2 md:grid-cols-4 gap-3 sm:gap-6">
        <StatCard label="总用户" value={String(s.total_users)} icon="user" color="bg-indigo-50 text-indigo-600" />
        <StatCard label="总文件" value={String(s.total_files)} icon="folder" color="bg-emerald-50 text-emerald-600" />
        <StatCard label="分享链接" value={String(s.total_shares)} icon="share" color="bg-amber-50 text-amber-600" />
        <StatCard label="存储使用" value={fmtSize(s.used_storage) + " / " + fmtSize(s.total_storage)} icon="setting" color="bg-rose-50 text-rose-600" />
      </div>

      {/* Storage bar */}
      <div class="bg-white rounded-xl border border-slate-200 p-4 sm:p-6">
        <h3 class="text-sm font-medium text-slate-700 mb-3">存储总览</h3>
        <div class="h-3 bg-slate-100 rounded-full overflow-hidden">
          <div class="h-full bg-indigo-500 rounded-full transition-all" style={{ width: `${s.total_storage > 0 ? Math.min((s.used_storage / s.total_storage) * 100, 100) : 0}%` }} />
        </div>
        <div class="flex justify-between mt-2 text-xs text-slate-500">
          <span>已使用 {fmtSize(s.used_storage)}</span>
          <span>总计 {fmtSize(s.total_storage)}</span>
        </div>
      </div>
    </div>
  );
}

function StatCard({ label, value, icon, color }: { label: string; value: string; icon: string; color: string }) {
  return (
    <div class="bg-white rounded-xl border border-slate-200 p-4 sm:p-6">
      <div class="flex items-center gap-3 mb-2 sm:mb-3">
        <div class={`w-8 h-8 sm:w-10 sm:h-10 rounded-xl ${color} flex items-center justify-center`}>
          <Icon name={icon as any} size={16} />
        </div>
      </div>
      <div class="text-lg sm:text-2xl font-bold text-slate-900 truncate">{value}</div>
      <div class="text-xs sm:text-sm text-slate-500 mt-1">{label}</div>
    </div>
  );
}

// ─── Users Tab ───

function UsersTab() {
  const data = useUsers();
  const users = useSignal(data.value);
  const editId = useSignal(0);
  const editActive = useSignal(false);
  const editQuota = useSignal("");
  const saving = useSignal(false);
  const expandedId = useSignal(0);

  const doToggleActive = async (id: number, active: boolean) => {
    saving.value = true;
    try {
      await api.put(`/admin/users/${id}`, { is_active: active });
      users.value = users.value.map(u => u.id === id ? { ...u, is_active: active } : u);
    } catch {}
    saving.value = false;
  };

  const doSetQuota = async (id: number) => {
    const bytes = parseInt(editQuota.value) * 1073741824;
    if (!bytes) return;
    saving.value = true;
    try {
      await api.put(`/admin/users/${id}`, { total_storage: bytes });
      users.value = users.value.map(u => u.id === id ? { ...u, total_storage: bytes } : u);
      editId.value = 0;
    } catch {}
    saving.value = false;
  };

  return (
    <div class="bg-white rounded-xl border border-slate-200 overflow-hidden">
      {users.value.length === 0 ? (
        <div class="p-8 text-center text-slate-400 text-sm">暂无用户数据</div>
      ) : (
        <>
          {/* Desktop table */}
          <div class="hidden sm:block overflow-x-auto">
            <table class="w-full min-w-[700px]">
              <thead>
                <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 bg-slate-50/95">
                  <th class="px-4 py-3">ID</th>
                  <th class="px-4 py-3">用户名</th>
                  <th class="px-4 py-3">邮箱</th>
                  <th class="px-4 py-3">状态</th>
                  <th class="px-4 py-3">角色</th>
                  <th class="px-4 py-3">已用/配额</th>
                  <th class="px-4 py-3">注册时间</th>
                  <th class="px-4 py-3 w-32">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-50">
                {users.value.map(u => (
                  <tr key={u.id} class="text-sm hover:bg-slate-50 transition-colors">
                    <td class="px-4 py-3 text-slate-500">{u.id}</td>
                    <td class="px-4 py-3">
                      <span class="font-medium text-slate-700">{u.username || "—"}</span>
                    </td>
                    <td class="px-4 py-3 text-slate-500">{u.email || "—"}</td>
                    <td class="px-4 py-3">
                      <Badge variant={u.is_active ? "success" : "danger"}>{u.is_active ? "活跃" : "禁用"}</Badge>
                    </td>
                    <td class="px-4 py-3">
                      {u.is_superuser ? <Badge variant="warning">管理员</Badge> : u.is_staff ? <Badge>员工</Badge> : <span class="text-slate-400">用户</span>}
                    </td>
                    <td class="px-4 py-3 text-slate-500 text-xs">
                      <div class="flex items-center gap-2">
                        <span>{fmtSize(u.used_storage)}</span>
                        <span class="text-slate-300">/</span>
                        {editId.value === u.id ? (
                          <div class="flex items-center gap-1">
                            <input type="number" bind:value={editQuota} class="w-20 px-2 py-1 text-xs rounded border border-slate-300" placeholder="GB" />
                            <button onClick$={() => doSetQuota(u.id)} disabled={saving.value} class="text-indigo-600 hover:text-indigo-800 text-xs touch-target">保存</button>
                            <button onClick$={() => editId.value = 0} class="text-slate-400 hover:text-slate-600 text-xs touch-target">取消</button>
                          </div>
                        ) : (
                          <span>{fmtSize(u.total_storage)}</span>
                        )}
                      </div>
                    </td>
                    <td class="px-4 py-3 text-xs text-slate-400">{fmtDate(u.date_joined)}</td>
                    <td class="px-4 py-3">
                      <div class="flex gap-2">
                        <button onClick$={() => doToggleActive(u.id, !u.is_active)} disabled={saving.value}
                          class={`text-xs touch-target px-2 py-1 rounded ${u.is_active ? 'text-red-600 hover:bg-red-50' : 'text-green-600 hover:bg-green-50'}`}>
                          {u.is_active ? '禁用' : '启用'}
                        </button>
                        <button onClick$={() => { editId.value = u.id; editQuota.value = String(u.total_storage / 1073741824); }}
                          class="text-xs text-indigo-600 hover:text-indigo-800 touch-target px-2 py-1 rounded hover:bg-indigo-50">调整配额</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile card list */}
          <div class="sm:hidden space-y-3 p-4">
            {users.value.map(u => (
              <div key={u.id} class="border border-slate-200 rounded-xl overflow-hidden">
                <div class="flex items-center justify-between px-4 py-3 bg-slate-50/50">
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-slate-400">#{u.id}</span>
                    <span class="text-sm font-medium text-slate-700">{u.username || "—"}</span>
                  </div>
                  <Badge variant={u.is_active ? "success" : "danger"}>{u.is_active ? "活跃" : "禁用"}</Badge>
                </div>
                <div class="px-4 py-3 space-y-2 text-sm">
                  <div class="flex justify-between">
                    <span class="text-slate-500">邮箱</span>
                    <span class="text-slate-700">{u.email || "—"}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-slate-500">角色</span>
                    <span>{u.is_superuser ? "管理员" : u.is_staff ? "员工" : "用户"}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-slate-500">存储</span>
                    <span class="text-xs">{fmtSize(u.used_storage)} / {fmtSize(u.total_storage)}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-slate-500">注册</span>
                    <span class="text-xs">{fmtDate(u.date_joined)}</span>
                  </div>
                </div>
                <div class="flex border-t border-slate-100">
                  <button onClick$={() => doToggleActive(u.id, !u.is_active)} disabled={saving.value}
                    class={`flex-1 touch-target py-3 text-xs font-medium text-center ${u.is_active ? 'text-red-600' : 'text-green-600'}`}>
                    {u.is_active ? '禁用' : '启用'}
                  </button>
                  <div class="w-px bg-slate-100" />
                  <button onClick$={() => { editId.value = u.id; editQuota.value = String(u.total_storage / 1073741824); }}
                    class="flex-1 touch-target py-3 text-xs font-medium text-indigo-600 text-center">调整配额</button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// ─── Audit Tab ───

function AuditTab() {
  const data = useLogs();
  const logs = useSignal(data.value.items);

  const actions: Record<string, string> = {
    file_upload: "上传文件",
    file_download: "下载文件",
    file_delete: "删除文件",
    file_share: "分享文件",
    user_login: "用户登录",
    user_logout: "用户登出",
    admin_action: "管理操作",
    share_create: "创建分享",
    share_revoke: "撤销分享",
  };

  return (
    <div class="bg-white rounded-xl border border-slate-200 overflow-hidden">
      {logs.value.length === 0 ? (
        <div class="p-8 text-center text-slate-400 text-sm">暂无审计日志</div>
      ) : (
        <>
          {/* Desktop table */}
          <div class="hidden sm:block overflow-x-auto">
            <table class="w-full min-w-[600px]">
              <thead>
                <tr class="text-left text-xs font-medium text-slate-400 uppercase tracking-wider border-b border-slate-100 bg-slate-50/95">
                  <th class="px-4 py-3">时间</th>
                  <th class="px-4 py-3">用户ID</th>
                  <th class="px-4 py-3">操作</th>
                  <th class="px-4 py-3">资源</th>
                  <th class="px-4 py-3">详情</th>
                  <th class="px-4 py-3">IP</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-50">
                {logs.value.map(l => (
                  <tr key={l.id} class="text-sm hover:bg-slate-50 transition-colors">
                    <td class="px-4 py-3 text-xs text-slate-500 whitespace-nowrap">{fmtDate(l.created_at)}</td>
                    <td class="px-4 py-3 text-slate-500">#{l.user_id}</td>
                    <td class="px-4 py-3">
                      <span class="px-2 py-0.5 rounded-full bg-slate-100 text-slate-700 text-xs">
                        {actions[l.action] || l.action}
                      </span>
                    </td>
                    <td class="px-4 py-3 text-slate-600 text-xs max-w-[150px] truncate">{l.resource || "—"}</td>
                    <td class="px-4 py-3 text-slate-500 text-xs max-w-[200px] truncate">{l.detail || "—"}</td>
                    <td class="px-4 py-3 text-xs text-slate-400 font-mono">{l.ip_address || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile card list */}
          <div class="sm:hidden space-y-2 p-4">
            {logs.value.map(l => (
              <div key={l.id} class="border border-slate-100 rounded-xl p-4 text-sm space-y-2">
                <div class="flex items-center justify-between">
                  <span class="text-xs text-slate-400">{fmtDate(l.created_at)}</span>
                  <span class="text-xs text-slate-500">#{l.user_id}</span>
                </div>
                <div class="flex items-center gap-2">
                  <span class="px-2 py-0.5 rounded-full bg-slate-100 text-slate-700 text-xs">
                    {actions[l.action] || l.action}
                  </span>
                  {l.resource && <span class="text-xs text-slate-500 truncate">{l.resource}</span>}
                </div>
                {l.detail && <p class="text-xs text-slate-500">{l.detail}</p>}
                {l.ip_address && <p class="text-xs text-slate-400 font-mono">{l.ip_address}</p>}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// ─── Settings Tab ───

function SettingsTab() {
  const data = useSettings();
  const settings = useSignal(Object.entries(data.value || {}).map(([key, value]) => ({ key, value })));
  const key = useSignal("");
  const value = useSignal("");
  const saving = useSignal(false);
  const message = useSignal("");

  const doAdd = async () => {
    if (!key.value || !value.value) return;
    saving.value = true;
    message.value = "";
    try {
      await api.put("/admin/settings", { key: key.value, value: value.value });
      settings.value = [...settings.value.filter(s => s.key !== key.value), { key: key.value, value: value.value }];
      key.value = "";
      value.value = "";
      message.value = "设置已保存";
    } catch (e: any) { message.value = e.message || "保存失败"; }
    saving.value = false;
  };

  return (
    <div class="space-y-6">
      <div class="bg-white rounded-xl border border-slate-200 p-4 sm:p-6">
        <h3 class="text-sm font-medium text-slate-700 mb-4">系统设置</h3>
        <div class="space-y-3 mb-4">
          {settings.value.map(s => (
            <div key={s.key} class="flex items-center justify-between py-2 border-b border-slate-50 last:border-0">
              <div class="min-w-0">
                <span class="text-sm font-mono text-slate-700 truncate block">{s.key}</span>
                <p class="text-xs text-slate-400 mt-0.5 truncate">{s.value}</p>
              </div>
            </div>
          ))}
          {settings.value.length === 0 && <p class="text-sm text-slate-400">暂无设置项</p>}
        </div>

        <div class="border-t border-slate-100 pt-4">
          <h4 class="text-sm font-medium text-slate-700 mb-3">添加/更新设置</h4>
          {message.value && <p class="text-xs text-green-600 mb-2">{message.value}</p>}
          <div class="flex flex-col sm:flex-row gap-3 items-stretch sm:items-end">
            <div class="flex-1">
              <Input bind:value={key} placeholder="设置键 (如 site_name)" label="键" />
            </div>
            <div class="flex-1">
              <Input bind:value={value} placeholder="设置值" label="值" />
            </div>
            <div class="sm:pt-6">
              <Button size="sm" onClick$={doAdd} loading={saving.value} class="w-full sm:w-auto">保存</Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
