# Stora API 参考

> 本文档由 `cmd/server/main.go` 的路由表整理而来。路由变更后请同步更新这里。

## 约定

| 项 | 说明 |
|---|---|
| Base URL | `http://<host>:9421/api/v2`（v3 为 `/api/v3`） |
| 认证 | `Authorization: Bearer <JWT>`，或同源请求自动携带的 `access_token` cookie |
| 请求体 | `application/json`，或 `multipart/form-data`（文件与表单类端点两者皆可） |
| 成功响应 | `{"success": true, "data": ...}` |
| 错误响应 | `{"success": false, "message": "..."}` |
| 时间格式 | RFC3339 字符串（`2026-07-06T12:00:00+08:00`） |

**状态码**

| 码 | 含义 |
|---|---|
| 400 | 参数无效 |
| 401 | 未认证 / token 失效 |
| 403 | 已认证但无权限 |
| 404 | 资源不存在（也用于「不属于你」以避免探测） |
| 409 | 冲突（重名、上传已完成等） |
| 413 | 请求体过大 |
| 429 | 触发限流 |
| 507 | 存储配额不足 |

**限流**：`POST /auth/login` 10 次/分钟/IP；`POST /auth/register` 3 次/分钟/IP。
限流按 TCP 对端地址计算；仅在显式配置 `TRUSTED_IP_HEADER` 时才信任转发头。

**超时**：普通请求 60 秒；传输类路由（`/files/upload`、`/files/download`、`/files/preview`、
`/files/blocks`、`/sync`、`/share`、`/wopi`）600 秒。服务端 `WriteTimeout` 600 秒。

---

## 公开端点（无需认证）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查（含数据库连通性） |
| GET | `/api/health/live` | 存活探针 |
| GET | `/api/v2` | API 版本信息 |
| POST | `/api/v2/auth/register` | 注册（限流 3/min） |
| POST | `/api/v2/auth/login` | 登录（限流 10/min，成功时下发 cookie） |
| POST | `/api/v2/auth/refresh` | 用 refresh token 换新令牌（会同步刷新 cookie） |
| POST | `/api/v2/auth/send-code` | 发送邮箱验证码 ⚠️ 尚未实现 |
| POST | `/api/v2/auth/login-with-code` | 验证码登录 ⚠️ 尚未实现（返回 501） |
| GET | `/api/v2/system/maintenance/public-status` | 维护模式公开状态 |

**分享访问**（`code` 为 `short_code`，密码保护的链接需带 `?password=`）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v2/share/{token}` | 访问分享（返回 file_id） |
| GET | `/api/v2/share/{code}/info` | 分享元信息（不计数） |
| GET | `/api/v2/share/{code}/download` | 下载（文件夹/批量分享返回 ZIP） |
| POST | `/api/v2/share/{code}/upload` | 向「收集」型分享上传（需 upload 权限） |
| POST | `/api/v2/share/{code}/save` | 转存到我的网盘（需登录） |
| GET | `/api/v2/share/{code}/qrcode` | 分享二维码（SVG 占位实现） |
| GET | `/api/v2/share/{code}/folder/{folderId}` | 分享内的子目录浏览 |
| GET | `/api/v2/files/shares/access/{code}` | 校验分享密码并返回文件信息 |

---

## 认证与账户

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v2/auth/logout` | 登出（服务端清除 cookie） |
| GET | `/api/v2/auth/me` | 当前用户资料 |
| PATCH | `/api/v2/auth/me` | 更新用户名/邮箱/简介 |
| POST | `/api/v2/auth/change-password` | 修改密码 |
| GET | `/api/v2/auth/sessions` | 登录会话列表 |
| DELETE | `/api/v2/auth/sessions/{id}` | 吊销某个会话 |
| POST | `/api/v2/auth/2fa/setup` | 开启 2FA ⚠️ 尚未实现 TOTP 校验 |
| POST | `/api/v2/auth/2fa/disable` | 关闭 2FA |
| GET | `/api/v2/users/me/quota` | 配额与用量 |
| POST | `/api/v2/users/me/quota/recalculate` | 按实际文件重算用量 |
| GET | `/api/v2/users/search?q=` | 搜索用户（用于团队邀请） |
| GET | `/api/v2/notifications` | 通知列表 |
| PUT | `/api/v2/notifications/{id}/read` | 标记已读 |

---

## 文件

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v2/files` | 列表（`page`/`per_page`/`folder_id`/`file_type`/`sort_by`/`sort_order`/`is_favorite`/`category`/`tag_id`） |
| GET | `/api/v2/files/{id}` | 详情 |
| PATCH | `/api/v2/files/{id}` | 部分更新（文件名/描述/收藏/类型/分类/排序） |
| DELETE | `/api/v2/files/{id}` | 软删除（进回收站） |
| PUT | `/api/v2/files/{id}/rename` | 重命名 |
| PUT | `/api/v2/files/{id}/move` | 移动到文件夹（目标必须属于自己） |
| PUT | `/api/v2/files/{id}/favorite` | 切换收藏 |
| PUT | `/api/v2/files/{id}/content` | 覆盖内容（multipart，生成新版本） |
| POST | `/api/v2/files/{id}/to-vault` | 复制/移动到加密空间 |
| GET | `/api/v2/files/{id}/download` | 下载（支持 `Range` 断点续传） |
| GET | `/api/v2/files/download/{id}` | 同上（前端使用的别名） |
| GET | `/api/v2/files/preview/{id}/{filename}` | 内联预览（需认证；`{filename}` 仅用于 Content-Disposition） |
| GET | `/api/v2/files/search?q=` | 按文件名搜索 |
| POST | `/api/v2/files/{id}/comments` | 添加评论 |
| GET | `/api/v2/files/{id}/comments` | 评论列表 |
| DELETE | `/api/v2/files/comments/{commentId}` | 删除评论 |

**文件夹**

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v2/files/folders/tree` | 完整文件夹树 |
| GET | `/api/v2/files/folders/by-path?path=` | 按路径取子项（含面包屑） |
| POST | `/api/v2/files/folders/by-path` | 按路径建文件夹 |
| GET | `/api/v2/files/folders/{id}/children` | 按 ID 取子项 |
| POST | `/api/v2/files/folders` | 建文件夹 |
| PATCH | `/api/v2/files/folders/{id}` | 重命名文件夹 |
| DELETE | `/api/v2/files/folders/{id}` | 删除文件夹 |

**标签**

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/api/v2/files/tags` | 列出/创建标签 |
| PATCH/DELETE | `/api/v2/files/tags/{id}` | 更新/删除标签 |
| GET/POST | `/api/v2/files/{id}/tags` | 列出/替换文件的标签 |
| DELETE | `/api/v2/files/{id}/tags/{tagId}` | 移除单个标签 |

---

## 上传

**单文件**

```
POST /api/v2/files/upload        multipart: file, folder_id?
```

**分片上传**（前端 `file.size > 10 MiB` 时使用）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v2/files/upload/init` | 初始化（JSON 或表单：`filename`/`file_size`/`chunk_size`/`total_chunks`/`folder_id`），返回 `upload_id` 与 `received_chunks` |
| POST | `/api/v2/files/upload/chunk` | 上传分片（multipart：`upload_id`/`chunk_index`/`chunk`） |
| PUT | `/api/v2/files/upload/{uploadId}/chunk/{index}` | 上传分片（原始 body，Content-Type 非 multipart 时） |
| GET | `/api/v2/files/upload/{uploadId}/status` | 已收分片与进度 |
| POST | `/api/v2/files/upload/complete` | 合并分片并落库（JSON：`upload_id`/`folder_id`） |
| DELETE | `/api/v2/files/upload/{uploadId}` | 取消上传（清理临时文件） |
| GET | `/api/v2/files/upload/check?file_hash=` | 秒传检查（内容是否已存在） |

**同步引擎（`.Stora` 客户端）**

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v2/sync/changes?since=&limit=` | 增量变更（含已删除） |
| POST | `/api/v2/sync/upload` | 按相对路径上传并自动建目录 |
| POST | `/api/v2/sync/upload/complete` | 合并分片 + 建目录 + 建文件（一次调用） |
| POST | `/api/v2/sync/upload/assign` | 把已上传的分片会话指派到目标路径 |
| POST | `/api/v2/sync/upload/session` | 创建 OneDrive 风格上传会话 |
| PUT | `/api/v2/sync/upload/session/{sessionId}` | 上传片段（需 `Content-Range`） |
| GET | `/api/v2/sync/upload/session/{sessionId}` | 会话进度 |
| POST | `/api/v2/blocks/upload` | 上传单个块（≤16 MiB） |
| GET | `/api/v2/blocks/{hash}` | 按内容哈希取块 |
| GET | `/api/v2/files/{id}/manifest` | 文件的分块清单 |
| GET | `/api/v2/files/{id}/snapshots` | 版本快照 |

---

## 分享链接

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v2/files/shares` | 我的分享列表 |
| POST | `/api/v2/files/shares` | 创建（`file_id` 或 `folder_id` 或 `file_ids[]`/`folder_ids[]`，可设 `password`/`expires_in_hours`/`max_downloads`/`permission`） |
| PUT | `/api/v2/files/shares/{id}` | 更新（权限/密码/有效期/下载上限/启停） |
| DELETE | `/api/v2/files/shares/{id}` | 吊销 |
| POST | `/api/v2/files/shares/share-with-user` | 直接分享给站内用户 |

> 旧路径 `/api/v2/share/links` 仍可用（别名）。

---

## 团队

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/api/v2/teams` | 我的团队 / 创建团队（创建者为 admin 成员） |
| DELETE | `/api/v2/teams/{teamId}` | 仅所有者可删 |
| GET/POST | `/api/v2/teams/{teamId}/members` | 成员列表 / 添加成员（需 admin 或所有者） |
| DELETE | `/api/v2/teams/{teamId}/members/{memberId}` | 移除成员 |
| GET/POST | `/api/v2/teams/{teamId}/folders` | 团队文件夹 / 添加（需 admin 或所有者） |
| DELETE | `/api/v2/teams/{teamId}/folders/{folderId}` | 移除团队文件夹 |

---

## 加密空间（Vault）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/api/v2/vaults` | 列表 / 创建（创建需设密码，返回解锁 token） |
| POST | `/api/v2/vaults/{vaultId}/verify-password` | 校验密码并换取解锁 token（有效期 30 分钟） |
| DELETE | `/api/v2/vaults/{vaultId}` | 删除保险库及其文件 |
| GET | `/api/v2/vaults/{vaultId}/items` | 条目列表（需 `vault_token`） |
| GET | `/api/v2/vaults/{vaultId}/items/{itemId}` | 下载并解密 |
| DELETE | `/api/v2/vaults/{vaultId}/items/{itemId}` | 删除条目 |

> 解锁 token 通过 `?vault_token=` 或 `X-Vault-Token` 头传递。
> **注意**：当前为服务端持钥的 AES-256-GCM，不是端到端加密（见 `docs/design-next-phase.md`）。

---

## 媒体处理、版本与回收站

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v2/files/transcode/{id}` | 开始转码（`target_format` 受白名单约束） |
| GET | `/api/v2/files/transcode/{id}/tasks` | 转码任务 |
| POST | `/api/v2/files/transcribe/{id}` | 开始语音转写（Whisper） |
| GET | `/api/v2/files/transcribe/{id}/status` | 转写状态 |
| GET | `/api/v2/files/transcribe/{id}/subtitle` | 取 SRT 字幕 |
| GET | `/api/v2/files/{id}/versions` | 版本列表 |
| POST | `/api/v2/files/{id}/versions/{versionId}/restore` | 回滚到某版本 |
| GET | `/api/v2/files/trash` | 回收站列表 |
| POST | `/api/v2/files/trash/{id}/restore` | 恢复 |
| POST | `/api/v2/files/trash/{id}/destroy` | 彻底删除 |
| POST | `/api/v2/files/trash/batch-destroy` | 批量彻底删除 |
| POST | `/api/v2/files/trash/batch-restore` | 批量恢复 |
| POST | `/api/v2/files/trash/clear` | 清空回收站 |

**批量操作**

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v2/files/batch/delete` | 批量删除 |
| POST | `/api/v2/files/batch/move` | 批量移动 |
| POST | `/api/v2/files/batch/copy` | 批量复制（≤500 个） |
| POST | `/api/v2/files/batch/tags` | 批量设置标签 |
| POST | `/api/v2/files/batch/category` | 批量设置分类 |
| POST | `/api/v2/files/download/batch` | 打包下载（ZIP 流） |

---

## 离线下载与 Webhook

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v2/files/offline-download` | 创建离线下载（HTTP/HTTPS 或 magnet；月上限 10 次） |
| GET | `/api/v2/files/offline-download` | 任务列表 |
| GET | `/api/v2/files/offline-download/{id}` | 任务详情 |
| POST | `/api/v2/files/offline-download/{id}/retry` | 重试失败任务（上限 5 次） |
| GET/POST | `/api/v2/webhooks` | 列出 / 创建 Webhook（URL 必须是公网地址） |
| DELETE | `/api/v2/webhooks/{id}` | 删除 |

---

## 管理端（需 superuser）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v2/admin/dashboard` | 统计概览 |
| GET | `/api/v2/admin/users` | 用户列表 |
| PUT | `/api/v2/admin/users/{id}` | 启用/禁用、超管、配额 |
| GET/POST | `/api/v2/admin/roles` | 角色列表 / 创建 |
| GET/PUT | `/api/v2/admin/settings` | 系统设置 |
| GET/PUT | `/api/v2/admin/maintenance` | 维护模式 |
| POST | `/api/v2/notifications` | 全站通知 |
| GET | `/api/v2/admin/audit-logs` | 审计日志 |
| GET | `/api/v2/admin/sensitive-words` | 敏感词列表 |

**管理 UI**（Go 模板，需登录 + 超管）：`/admin/ui`、`/admin/ui/migrate`

---

## WOPI（Collabora Online）

| 方法 | 路径 | 认证 | 说明 |
|---|---|---|---|
| GET | `/api/v2/wopi/access/{fileId}` | Stora 会话 | 换取编辑器 URL（内含短期 JWT） |
| GET | `/api/v2/wopi/files/{fileId}` | access_token | CheckFileInfo |
| GET | `/api/v2/wopi/files/{fileId}/contents` | access_token | 取文件内容 |
| POST | `/api/v2/wopi/files/{fileId}/contents` | access_token | 保存内容（生成新版本） |

详见 `docs/collabora-deploy.md`。

---

## Mobile（v3）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v3/auth/login` | 移动端登录（JSON，返回 token 对） |
