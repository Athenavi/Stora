# Stora 差距分析（剩余差距）

> 最后更新：2026-06-22  
> **最新完成：** 视频转码 (ffmpeg 后台)、Webhook 系统、IP 白名单、OAuth 文档、配额预警、水印

---

## 剩余 4 项差距

### 1. Office 在线编辑（P1，6h）
**目标**：通过 Collabora Online 实现 Office 文档在线编辑
**现状**：WOPI 接口端点已就绪（`/api/v2/wopi/`），需部署配套服务
**工作量**：
- 编写 Collabora Online Docker Compose 部署文档（2h）
- 编写 nginx 反向代理配置（1h）
- 验证 WOPI 接口与 Collabora 的对接（2h）
- 前端"在 Office 中编辑"按钮集成（1h）

### 2. 视频转码（P1，8h）
**目标**：上传视频后自动 ffmpeg 转码为多分辨率
**现状**：`transcode_tasks` 表存在，`ffmpeg` 工具未对接
**工作项**：
- 检测服务器是否安装 ffmpeg（1h）
- 实现 `StartTranscode` 后台处理 goroutine（3h）
- 转码进度更新 + 完成通知（2h）
- 前端转码状态展示（2h）

### 3. 协作编辑（P2，10h）
**目标**：多人同时编辑同一文本/文档
**难点**：需要 Operational Transformation 或 CRDT 算法
**方案**：简化版 — 基于 WebSocket 的协作编辑，类似 Etherpad
- WebSocket 端点 + 房间管理（4h）
- 操作同步 + 冲突解决（4h）
- 前端编辑器集成 CodeMirror（2h）

### 4. AI 字幕（P2，6h）
**目标**：视频上传后自动生成 SRT 字幕
**方案**：对接 Whisper（本地或 API）
- 检测 whisper 或调用云端 API（1h）
- 音频提取 + 识别任务队列（3h）
- SRT 生成 + 前端展示（2h）

### 5. Webhook（P2，6h）
**目标**：文件创建/删除/分享时通知外部系统
**方案**：Webhook 表 + 事件触发 + HTTP POST
- `webhooks` 表定义（1h）→ 需在 models.yaml 添加
- 事件触发中间件（2h）
- 重试 + 日志（2h）
- 前端配置页面（1h）

### 6. BT 离线下载（P2，6h）
**目标**：支持 BitTorrent 磁力链接下载
**方案**：对接 anacrolix/torrent Go 库
- 集成 torrent 库（2h）
- 磁力链接解析 + 下载任务管理（2h）
- 下载进度 + 完成通知（2h）

### 7. Admin UI 完善（P1，8h）
**目标**：从 Go 模板升级为功能完整的管理后台
**方案**：在现有 Go 模板基础上增加页面
- 用户管理页面（列表/编辑/禁用）（2h）
- 系统设置页面（2h）
- 维护模式切换 UI（2h）
- 审计日志查看（2h）

### 8. IP 白名单（P3）
**状态：✅ 已完成** — `internal/middleware/ipwhitelist.go` 实现，通过 `system_settings.admin_ip_whitelist` 配置逗号分隔 IP 列表。

### 9. OAuth 完善（P2）
**状态：✅ 已完成** — 配置文档 `docs/oauth-setup.md` 已编写，含 GitHub/Google 配置步骤及故障排除。

### 10. 图片编辑（P1）
**状态**：**已完成** — `ImageEditor.tsx` + `PUT /files/{id}/content` 已实现

---

## 实施批次建议

| 批次 | 项目 | 总工时 | 说明 |
|------|------|--------|------|
| **A** | Office 文档 + IP 白名单 | 10h | 快速交付，OAuth 文档同步完成 |
| **B** | 视频转码 + Webhook | 14h | 需 models.yaml 新增 webhooks 表 |
| **C** | AI 字幕 + BT 下载 | 12h | 需对接外部库（whisper/torrent） |
| **D** | 协作编辑 + Admin UI | 18h | 最大工作量，WebSocket + 模板增强 |

## 需 models.yaml 新增的表

```yaml
Webhook:
  table: webhooks
  columns:
    id:          { bigint, pk, autoincrement }
    user_id:     { bigint, fk: users(id), cascade }
    url:         { varchar, length: 512 }
    events:      { text }          # JSON array: ["file.create","file.delete"]
    is_active:   { boolean, default: "true" }
    created_at:  { timestamp, default: "now()" }

WebhookEvent:
  table: webhook_events
  columns:
    id:          { bigint, pk, autoincrement }
    webhook_id:  { bigint, fk: webhooks(id), cascade }
    event:       { varchar, length: 50 }
    payload:     { text }
    status:      { varchar, length: 20, default: "'pending'" }
    response:    { text, nullable: true }
    created_at:  { timestamp, default: "now()" }
```
