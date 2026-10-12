/**
 * E2EE 客户端密码学 —— 零知识私密空间
 *
 * 契约见 docs/design-e2ee.md。本文件是**唯一持有密钥的地方**：服务端拿不到
 * masterKey / vaultKey / fileKey，所以这里的每个函数都是安全性的落脚点。
 *
 * 三条硬规则：
 *   1. 密钥材料只存在于内存，绝不写入 localStorage / cookie / 服务端。
 *   2. 每次加密都用新随机 nonce，绝不复用（AES-GCM 下复用即灾难）。
 *   3. 服务端提交的参数（salt / iterations / wrapped key）视为不可信输入校验。
 */

const PBKDF2_ITERATIONS = 600_000;
const SALT_BYTES = 16;
const KEY_BYTES = 32;
const NONCE_BYTES = 12;
const TAG_BYTES = 16;

/** 分块加密的块大小。大文件不能整体进内存。 */
export const E2EE_CHUNK_SIZE = 4 << 20; // 4 MiB

const enc = new TextEncoder();
const dec = new TextDecoder();

// ── 编解码 ────────────────────────────────────────────────────────────────

function toB64(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function fromB64(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function randomBytes(n: number): Uint8Array {
  const b = new Uint8Array(n);
  crypto.getRandomValues(b);
  return b;
}

/**
 * 生成恢复码：32 随机字节的 base64。
 * 只展示一次，由用户自行保管；服务端只会看到它的密文产物。
 */
export function generateRecoveryCode(): string {
  return toB64(randomBytes(KEY_BYTES));
}

// ── 密钥派生与包裹 ────────────────────────────────────────────────────────

/** 从口令派生主密钥。返回的 CryptoKey 不可导出，也永不发送。 */
export async function deriveMasterKey(
  passphrase: string,
  salt: Uint8Array,
  iterations: number,
): Promise<CryptoKey> {
  if (!passphrase) throw new Error("口令不能为空");
  const base = await crypto.subtle.importKey("raw", enc.encode(passphrase), "PBKDF2", false, [
    "deriveKey",
  ]);
  return crypto.subtle.deriveKey(
    { name: "PBKDF2", salt: salt as BufferSource, iterations, hash: "SHA-256" },
    base,
    { name: "AES-GCM", length: 256 },
    false, // 不可导出：即便有 XSS，也无法直接把密钥抠出来带走
    ["encrypt", "decrypt"],
  );
}

/** AES-GCM 加密，输出 base64(nonce || ciphertext || tag)。 */
export async function encryptBytes(key: CryptoKey, plaintext: Uint8Array): Promise<string> {
  const nonce = randomBytes(NONCE_BYTES); // 每次调用都是新的
  const ct = new Uint8Array(
    await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, key, plaintext as BufferSource),
  );
  const out = new Uint8Array(nonce.length + ct.length);
  out.set(nonce, 0);
  out.set(ct, nonce.length);
  return toB64(out);
}

/** 解密 base64(nonce || ciphertext || tag)。口令/密钥不对时抛出。 */
export async function decryptBytes(key: CryptoKey, payload: string): Promise<Uint8Array> {
  const raw = fromB64(payload);
  if (raw.length < NONCE_BYTES + TAG_BYTES) throw new Error("密文格式不正确");
  const nonce = raw.slice(0, NONCE_BYTES);
  const ct = raw.slice(NONCE_BYTES);
  const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv: nonce }, key, ct as BufferSource);
  return new Uint8Array(pt);
}

/** 从 vaultKey 派生子密钥（HKDF-SHA256）。 */
export async function deriveSubKey(
  vaultKeyBytes: Uint8Array,
  vaultId: number,
  info: string,
): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey("raw", vaultKeyBytes as BufferSource, "HKDF", false, [
    "deriveKey",
  ]);
  return crypto.subtle.deriveKey(
    {
      name: "HKDF",
      hash: "SHA-256",
      salt: enc.encode(String(vaultId)),
      info: enc.encode(info),
    },
    base,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

// ── 保险库创建与解锁 ──────────────────────────────────────────────────────

/** 服务端保存的公开参数（服务端无法用它解密任何东西）。 */
export interface VaultMaterial {
  kdfSalt: string;
  kdfIterations: number;
  wrappedKey: string;
  recoveryWrappedKey: string;
}

export interface CreatedVault {
  material: VaultMaterial;
  recoveryCode: string;
  /** 保险库密钥，仅内存持有。 */
  vaultKeyBytes: Uint8Array;
}

/** 创建保险库：生成 salt/vaultKey/恢复码，并各包裹一份 vaultKey。 */
export async function createVault(passphrase: string): Promise<CreatedVault> {
  if (passphrase.length < 8) throw new Error("口令至少 8 个字符");

  const salt = randomBytes(SALT_BYTES);
  const vaultKeyBytes = randomBytes(KEY_BYTES);
  const recoveryCode = generateRecoveryCode();

  const masterKey = await deriveMasterKey(passphrase, salt, PBKDF2_ITERATIONS);
  const wrappedKey = await encryptBytes(masterKey, vaultKeyBytes);

  const recoveryKey = await deriveMasterKey(recoveryCode, salt, PBKDF2_ITERATIONS);
  const recoveryWrappedKey = await encryptBytes(recoveryKey, vaultKeyBytes);

  return {
    material: {
      kdfSalt: toB64(salt),
      kdfIterations: PBKDF2_ITERATIONS,
      wrappedKey,
      recoveryWrappedKey,
    },
    recoveryCode,
    vaultKeyBytes,
  };
}

/** 用口令解锁，返回 vaultKey。口令错误时抛出。 */
export async function unlockVault(passphrase: string, m: VaultMaterial): Promise<Uint8Array> {
  const masterKey = await deriveMasterKey(passphrase, fromB64(m.kdfSalt), m.kdfIterations);
  try {
    return await decryptBytes(masterKey, m.wrappedKey);
  } catch {
    throw new Error("口令不正确");
  }
}

/** 用恢复码解锁（忘记口令时的唯一出路）。 */
export async function unlockVaultWithRecovery(
  recoveryCode: string,
  m: VaultMaterial,
): Promise<Uint8Array> {
  if (!m.recoveryWrappedKey) throw new Error("该保险库没有恢复码");
  const recoveryKey = await deriveMasterKey(recoveryCode, fromB64(m.kdfSalt), m.kdfIterations);
  try {
    return await decryptBytes(recoveryKey, m.recoveryWrappedKey);
  } catch {
    throw new Error("恢复码不正确");
  }
}

/** 改口令：只重新包裹 vaultKey，不需要重新加密任何数据。 */
export async function rewrapVault(
  vaultKeyBytes: Uint8Array,
  newPassphrase: string,
  m: VaultMaterial,
): Promise<VaultMaterial> {
  const salt = randomBytes(SALT_BYTES);
  const masterKey = await deriveMasterKey(newPassphrase, salt, PBKDF2_ITERATIONS);
  return {
    ...m,
    kdfSalt: toB64(salt),
    kdfIterations: PBKDF2_ITERATIONS,
    wrappedKey: await encryptBytes(masterKey, vaultKeyBytes),
  };
}

// ── 条目加解密 ────────────────────────────────────────────────────────────

/** enc_meta 的明文结构（服务端只看到它的密文）。 */
export interface ItemMeta {
  name: string;
  size: number;
  mime: string;
  /** 用 nameKey 包裹的条目密钥（base64），使每个条目有独立密钥。 */
  wrappedFileKey: string;
}

/** 加密条目文件名（列表展示字段；内容与元数据另有独立密钥）。 */
export async function encryptItemName(
  vaultId: number,
  vaultKeyBytes: Uint8Array,
  name: string,
): Promise<string> {
  const nameKey = await deriveSubKey(vaultKeyBytes, vaultId, "name");
  return encryptBytes(nameKey, enc.encode(name));
}

/**
 * 加密条目的名称与元数据，并返回该条目自己的 fileKey。
 *
 * 调用方拿返回的 fileKey 去 encryptFile；它被包裹在 enc_meta 里随内容一起上传，
 * 所以服务端拿不到它，而下次解密可以从 enc_meta 取回。
 */
export async function encryptItemMeta(
  vaultId: number,
  vaultKeyBytes: Uint8Array,
  meta: { name: string; size: number; mime: string },
): Promise<{ encMeta: string; fileKey: CryptoKey }> {
  const nameKey = await deriveSubKey(vaultKeyBytes, vaultId, "name");
  const fileKeyBytes = randomBytes(KEY_BYTES);
  const payload: ItemMeta = {
    ...meta,
    wrappedFileKey: await encryptBytes(nameKey, fileKeyBytes),
  };
  const encMeta = await encryptBytes(nameKey, enc.encode(JSON.stringify(payload)));
  const fileKey = await crypto.subtle.importKey(
    "raw",
    fileKeyBytes as BufferSource,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
  return { encMeta, fileKey };
}

/** 解密条目元数据，同时取回该条目的 fileKey。 */
export async function decryptItemMeta(
  vaultId: number,
  vaultKeyBytes: Uint8Array,
  encMeta: string,
): Promise<{ meta: ItemMeta; fileKey: CryptoKey }> {
  const nameKey = await deriveSubKey(vaultKeyBytes, vaultId, "name");
  const json = dec.decode(await decryptBytes(nameKey, encMeta));
  const meta = JSON.parse(json) as ItemMeta;

  const fileKeyBytes = await decryptBytes(nameKey, meta.wrappedFileKey);
  const fileKey = await crypto.subtle.importKey(
    "raw",
    fileKeyBytes as BufferSource,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
  return { meta, fileKey };
}

/**
 * 分块加密文件内容。
 *
 * 格式：每块 [nonce(12) || ciphertext || tag]，块之间无关联；块大小固定为
 * E2EE_CHUNK_SIZE，最后一块可短。解密方按同样大小切分即可。
 * 不整体进内存，因此可以处理远超浏览器内存上限的文件。
 */
export async function encryptFile(file: File, fileKey: CryptoKey): Promise<Blob> {
  const parts: BlobPart[] = [];
  for (let offset = 0; offset < file.size; offset += E2EE_CHUNK_SIZE) {
    const slice = file.slice(offset, Math.min(offset + E2EE_CHUNK_SIZE, file.size));
    const buf = new Uint8Array(await slice.arrayBuffer());
    const nonce = randomBytes(NONCE_BYTES);
    const ct = new Uint8Array(
      await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, fileKey, buf as BufferSource),
    );
    const framed = new Uint8Array(nonce.length + ct.length);
    framed.set(nonce, 0);
    framed.set(ct, nonce.length);
    parts.push(framed);
  }
  return new Blob(parts, { type: "application/octet-stream" });
}

/** 分块解密（与 encryptFile 对称）。密文被篡改或密钥不对时抛出。 */
export async function decryptFile(cipher: Blob, fileKey: CryptoKey, plainSize: number): Promise<Blob> {
  const parts: BlobPart[] = [];
  const frame = NONCE_BYTES + TAG_BYTES + E2EE_CHUNK_SIZE;
  let offset = 0;
  let written = 0;
  while (offset < cipher.size) {
    // 最后一块比整块短，按剩余明文长度推算它的密文长度。
    const remainingPlain = Math.max(plainSize - written, 0);
    const frameLen = Math.min(frame, cipher.size - offset, remainingPlain + NONCE_BYTES + TAG_BYTES);
    const framed = new Uint8Array(await cipher.slice(offset, offset + frameLen).arrayBuffer());
    const nonce = framed.slice(0, NONCE_BYTES);
    const ct = framed.slice(NONCE_BYTES);
    const pt = new Uint8Array(
      await crypto.subtle.decrypt({ name: "AES-GCM", iv: nonce }, fileKey, ct as BufferSource),
    );
    parts.push(pt);
    written += pt.length;
    offset += frameLen;
  }
  return new Blob(parts, { type: "application/octet-stream" });
}

// ── 解锁状态（仅内存） ────────────────────────────────────────────────────
//
// 页面刷新即失效；不落 localStorage，也不发往服务端。

const unlocked = new Map<number, Uint8Array>();

export function rememberVaultKey(vaultId: number, key: Uint8Array): void {
  unlocked.set(vaultId, key);
}

export function forgetVaultKey(vaultId: number): void {
  unlocked.delete(vaultId);
}

export function vaultKeyOf(vaultId: number): Uint8Array | undefined {
  return unlocked.get(vaultId);
}

export function isUnlocked(vaultId: number): boolean {
  return unlocked.has(vaultId);
}

function requireVaultKey(vaultId: number): Uint8Array {
  const key = unlocked.get(vaultId);
  if (!key) throw new Error("保险库未解锁");
  return key;
}

/** 便捷入口：用内存中的 vaultKey 加密条目文件名。 */
export async function encryptItemNameUnlocked(vaultId: number, name: string): Promise<string> {
  return encryptItemName(vaultId, requireVaultKey(vaultId), name);
}

/** 便捷入口：用内存中的 vaultKey 加密条目元数据，并取回其 fileKey。 */
export async function encryptItemMetaUnlocked(
  vaultId: number,
  meta: { name: string; size: number; mime: string },
): Promise<{ encMeta: string; fileKey: CryptoKey }> {
  return encryptItemMeta(vaultId, requireVaultKey(vaultId), meta);
}

/** 便捷入口：用内存中的 vaultKey 解密条目元数据（列表展示用）。 */
export async function decryptItemMetaUnlocked(
  vaultId: number,
  encMeta: string,
): Promise<{ meta: ItemMeta; fileKey: CryptoKey }> {
  return decryptItemMeta(vaultId, requireVaultKey(vaultId), encMeta);
}
