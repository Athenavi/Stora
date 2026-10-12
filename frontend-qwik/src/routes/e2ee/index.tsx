/**
 * E2EE 私密空间 —— 零知识
 *
 * 与「加密空间」(vault) 的关键区别：那个由服务端持钥，可搜索、可生成缩略图；
 * 这里的一切都在浏览器里加密，服务端只存密文，因此**不参与搜索**。
 * 契约见 docs/design-e2ee.md。
 *
 * 实现约束：Qwik 的 $() 闭包不能引用**组件作用域内的普通函数**（它们不会被打包到
 * 客户端），所以所有数据访问与加解密辅助都定义在模块级，组件只持有状态与事件。
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import {
  listE2eeVaults,
  createE2eeVault,
  deleteE2eeVault,
  listE2eeItems,
  uploadE2eeItem,
  downloadE2eeItem,
  deleteE2eeItem,
  type E2eeVault,
} from "~/lib/api";
import {
  createVault,
  unlockVault,
  unlockVaultWithRecovery,
  encryptItemNameUnlocked,
  encryptItemMetaUnlocked,
  decryptItemMetaUnlocked,
  encryptFile,
  decryptFile,
  rememberVaultKey,
  forgetVaultKey,
  isUnlocked,
} from "~/lib/e2ee/crypto";

interface Row {
  id: number;
  name: string;
  size: number;
  mime: string;
  /**
   * 密文元数据。**故意不缓存解密出的 CryptoKey**：它不可序列化，放进信号会让
   * Qwik 的 $() 闭包无法捕获这一行；何况密钥在内存里少待一会儿总是更好。
   */
  encMeta: string;
  decryptable: boolean;
}

function fmtSize(b: number): string {
  if (!b) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

/** 解密一个保险库的全部条目元数据（模块级，才能被 $() 闭包引用）。 */
async function decryptRows(vault: E2eeVault): Promise<Row[]> {
  const items = await listE2eeItems(vault.id);
  const out: Row[] = [];
  for (const it of items) {
    try {
      const { meta } = await decryptItemMetaUnlocked(vault.id, it.enc_meta);
      out.push({
        id: it.id,
        name: meta.name,
        size: meta.size,
        mime: meta.mime,
        encMeta: it.enc_meta,
        decryptable: true,
      });
    } catch {
      // 解不开说明密钥不对：照实列出来，但不假装它可用。
      out.push({ id: it.id, name: "（无法解密）", size: 0, mime: "", encMeta: it.enc_meta, decryptable: false });
    }
  }
  return out;
}

/** 加密单个文件，返回可以直接上传的三样东西。 */
async function encryptOne(vaultId: number, file: File) {
  const encName = await encryptItemNameUnlocked(vaultId, file.name);
  const { encMeta, fileKey } = await encryptItemMetaUnlocked(vaultId, {
    name: file.name,
    size: file.size,
    mime: file.type || "application/octet-stream",
  });
  const cipher = await encryptFile(file, fileKey);
  return { encName, encMeta, cipher };
}

export default component$(() => {
  const vaults = useSignal<E2eeVault[]>([]);
  const openVault = useSignal<E2eeVault | null>(null);
  const rows = useSignal<Row[]>([]);
  const busy = useSignal("");
  const error = useSignal("");
  /** 仅在创建后展示一次的恢复码。 */
  const recovery = useSignal("");
  const recoveryAcknowledged = useSignal(false);

  const newName = useSignal("");
  const newPass = useSignal("");
  const unlockSecret = useSignal("");
  const useRecoveryToUnlock = useSignal(false);

  const refresh = $(async () => {
    try {
      vaults.value = await listE2eeVaults();
    } catch (e: any) {
      error.value = e.message || "加载失败";
    }
  });

  const openItems = $(async (vault: E2eeVault) => {
    busy.value = "正在解密条目列表…";
    error.value = "";
    try {
      rows.value = await decryptRows(vault);
    } catch (e: any) {
      error.value = e.message || "加载条目失败";
    } finally {
      busy.value = "";
    }
  });

  // 只在客户端跑：密钥材料不允许出现在 SSR 输出里。
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(() => {
    refresh();
  });

  const handleCreate = $(async () => {
    error.value = "";
    if (!newName.value.trim() || !newPass.value) {
      error.value = "请填写名称与口令";
      return;
    }
    busy.value = "正在派生密钥（约需 1 秒）…";
    try {
      const created = await createVault(newPass.value);
      busy.value = "正在创建…";
      const res = await createE2eeVault({
        name: newName.value.trim(),
        kdf_salt: created.material.kdfSalt,
        kdf_iterations: created.material.kdfIterations,
        wrapped_key: created.material.wrappedKey,
        recovery_wrapped_key: created.material.recoveryWrappedKey,
      });
      rememberVaultKey(res.id, created.vaultKeyBytes);
      recovery.value = created.recoveryCode;
      recoveryAcknowledged.value = false;
      newName.value = "";
      newPass.value = "";
      await refresh();
    } catch (e: any) {
      error.value = e.message || "创建失败";
    } finally {
      busy.value = "";
    }
  });

  const handleUnlock = $(async (vault: E2eeVault) => {
    error.value = "";
    if (!unlockSecret.value) {
      error.value = useRecoveryToUnlock.value ? "请输入恢复码" : "请输入口令";
      return;
    }
    busy.value = "正在派生密钥（约需 1 秒）…";
    try {
      const material = {
        kdfSalt: vault.kdf_salt,
        kdfIterations: vault.kdf_iterations,
        wrappedKey: vault.wrapped_key,
        recoveryWrappedKey: vault.recovery_wrapped_key,
      };
      const key = useRecoveryToUnlock.value
        ? await unlockVaultWithRecovery(unlockSecret.value, material)
        : await unlockVault(unlockSecret.value, material);
      rememberVaultKey(vault.id, key);
      unlockSecret.value = "";
      openVault.value = vault;
      await openItems(vault);
    } catch (e: any) {
      error.value = e.message || "解锁失败";
    } finally {
      busy.value = "";
    }
  });

  const handleUpload = $(async (files: FileList | null) => {
    const vault = openVault.value;
    if (!vault || !files || files.length === 0) return;
    error.value = "";
    try {
      for (const file of Array.from(files)) {
        busy.value = `正在加密并上传 ${file.name}…`;
        const { encName, encMeta, cipher } = await encryptOne(vault.id, file);
        await uploadE2eeItem(vault.id, cipher, encName, encMeta);
      }
      await openItems(vault);
    } catch (e: any) {
      error.value = e.message || "上传失败";
    } finally {
      busy.value = "";
    }
  });

  const handleDownload = $(async (row: Row) => {
    const vault = openVault.value;
    if (!vault || !row.decryptable) return;
    busy.value = `正在下载并解密 ${row.name}…`;
    try {
      // 密钥不常驻：下载时才从 enc_meta 解出来，用完即弃。
      const { fileKey } = await decryptItemMetaUnlocked(vault.id, row.encMeta);
      const cipher = await downloadE2eeItem(row.id);
      const plain = await decryptFile(cipher, fileKey, row.size);
      const url = URL.createObjectURL(plain);
      const a = document.createElement("a");
      a.href = url;
      a.download = row.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e: any) {
      error.value = e.message || "下载失败";
    } finally {
      busy.value = "";
    }
  });

  const handleDeleteItem = $(async (row: Row) => {
    try {
      await deleteE2eeItem(row.id);
      rows.value = rows.value.filter((r) => r.id !== row.id);
    } catch (e: any) {
      error.value = e.message || "删除失败";
    }
  });

  const handleDeleteVault = $(async (vault: E2eeVault) => {
    try {
      await deleteE2eeVault(vault.id);
      forgetVaultKey(vault.id);
      if (openVault.value?.id === vault.id) {
        openVault.value = null;
        rows.value = [];
      }
      await refresh();
    } catch (e: any) {
      error.value = e.message || "删除失败";
    }
  });

  const handleLock = $(() => {
    const vault = openVault.value;
    if (vault) forgetVaultKey(vault.id);
    openVault.value = null;
    rows.value = [];
  });

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">私密空间（零知识加密）</h1>
        <p class="text-sm text-slate-500 mt-0.5">
          密钥只在浏览器里，服务端只存密文，因此这里的内容<strong>不会出现在搜索结果中</strong>。忘记口令时请用恢复码。
        </p>
      </div>

      <div class="flex-1 overflow-auto scrollbar-thin p-6 space-y-4">
        {error.value && (
          <div class="px-4 py-2 rounded-lg bg-red-50 border border-red-200 text-sm text-red-700">{error.value}</div>
        )}
        {busy.value && (
          <div class="px-4 py-2 rounded-lg bg-slate-50 border border-slate-200 text-sm text-slate-600">{busy.value}</div>
        )}

        {recovery.value && (
          <div class="px-4 py-3 rounded-lg bg-amber-50 border border-amber-200">
            <p class="text-sm font-medium text-amber-900">请立即抄写恢复码——它只会显示这一次</p>
            <p class="mt-2 font-mono text-sm break-all bg-white border border-amber-200 rounded p-2">{recovery.value}</p>
            <label class="flex items-center gap-2 mt-2 text-xs text-amber-900">
              <input
                type="checkbox"
                checked={recoveryAcknowledged.value}
                onChange$={() => (recoveryAcknowledged.value = !recoveryAcknowledged.value)}
              />
              我已妥善保存恢复码
            </label>
            <button
              class="mt-2 px-3 py-1.5 text-xs rounded border border-amber-300 bg-white disabled:opacity-50"
              disabled={!recoveryAcknowledged.value}
              onClick$={() => (recovery.value = "")}
            >
              完成
            </button>
          </div>
        )}

        {!openVault.value ? (
          <>
            <div class="bg-white rounded-lg border border-slate-200 p-4">
              <h2 class="text-sm font-semibold text-slate-700 mb-3">新建私密空间</h2>
              <div class="flex flex-wrap gap-2">
                <input class="px-3 py-2 text-sm border border-slate-200 rounded-md" placeholder="名称" bind:value={newName} />
                <input
                  class="px-3 py-2 text-sm border border-slate-200 rounded-md"
                  type="password"
                  placeholder="口令（至少 8 位）"
                  bind:value={newPass}
                />
                <button class="px-4 py-2 text-sm rounded-md bg-stora-primary text-white" onClick$={handleCreate}>
                  创建
                </button>
              </div>
            </div>

            {vaults.value.length === 0 ? (
              <div class="text-center text-slate-400 mt-16">还没有私密空间</div>
            ) : (
              <div class="space-y-2">
                {vaults.value.map((v) => (
                  <div key={v.id} class="flex items-center gap-3 px-4 py-3 bg-white rounded-lg border border-slate-200">
                    <div class="w-10 h-10 rounded-lg bg-violet-50 flex items-center justify-center text-lg">🔐</div>
                    <div class="flex-1 min-w-0">
                      <p class="text-sm font-medium text-slate-700 truncate">{v.name}</p>
                      <p class="text-xs text-slate-400">
                        {v.item_count} 个条目 · {v.created_at?.slice(0, 10)}
                        {isUnlocked(v.id) ? " · 已解锁" : ""}
                      </p>
                    </div>
                    <div class="flex gap-2">
                      <input
                        class="w-40 px-2 py-1.5 text-xs border border-slate-200 rounded"
                        type={useRecoveryToUnlock.value ? "text" : "password"}
                        placeholder={useRecoveryToUnlock.value ? "恢复码" : "口令"}
                        bind:value={unlockSecret}
                      />
                      <button
                        class="px-3 py-1.5 text-xs rounded border border-slate-200 hover:bg-slate-50"
                        onClick$={() => handleUnlock(v)}
                      >
                        解锁
                      </button>
                      <button
                        class="px-3 py-1.5 text-xs rounded border border-slate-200 hover:bg-slate-50"
                        onClick$={() => (useRecoveryToUnlock.value = !useRecoveryToUnlock.value)}
                      >
                        {useRecoveryToUnlock.value ? "用口令" : "用恢复码"}
                      </button>
                      <button
                        class="px-3 py-1.5 text-xs rounded border border-red-200 text-red-600 hover:bg-red-50"
                        onClick$={() => handleDeleteVault(v)}
                      >
                        删除
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        ) : (
          <>
            <div class="flex items-center gap-3">
              <h2 class="text-sm font-semibold text-slate-700">{openVault.value.name}</h2>
              <label class="px-3 py-1.5 text-xs rounded border border-slate-200 hover:bg-slate-50 cursor-pointer">
                加密上传
                <input
                  type="file"
                  multiple
                  class="hidden"
                  onChange$={(e) => handleUpload((e.target as HTMLInputElement).files)}
                />
              </label>
              <button class="px-3 py-1.5 text-xs rounded border border-slate-200 hover:bg-slate-50" onClick$={handleLock}>
                锁定
              </button>
            </div>

            {rows.value.length === 0 ? (
              <div class="text-center text-slate-400 mt-16">这个私密空间还是空的</div>
            ) : (
              <div class="space-y-2">
                {rows.value.map((r) => (
                  <div key={r.id} class="flex items-center gap-3 px-4 py-3 bg-white rounded-lg border border-slate-200">
                    <div class="w-10 h-10 rounded-lg bg-slate-100 flex items-center justify-center text-lg">🔒</div>
                    <div class="flex-1 min-w-0">
                      <p class="text-sm font-medium text-slate-700 truncate">{r.name}</p>
                      <p class="text-xs text-slate-400">
                        {fmtSize(r.size)} · {r.mime}
                      </p>
                    </div>
                    <button
                      class="px-3 py-1.5 text-xs rounded border border-slate-200 hover:bg-slate-50 disabled:opacity-40"
                      disabled={!r.decryptable}
                      onClick$={() => handleDownload(r)}
                    >
                      下载解密
                    </button>
                    <button
                      class="px-3 py-1.5 text-xs rounded border border-red-200 text-red-600 hover:bg-red-50"
                      onClick$={() => handleDeleteItem(r)}
                    >
                      删除
                    </button>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
});
