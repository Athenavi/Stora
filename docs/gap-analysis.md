# Stora 差距分析（剩余差距）

> 最后更新：2026-06-22  
> **最新完成：** AI 字幕 (Whisper)、Office 在线编辑 (WOPI)、视频转码、Webhook、BT 下载、Admin UI

---

## 全部差距已完成

### 1. Office 在线编辑（P1）
**状态：✅ 已完成**
- WOPI 接口端点：`/api/v2/wopi/files/{fileId}`（`CheckFileInfo` / `GetFile` / `PutFile`）
- WOPI 访问端点：`/api/v2/wopi/access/{fileId}`（返回 Collabora 编辑 URL）
- 部署文档：`docs/collabora-deploy.md`
- 前端"在 Office 中编辑"按钮已集成

### 2. 视频转码（P1）
**状态：✅ 已完成**
- `TranscodeHandler` 含 `StartTranscode` + `ListTranscodeTasks`
- 后台 goroutine 调用 ffmpeg 转码
- 转码完成通知
- 前端转码状态展示

### 3. 协作编辑（P2，10h）
**状态：⏸️ 搁置** — 需要 Operational Transformation / CRDT 算法，待后续版本实现。
- WebSocket 端点 + 房间管理
- 操作同步 + 冲突解决
- 前端编辑器集成

### 4. AI 字幕（P2）
**状态：✅ 已完成**
- `TranscribeHandler`：`StartTranscription` / `GetTranscriptionStatus` / `GetSubtitleFile`
- 后台 goroutine：ffmpeg 提取音频 → Whisper CLI 识别 → 生成 SRT
- 前端字幕切换按钮 + `<track>` 标签嵌入
- 降级方案：whisper 未安装时生成占位提示字幕

### 5. Webhook（P2）
**状态：✅ 已完成**
- `WebhookHandler`：List / Create / Delete
- `webhooks` + `webhook_events` 表已定义
- 路由：`/api/v2/webhooks`

### 6. BT 离线下载（P2）
**状态：✅ 已完成**
- `OfflineDownloadHandler`：CreateDownloadTask / ListDownloadTasks
- 支持磁力链接 / HTTP / FTP 下载

### 7. Admin UI 完善（P1）
**状态：✅ 已完成**
- Admin API：用户管理、角色权限、系统设置、维护模式、审计日志、敏感词、通知
- Admin UI（Go 模板）：Dashboard + 迁移管理页面
- 前端管理页面（Qwik）：用户管理、系统设置

### 8. IP 白名单（P3）
**状态：✅ 已完成** — `internal/middleware/ipwhitelist.go`

### 9. OAuth 完善（P2）
**状态：✅ 已完成** — `docs/oauth-setup.md`

### 10. 图片编辑（P1）
**状态：✅ 已完成** — `ImageEditor.tsx` + `PUT /files/{id}/content`

---

## 建议后续工作

| 项目 | 优先级 | 说明 |
|------|--------|------|
| 协作编辑 | P2 | CRDT/OT 实时协作，较复杂 |
| 完善测试覆盖 | P2 | 为新增 handler 编写测试 |
| 部署文档 | P2 | 整合 Docker 部署指南 |
