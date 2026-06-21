/**
 * Stora Vault — 私密空间管理
 * 独立于主文件系统的加密存储区域
 */
import { component$, useSignal, useVisibleTask$ } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { api, createServerApi } from "~/lib/api";
import { Button } from "~/components/ui/Button";
import { Icon } from "~/components/ui/Icon";

interface VaultInfo {
  id: number; name: string; file_count: number; total_size: number;
  lock_timeout: number; created_at: string;
}
interface VaultItem { id: number; filename: string; file_size: number; mime_type: string; created_at: string; }

export const useVaults = routeLoader$(async ({ request }) => {
  const srv = createServerApi(request);
  try { return await srv.get<VaultInfo[]>("/vaults"); } catch { return []; }
});

function fmtSize(b: number): string {
  if (!b) return "0 B"; const k = 1024, i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

export default component$(() => {
  const nav = useNavigate();
  const loc = useLocation();
  const vaults = useVaults();
  const vaultToken = useSignal<string | null>(null);
  const unlockId = useSignal(0);
  const unlockPw = useSignal("");
  const unlockErr = useSignal("");
  const items = useSignal<VaultItem[]>([]);
  const showCreate = useSignal(false);
  const newName = useSignal("");
  const newPw = useSignal("");
  const newPw2 = useSignal("");
  const createErr = useSignal("");
  const loading = useSignal(false);

  // Check URL for existing vault token (from unlock flow)
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const t = loc.url.searchParams.get("vault_token");
    const id = loc.url.searchParams.get("vault_id");
    if (t && id) {
      vaultToken.value = t;
      unlockId.value = Number(id);
      loadItems(Number(id), t);
    }
  });

  const loadItems = async (id: number, token: string) => {
    try {
      const data = await api.get<VaultItem[]>(`/vaults/${id}/items`, { headers: { "X-Vault-Token": token } });
      items.value = data || [];
    } catch { items.value = []; }
  };

  const doUnlock = async () => {
    unlockErr.value = "";
    const fd = new FormData(); fd.append("password", unlockPw.value);
    try {
      const res: any = await api.post(`/vaults/${unlockId.value}/verify-password`, fd);
      vaultToken.value = res.token;
      await loadItems(unlockId.value, res.token);
    } catch (e: any) { unlockErr.value = e.message || "密码错误"; }
  };

  const doLock = () => {
    vaultToken.value = null; items.value = []; unlockPw.value = "";
  };

  const doCreate = async () => {
    if (!newName.value || !newPw.value) { createErr.value = "请填写名称和密码"; return; }
    if (newPw.value !== newPw2.value) { createErr.value = "两次密码不一致"; return; }
    createErr.value = ""; loading.value = true;
    const fd = new FormData(); fd.append("name", newName.value); fd.append("password", newPw.value);
    try {
      const v: any = await api.post("/vaults", fd);
      showCreate.value = false; newName.value = ""; newPw.value = ""; newPw2.value = "";
      // Reload vault list
      const data = await api.get<VaultInfo[]>("/vaults");
      vaults.value = data || [];
    } catch (e: any) { createErr.value = e.message || "创建失败"; }
    loading.value = false;
  };

  const doDeleteVault = async (id: number) => {
    if (!confirm("确认删除此私密空间？所有加密文件将永久丢失！")) return;
    try { await api.delete(`/vaults/${id}`); vaults.value = vaults.value.filter(v => v.id !== id); } catch {}
  };

  const doUpload = async () => {
    const input = document.createElement("input"); input.type = "file"; input.multiple = true;
    input.onchange = async () => {
      if (!input.files?.length) return;
      for (const file of Array.from(input.files)) {
        const reader = new FileReader();
        reader.onload = async () => {
          const base64 = (reader.result as string).split(",")[1];
          const fd = new FormData();
          fd.append("filename", file.name); fd.append("file_size", String(file.size));
          fd.append("mime_type", file.type || "application/octet-stream");
          fd.append("file_content", base64);
          try {
            await api.post(`/vaults/${unlockId.value}/items/upload`, fd, {
              headers: { "X-Vault-Token": vaultToken.value! }
            });
            await loadItems(unlockId.value, vaultToken.value!);
          } catch {}
        };
        reader.readAsDataURL(file);
      }
    };
    input.click();
  };

  const doDownload = async (itemId: number) => {
    window.open(`/api/v2/vaults/${unlockId.value}/items/${itemId}`, "_blank");
  };

  const doDeleteItem = async (itemId: number) => {
    if (!confirm("确认删除？")) return;
    try {
      // Use raw fetch with vault token
      await fetch(`/api/v2/vaults/${unlockId.value}/items/${itemId}`, {
        method: "DELETE", headers: { "X-Vault-Token": vaultToken.value! }
      });
      items.value = items.value.filter(i => i.id !== itemId);
    } catch {}
  };

  // Locked vault display — show list + unlock prompt
  if (unlockId.value > 0 && !vaultToken.value) {
    const vault = vaults.value.find(v => v.id === unlockId.value);
    return (
      <div class="flex flex-col items-center justify-center h-full gap-6 p-8 text-center">
        <div class="w-20 h-20 rounded-2xl bg-indigo-50 flex items-center justify-center text-4xl">🔒</div>
        <h2 class="text-xl font-semibold text-slate-900">{vault?.name || "私密空间"}</h2>
        <p class="text-sm text-slate-500">请输入密码解锁</p>
        {unlockErr.value && <p class="text-sm text-red-600">{unlockErr.value}</p>}
        <input type="password" bind:value={unlockPw}
          onKeyDown$={(e: any) => { if (e.key === "Enter") doUnlock(); }}
          class="w-full max-w-xs sm:w-72 px-4 py-3 rounded-xl border border-slate-300 text-sm text-center focus:outline-none focus:ring-2 focus:ring-indigo-500"
          placeholder="输入私密空间密码" />
        <div class="flex gap-3">
          <Button onClick$={doUnlock}>解锁</Button>
          <Button variant="ghost" onClick$={() => { unlockId.value = 0; nav("/vault"); }}>返回</Button>
        </div>
      </div>
    );
  }

  // Unlocked vault file view
  if (unlockId.value > 0 && vaultToken.value) {
    const vault = vaults.value.find(v => v.id === unlockId.value);
    return (
      <div class="flex flex-col h-full">
        <div class="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
          <div class="flex items-center gap-3 min-w-0">
            <button onClick$={() => { doLock(); nav("/vault"); }} class="text-slate-400 hover:text-slate-600 touch-target p-1">
              <Icon name="chevronLeft" size={20} />
            </button>
            <h1 class="text-lg font-semibold text-slate-900 truncate">{vault?.name || "私密空间"}</h1>
            <span class="text-xs text-slate-400 bg-slate-100 px-2 py-0.5 rounded-full shrink-0">🔒 已解锁</span>
          </div>
          <div class="flex gap-2 shrink-0">
            <Button variant="primary" size="sm" onClick$={doUpload}><Icon name="upload" size={16} /> 上传</Button>
            <Button variant="ghost" size="sm" onClick$={doLock}>锁定</Button>
          </div>
        </div>
        <div class="flex-1 overflow-auto p-4 sm:p-6">
          {items.value.length === 0 ? (
            <div class="flex flex-col items-center justify-center h-full text-slate-400">
              <div class="text-5xl mb-4">📁</div>
              <p class="text-lg font-medium text-slate-500">此私密空间为空</p>
              <p class="text-sm mt-1">点击「上传」按钮添加加密文件</p>
            </div>
          ) : (
            <div class="space-y-2">
              {items.value.map(item => (
                <div key={item.id} class="flex items-center gap-3 sm:gap-4 px-4 py-3 bg-white rounded-xl border border-slate-200 hover:shadow-sm transition-all">
                  <div class="w-10 h-10 rounded-lg bg-slate-100 flex items-center justify-center text-lg shrink-0">📄</div>
                  <div class="flex-1 min-w-0">
                    <p class="text-sm font-medium text-slate-700 truncate">{item.filename}</p>
                    <p class="text-xs text-slate-400">{fmtSize(item.file_size)} · {item.mime_type}</p>
                  </div>
                  <button onClick$={() => doDownload(item.id)} class="touch-target px-3 py-1.5 text-xs font-medium text-indigo-600 bg-indigo-50 hover:bg-indigo-100 rounded-lg transition-colors">下载</button>
                  <button onClick$={() => doDeleteItem(item.id)} class="touch-target px-3 py-1.5 text-xs font-medium text-red-600 bg-red-50 hover:bg-red-100 rounded-lg transition-colors">删除</button>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  // Vault list view (default)
  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-slate-200 bg-white">
        <div>
          <h1 class="text-lg font-semibold text-slate-900">私密空间</h1>
          <p class="text-sm text-slate-500 mt-0.5">加密存储你的私密文件</p>
        </div>
        <Button variant="primary" size="sm" onClick$={() => showCreate.value = !showCreate.value}>
          <Icon name="plus" size={16} /> 新建私密空间
        </Button>
      </div>

      {/* Create form */}
      {showCreate.value && (
        <div class="px-4 sm:px-6 py-4 border-b border-slate-200 bg-slate-50/80 space-y-3">
          <input type="text" bind:value={newName} placeholder="私密空间名称"
            class="w-full max-w-md px-3 py-2.5 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500" />
          <div class="flex flex-col sm:flex-row gap-3">
            <input type="password" bind:value={newPw} placeholder="设置密码（至少6位）"
              class="w-full sm:w-48 px-3 py-2.5 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500" />
            <input type="password" bind:value={newPw2} placeholder="确认密码"
              class="w-full sm:w-48 px-3 py-2.5 rounded-lg border border-slate-300 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500" />
          </div>
          {createErr.value && <p class="text-sm text-red-600">{createErr.value}</p>}
          <div class="flex gap-2">
            <Button size="sm" onClick$={doCreate} loading={loading.value}>创建</Button>
            <Button variant="ghost" size="sm" onClick$={() => showCreate.value = false}>取消</Button>
          </div>
          <p class="text-xs text-amber-600">⚠ 密码丢失后数据无法恢复！请牢记密码。</p>
        </div>
      )}

      {/* Vault list */}
      <div class="flex-1 overflow-auto p-4 sm:p-6">
        {vaults.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-slate-400">
            <div class="w-20 h-20 rounded-2xl bg-slate-100 flex items-center justify-center text-4xl mb-5">🔒</div>
            <h3 class="text-lg font-semibold text-slate-500 mb-1">暂无私密空间</h3>
            <p class="text-sm text-slate-400 mb-6">创建加密空间，安全存储你的私密文件</p>
            <Button variant="primary" onClick$={() => showCreate.value = true}>新建私密空间</Button>
          </div>
        ) : (
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {vaults.value.map(v => (
              <div key={v.id} class="bg-white rounded-xl border border-slate-200 hover:shadow-md transition-all p-5 cursor-pointer"
                onClick$={() => { unlockId.value = v.id; unlockPw.value = ""; unlockErr.value = ""; }}>
                <div class="flex items-center gap-3 mb-3">
                  <div class="w-10 h-10 rounded-xl bg-indigo-50 flex items-center justify-center text-xl shrink-0">🔒</div>
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-semibold text-slate-900 truncate">{v.name}</h3>
                    <p class="text-xs text-slate-400">{v.file_count} 个文件 · {fmtSize(v.total_size)}</p>
                  </div>
                </div>
                <div class="flex gap-3 pt-3 border-t border-slate-100">
                  <button onClick$={(e: any) => { e.stopPropagation(); unlockId.value = v.id; }} class="text-xs font-medium text-indigo-600 hover:text-indigo-800 touch-target px-3 py-1.5 rounded-lg hover:bg-indigo-50">解锁</button>
                  <button onClick$={(e: any) => { e.stopPropagation(); doDeleteVault(v.id); }} class="text-xs font-medium text-red-500 hover:text-red-700 touch-target px-3 py-1.5 rounded-lg hover:bg-red-50">删除</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
