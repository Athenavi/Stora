# E2EE 私密空间设计（零知识）

> **决策**（已与产品确认）：
> 1. **零知识，元数据一并加密** —— 文件名、大小、类型都由客户端加密，服务端零知识；
>    代价是**私密空间完全不参与内容搜索**，只能进去翻。
> 2. **一次性恢复码** —— 忘记口令时凭恢复码找回，恢复码由用户自行保管，服务端只存它的密文产物。
>
> 本文是**密码学契约**。密码学实现一旦写错，后果是用户数据永久不可读，因此格式与参数
> 先定死，客户端与服务端各自对着本文实现。

---

## 一、威胁模型

| 防护对象 | 是否防护 |
|---|---|
| 服务端被入侵、数据库泄露 | ✅ 攻击者拿不到任何明文（含文件名） |
| 运维人员（含超级管理员）窥探 | ✅ 服务端无解密能力 |
| 对象存储被拖走 | ✅ 只有密文 |
| 客户端设备被入侵 / 浏览器扩展作恶 | ❌ 不在防护范围 |
| 用户口令过于简单 | ⚠️ 仅靠 KDF 迭代次数缓解，不做强制口令强度校验 |

---

## 二、密钥层级

```
用户口令 passphrase
   │  PBKDF2-SHA256(passphrase, kdf_salt, kdf_iterations, 32 bytes)
   ▼
主密钥 masterKey                    ← 永不离开客户端，永不发送
   │  AES-256-GCM 解包
   ▼
保险库密钥 vaultKey                 ← 每个保险库一条，随机 32 字节
   │  HKDF-SHA256(vaultKey, salt=vaultId, info="name")
   ├──────────────► nameKey          → 加密文件名与元数据
   │
   └── 每个条目随机生成 32 字节 fileKey，用 nameKey 包裹后放进 enc_meta
       （不依赖服务端分配的 itemId，因此上传可以是单次请求）
```

**条目密钥**：`fileKey` 是**每个文件随机生成的 32 字节**，用 `nameKey` 加密后随 `enc_meta`
一起上传。这样无需服务端先分配 itemId 再回传（否则上传要两阶段），同时每个条目仍有独立密钥。

**KDF 参数**（首版冻结）

| 参数 | 值 | 理由 |
|---|---|---|
| 算法 | PBKDF2-SHA256 | WebCrypto 内置，无需 wasm；Argon2id 留作后续升级 |
| kdf_iterations | 600000 | OWASP 对 PBKDF2-SHA256 的当前建议下限 |
| kdf_salt | 16 字节随机，每保险库一份 | 由客户端生成，随保险库记录保存 |
| 派生密钥长度 | 32 字节 | AES-256 |

**恢复码**

```
客户端生成 32 随机字节 → base64url 展示给用户（仅展示一次）
恢复密钥 recoveryKey = PBKDF2-SHA256(recoveryCode, kdf_salt, kdf_iterations, 32)
recovery_wrapped_key = AES-256-GCM(recoveryKey, vaultKey)
```

服务端只保存 `recovery_wrapped_key`，**看不到 recoveryCode，也无法用它解包**。

---

## 三、密文格式

**内容与文件名统一采用**：

```
base64( nonce(12 字节随机) || ciphertext || tag(16 字节) )
```

- AES-256-GCM；**每条目每条目独立的 key**，因此 nonce 重用风险被限制在单条目内
  （客户端仍必须每次加密生成新 nonce，不做任何复用）。
- 文件名加密为 `enc_name`：`AES-GCM(nameKey, plaintextName)`。
- 元数据（原始大小、MIME、修改时间）序列化为 JSON 后用同一 `nameKey` 加密为 `enc_meta`；
  服务端只看到密文长度。
- **明文大小不可从密文精确反推**（GCM 只增加固定开销），因此服务端无法据此猜测内容大小。

---

## 四、服务端的职责边界（严格的零知识）

服务端**只做三件事**：

1. 存 `kdf_salt` / `kdf_iterations` / `wrapped_key` / `recovery_wrapped_key`（都是公开参数或密文）。
2. 存密文对象与其 `enc_name` / `enc_meta` / 密文长度。
3. 按 `vault_id` 与 `user_id` 做归属校验与增删。

服务端**永远不**：

- 持有 `masterKey` / `vaultKey` / `fileKey`；
- 尝试解密任何字段（包括"顺手校验一下"）；
- 对 E2EE 条目调用 `EnqueueThumbnail` / `EnqueueOCR` / `ReindexSearch` ——
  服务端读不了，索引里也不该出现（这正是"不参与搜索"的实现方式）；
- 在日志里记录 `enc_name`/`enc_meta` 的**明文**（它们本来就是密文，但避免误记）。

> 与既有 `vaults` 表的关系：现有 vault 是**服务端持钥**的 AES-256-GCM（可搜、可缩略），
> 本文的 E2EE 空间是**另一套**、独立的表与端点。两者并存，语义在 UI 上必须区分清楚：
> 「加密空间（服务端可解密）」vs「私密空间（零知识）」。

---

## 五、交互流程

**创建**

1. 客户端生成 `kdf_salt`、`vaultKey`、`recoveryCode`；
2. 用口令派生 `masterKey`，包裹 `vaultKey`；用 `recoveryCode` 派生 `recoveryKey`，再包裹一次；
3. `POST /api/v2/e2ee/vaults` 提交 `kdf_salt`/`kdf_iterations`/`wrapped_key`/`recovery_wrapped_key`；
4. 客户端**立即展示恢复码**并要求用户确认已抄写（此后不再可见）。

**解锁**：口令 → `masterKey` → 解包 `vaultKey`（或恢复码 → `recoveryKey` → 解包）。`vaultKey` 仅存于内存，随页面关闭消失。

**上传**：客户端加密内容 → `POST /api/v2/e2ee/vaults/{id}/items`（密文 + `enc_name` + `enc_meta`）。

**浏览**：`GET` 列表拿到 `enc_name`，客户端用 `nameKey` 解密显示。

**下载**：`GET /api/v2/e2ee/items/{itemId}/content` 取密文 → 客户端解密。

---

## 六、必须做的验证（否则不上线）

1. **客户端加解密 round-trip**：加密→解密逐字节一致（含空文件、1 字节、多 MB、UTF-8 文件名）。
2. **恢复码 round-trip**：用恢复码能解出同一 `vaultKey`。
3. **服务端契约**：提交的密文字节与取回的逐字节相同；服务端任何接口都不返回 `wrapped_key` 以外的密钥材料。
4. **零知识反证测试**：断言服务端代码里没有任何路径能拿到 `masterKey`/`vaultKey`（人工审阅 + 无解密调用）。
5. **搜索隔离**：E2EE 条目不出现在 `/files/search` 结果中（服务端不索引）。
6. **口令错误**：解包失败必须给出明确错误，且**不得**清空或覆盖已有数据。

---

## 七、实施状态与未完成项（诚实记录）

**已完成（服务端零知识存储）**

- `e2ee_vaults` / `e2ee_items` 两张表（服务端只存公开参数与密文）。
- `internal/api/v2/e2ee/handler.go`：创建/列出/删除保险库，上传/列出/下载/删除密文条目。
- 明确的"不做派生处理"边界：E2EE 条目不会进入搜索索引、缩略图或 OCR 队列。

**未完成（因此该功能当前不可用）**

- **客户端密码学模块**（WebCrypto）：KDF、密钥包裹/解包、内容与文件名加解密。这是关键路径，
  必须单独一轮实现并做第六节的 round-trip 验证。
- **前端 UI**：创建向导（口令 + 恢复码展示与确认）、解锁、加密上传/下载、文件名解密展示。
- **口令变更**：重新包裹 `vaultKey` 即可，无需重新加密数据（设计已支持，实现未做）。
- **Argon2id 升级路径**：PBKDF2 是首版选择；升级需在 `kdf_params` 里带算法标识（当前表结构
  只存 iterations，升级时需加列）。

> ⚠️ 在客户端密码学落地之前，**不要**把本功能暴露给用户：服务端就绪不代表可用。
