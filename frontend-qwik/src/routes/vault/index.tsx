/**
 * Stora Vault — flat design centered unlock + file list
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { routeLoader$, useNavigate, useLocation } from "@builder.io/qwik-city";
import { api, createServerApi } from "~/lib/api";
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
  const vaultList = useSignal<VaultInfo[]>(vaults.value || []);
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
  const vaultSelIds = useSignal<number[]>([]);

  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    const t = loc.url.searchParams.get("vault_token");
    const id = loc.url.searchParams.get("vault_id");
    if (t && id) { vaultToken.value = t; unlockId.value = Number(id); loadItems(Number(id), t); }
  });

  const loadItems = $(async (id: number, token: string) => {
    try { const data = await api.get<VaultItem[]>(`/vaults/${id}/items`, { headers: { "X-Vault-Token": token } }); items.value = data || []; } catch { items.value = []; }
  });

  const doUnlock = $(async () => {
    unlockErr.value = ""; const fd = new FormData(); fd.append("password", unlockPw.value);
    try { const res: any = await api.post(`/vaults/${unlockId.value}/verify-password`, fd); vaultToken.value = res.token; await loadItems(unlockId.value, res.token); } catch (e: any) { unlockErr.value = e.message || "密码错误"; }
  });

  const doLock = $(() => { vaultToken.value = null; items.value = []; unlockPw.value = ""; });

  const doCreate = $(async () => {
    if (!newName.value || !newPw.value) { createErr.value = "请填写名称和密码"; return; }
    if (newPw.value !== newPw2.value) { createErr.value = "两次密码不一致"; return; }
    createErr.value = ""; loading.value = true; const fd = new FormData(); fd.append("name", newName.value); fd.append("password", newPw.value);
    try { await api.post("/vaults", fd); showCreate.value = false; newName.value = ""; newPw.value = ""; newPw2.value = ""; const data = await api.get<VaultInfo[]>("/vaults"); vaultList.value = data || []; } catch (e: any) { createErr.value = e.message || "创建失败"; }
    loading.value = false;
  });

  const doDeleteVault = $(async (id: number) => {
    if (!confirm("确认删除此私密空间？所有加密文件将永久丢失！")) return;
    try { await api.delete(`/vaults/${id}`); vaultList.value = vaultList.value.filter(v => v.id !== id); } catch {}
  });

  const doUpload = $(async () => {
    const input = document.createElement("input"); input.type = "file"; input.multiple = true;
    input.onchange = async () => {
      if (!input.files?.length) return;
      for (const file of Array.from(input.files)) {
        const reader = new FileReader();
        reader.onload = async () => {
          const base64 = (reader.result as string).split(",")[1];
          const fd = new FormData(); fd.append("filename", file.name); fd.append("file_size", String(file.size)); fd.append("mime_type", file.type || "application/octet-stream"); fd.append("file_content", base64);
          try { await api.post(`/vaults/${unlockId.value}/items/upload`, fd, { headers: { "X-Vault-Token": vaultToken.value! } }); await loadItems(unlockId.value, vaultToken.value!); } catch {}
        };
        reader.readAsDataURL(file);
      }
    };
    input.click();
  });

  const doDownload = $(async (itemId: number) => { window.open(`/api/v2/vaults/${unlockId.value}/items/${itemId}`, "_blank"); });

  const doDeleteItem = $(async (itemId: number) => {
    if (!confirm("确认删除？")) return;
    try { await fetch(`/api/v2/vaults/${unlockId.value}/items/${itemId}`, { method: "DELETE", headers: { "X-Vault-Token": vaultToken.value! } }); items.value = items.value.filter(i => i.id !== itemId); } catch {}
  });

  // Locked vault unlock screen — per spec centered layout
  if (unlockId.value > 0 && !vaultToken.value) {
    const vault = vaultList.value.find(v => v.id === unlockId.value);
    return (
      <div class="flex flex-col items-center justify-center h-full gap-6 p-10 text-center max-w-sm mx-auto">
        {/* Lock icon 80x80 circle per spec */}
        <div class="w-20 h-20 rounded-full bg-stora-vault flex items-center justify-center text-4xl">🔒</div>
        {/* Title 28px Bold per spec */}
        <h1 class="text-[28px] font-bold text-stora-foreground">{vault?.name || "私密空间"}</h1>
        {/* Subtitle per spec */}
        <p class="text-sm text-stora-muted-foreground">输入密码以访问你的私密文件</p>
        {unlockErr.value && <div class="w-full px-4 py-3 bg-red-50 border border-stora-border text-sm text-stora-destructive">{unlockErr.value}</div>}
        {/* Password input 360x48 per spec */}
        <input type="password" bind:value={unlockPw}
          onKeyDown$={(e: any) => { if (e.key === "Enter") doUnlock(); }}
          class="w-[360px] max-w-full h-12 px-4 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-vault"
          placeholder="请输入访问密码" />
        {/* Unlock button 160x48 per spec */}
        <button onClick$={doUnlock}
          class="w-[160px] h-12 text-[15px] font-semibold text-white bg-stora-vault hover:bg-[#6D28D9]">
          解锁
        </button>
        <button onClick$={() => { unlockId.value = 0; nav("/vault"); }} class="text-sm text-stora-muted-foreground hover:text-stora-foreground">返回</button>
      </div>
    );
  }

  // Unlocked vault file view
  if (unlockId.value > 0 && vaultToken.value) {
    const vault = vaultList.value.find(v => v.id === unlockId.value);
    return (
      <div class="flex flex-col h-full">
        <div class="flex items-center justify-between px-6 py-4 border-b border-stora-border bg-stora-card">
          <div class="flex items-center gap-3 min-w-0">
            <button onClick$={() => { doLock(); nav("/vault"); }} class="text-stora-muted-foreground hover:text-stora-foreground touch-target p-1">
              <Icon name="chevronLeft" size={20} />
            </button>
            <h1 class="text-lg font-semibold text-stora-foreground truncate">{vault?.name || "私密空间"}</h1>
            <span class="text-xs text-stora-muted-foreground bg-stora-muted px-2 py-0.5">🔒 已解锁</span>
          </div>
          <div class="flex gap-2 shrink-0">
            {vaultSelIds.value.length > 0 ? (
              <>
                <button onClick$={async () => {
                  if (!confirm(`确认删除 ${vaultSelIds.value.length} 项？`)) return;
                  for (const id of vaultSelIds.value) { try { await fetch(`/api/v2/vaults/${unlockId.value}/items/${id}`, { method: "DELETE", headers: { "X-Vault-Token": vaultToken.value! } }); } catch {} }
                  items.value = items.value.filter(i => !vaultSelIds.value.includes(i.id)); vaultSelIds.value = [];
                }} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-destructive hover:bg-red-50">删除 ({vaultSelIds.value.length})</button>
                <button onClick$={() => vaultSelIds.value = []} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-muted-foreground hover:bg-stora-muted">取消</button>
              </>
            ) : (
              <button onClick$={doUpload} class="h-9 px-4 text-sm font-medium text-white bg-stora-vault hover:bg-[#6D28D9]"><span>⬆</span> 上传</button>
            )}
            <button onClick$={doLock} class="h-9 px-4 text-sm font-medium text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-muted">锁定</button>
          </div>
        </div>
        <div class="flex-1 overflow-auto p-6">
          {items.value.length === 0 ? (
            <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
              <div class="text-5xl mb-4">📁</div>
              <p class="text-lg font-medium text-stora-foreground">此私密空间为空</p>
              <p class="text-sm mt-1">点击「上传」按钮添加加密文件</p>
            </div>
          ) : (
            <div class="divide-y divide-stora-border">
              {items.value.map(item => {
                const sel = vaultSelIds.value.includes(item.id);
                return (
                  <div key={item.id} onClick$={() => { const i = vaultSelIds.value.indexOf(item.id); if (i >= 0) vaultSelIds.value.splice(i, 1); else vaultSelIds.value.push(item.id); vaultSelIds.value = [...vaultSelIds.value]; }}
                    class={`flex items-center gap-4 px-4 py-3 bg-stora-card cursor-pointer ${sel ? 'bg-stora-muted' : 'hover:bg-stora-muted'}`}>
                    <input type="checkbox" checked={sel} class="border-stora-border w-4 h-4 shrink-0" />
                    <span class="text-lg">📄</span>
                    <div class="flex-1 min-w-0">
                      <p class="text-sm font-medium text-stora-foreground truncate">{item.filename}</p>
                      <p class="text-xs text-stora-muted-foreground">{fmtSize(item.file_size)} · {item.mime_type}</p>
                    </div>
                    <button onClick$={(e: any) => { e.stopPropagation(); doDownload(item.id); }} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-primary hover:bg-stora-muted">下载</button>
                    <button onClick$={(e: any) => { e.stopPropagation(); doDeleteItem(item.id); }} class="touch-target px-3 py-1.5 text-xs font-medium text-stora-destructive hover:bg-red-50">删除</button>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    );
  }

  // Vault list view (default)
  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 bg-stora-card border-b border-stora-border">
        <h1 class="text-[28px] font-bold text-stora-foreground">私密空间</h1>
        <p class="text-sm text-stora-muted-foreground mt-1">加密存储你的私密文件</p>
      </div>

      {/* Create form */}
      {showCreate.value && (
        <div class="px-6 py-4 border-b border-stora-border bg-stora-muted space-y-3">
          <input type="text" bind:value={newName} placeholder="私密空间名称"
            class="w-full max-w-md h-10 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-vault" />
          <div class="flex flex-col sm:flex-row gap-3">
            <input type="password" bind:value={newPw} placeholder="设置密码（至少6位）"
              class="w-full sm:w-48 h-10 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-vault" />
            <input type="password" bind:value={newPw2} placeholder="确认密码"
              class="w-full sm:w-48 h-10 px-3 text-sm border border-stora-border bg-white text-stora-foreground placeholder:text-stora-nav-text outline-none focus:border-stora-vault" />
          </div>
          {createErr.value && <p class="text-sm text-stora-destructive">{createErr.value}</p>}
          <div class="flex gap-2">
            <button onClick$={doCreate} disabled={loading.value} class="h-9 px-4 text-sm font-medium text-white bg-stora-vault hover:bg-[#6D28D9]">{loading.value ? "..." : "创建"}</button>
            <button onClick$={() => showCreate.value = false} class="h-9 px-4 text-sm font-medium text-stora-foreground bg-stora-card border border-stora-border hover:bg-stora-muted">取消</button>
          </div>
          <p class="text-xs text-amber-600">⚠ 密码丢失后数据无法恢复！请牢记密码。</p>
        </div>
      )}

      <div class="flex-1 overflow-auto p-6">
        {vaultList.value.length === 0 ? (
          <div class="flex flex-col items-center justify-center h-full text-stora-muted-foreground">
            <div class="w-20 h-20 bg-stora-muted flex items-center justify-center text-4xl mb-5">🔒</div>
            <h3 class="text-lg font-semibold text-stora-foreground mb-1">暂无私密空间</h3>
            <p class="text-sm text-stora-muted-foreground mb-6">创建加密空间，安全存储你的私密文件</p>
            <button onClick$={() => showCreate.value = true} class="px-6 py-3 bg-stora-vault text-white text-sm font-medium">新建私密空间</button>
          </div>
        ) : (
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {vaultList.value.map(v => (
              <div key={v.id} class="bg-stora-card border border-stora-border hover:border-stora-vault cursor-pointer p-5"
                onClick$={() => { unlockId.value = v.id; unlockPw.value = ""; unlockErr.value = ""; }}>
                <div class="flex items-center gap-3 mb-3">
                  <div class="w-10 h-10 bg-stora-muted flex items-center justify-center text-xl shrink-0">🔒</div>
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-semibold text-stora-foreground truncate">{v.name}</h3>
                    <p class="text-xs text-stora-muted-foreground">{v.file_count} 个文件 · {fmtSize(v.total_size)}</p>
                  </div>
                </div>
                <div class="flex gap-3 pt-3 border-t border-stora-border">
                  <button onClick$={(e: any) => { e.stopPropagation(); unlockId.value = v.id; }} class="text-xs font-medium text-stora-vault hover:bg-stora-muted touch-target px-3 py-1.5">解锁</button>
                  <button onClick$={(e: any) => { e.stopPropagation(); doDeleteVault(v.id); }} class="text-xs font-medium text-stora-destructive hover:bg-red-50 touch-target px-3 py-1.5">删除</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});
