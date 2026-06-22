# Stora

**Self-hosted enterprise file storage and sharing platform — now in Go.**

> **Active backend:** Go (Chi + sqlx + PostgreSQL)  
> **Frontend:** Qwik  
> **Previous backend:** Python/FastAPI (archived)

---

## Architecture

```
cmd/
  server/          # Main server entry point (port 9421)
  cli/             # CLI commands
internal/
  api/
    v2/
      auth/        # JWT auth, OAuth, verification code login
      files/       # File CRUD, upload, download, search, tags, folders
      share/       # Share links, user-to-user sharing
      admin/       # User/role management, system settings, audit logs
    v3/mobile/     # Mobile-optimized endpoints
  middleware/       # Auth, RBAC, CORS, security middleware
  services/        # Business logic layer
pkg/
  config/          # Environment-based configuration (viper)
  database/        # PostgreSQL + Redis connection pools
  models/          # Data models (Go structs, 33 tables)
  auth/            # JWT tokens, bcrypt passwords
  storage/         # Local filesystem + S3/MinIO adapters
  cache/           # Multi-level caching
  utils/           # Shared utilities
```

## Quick Start

```bash
# Prerequisites: Go 1.26+, PostgreSQL 15+, Redis 7+

# Clone and configure
cp .env.example .env
# Edit .env with your database credentials (SECRET_KEY is required)

# Run (from project root)
go run ./cmd/server

# Server starts at http://localhost:9421
# Health check: http://localhost:9421/api/health
```

## Deployment

> **🚀 Production deployment** → [Nginx 反向代理部署指南](docs/deployment-nginx.md)
>
> 涵盖 HTTPS 强制、Cookie 安全加固、CORS 生产配置、速率限制、完整配置示例及安全检查清单。

- [部署指南（完整版）](docs/deployment-nginx.md)
- [功能差距分析](docs/gap-analysis.md)
- [分享功能分析](docs/share-analysis.md)
- [Collabora Online 集成](docs/wopi-collabora.md)

## API Endpoints

### Public
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check (DB connectivity) |
| GET | `/api/health/live` | Liveness probe |
| GET | `/api/v2` | API version info |
| POST | `/api/v2/auth/register` | User registration |
| POST | `/api/v2/auth/login` | Login (password or email) |
| POST | `/api/v2/auth/refresh` | Refresh access token |
| POST | `/api/v2/auth/send-code` | Send verification code |
| POST | `/api/v2/auth/login-with-code` | Login with verification code |
| GET | `/api/v2/share/{token}` | Access shared file (public) |

### Authenticated
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v2/auth/me` | Current user profile |
| POST | `/api/v2/auth/logout` | Logout |
| POST | `/api/v2/auth/2fa/setup` | Enable 2FA |
| POST | `/api/v2/auth/2fa/disable` | Disable 2FA |
| GET | `/api/v2/files` | List files (paginated, filterable) |
| GET | `/api/v2/files/{id}` | Get file details |
| POST | `/api/v2/files/upload` | Upload file |
| DELETE | `/api/v2/files/{id}` | Soft-delete file |
| PUT | `/api/v2/files/{id}/rename` | Rename file |
| PUT | `/api/v2/files/{id}/favorite` | Toggle favorite |
| PUT | `/api/v2/files/{id}/move` | Move to folder |
| GET | `/api/v2/files/{id}/download` | Download file |
| GET | `/api/v2/files/search` | Search files |
| POST | `/api/v2/files/{id}/transcode` | Start video transcoding |
| GET | `/api/v2/files/{id}/versions` | List file versions |
| POST | `/api/v2/files/batch/delete` | Batch delete |
| POST | `/api/v2/files/batch/move` | Batch move |
| GET | `/api/v2/files/folders/tree` | Folder tree |
| POST | `/api/v2/files/folders` | Create folder |
| DELETE | `/api/v2/files/folders/{id}` | Delete folder |
| GET | `/api/v2/files/tags` | List tags |
| POST | `/api/v2/files/tags` | Create tag |
| GET | `/api/v2/vaults` | List vaults |
| POST | `/api/v2/vaults` | Create vault |
| GET | `/api/v2/vaults/{id}/items` | List vault items |
| POST | `/api/v2/vaults/{id}/items` | Create vault item (AES-256 encrypted) |
| GET | `/api/v2/trash` | List trash |
| POST | `/api/v2/trash/{id}/restore` | Restore from trash |
| POST | `/api/v2/trash/empty` | Empty trash |
| GET | `/api/v2/share/links` | List share links |
| POST | `/api/v2/share/links` | Create share link |
| DELETE | `/api/v2/share/links/{id}` | Delete share link |
| GET | `/api/v2/notifications` | List notifications |
| PUT | `/api/v2/notifications/{id}/read` | Mark notification read |

### Admin (superuser required)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v2/admin/users` | List all users |
| PUT | `/api/v2/admin/users/{id}` | Update user |
| GET | `/api/v2/admin/roles` | List roles |
| POST | `/api/v2/admin/roles` | Create role |
| GET | `/api/v2/admin/settings` | List system settings |
| PUT | `/api/v2/admin/settings` | Update setting |
| GET | `/api/v2/admin/audit-logs` | View audit logs |
| GET | `/api/v2/admin/sensitive-words` | List sensitive words |

### Mobile (v3)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v3/auth/login` | Mobile login |

## Database

**33 tables** covering:
- **Users & Auth:** `users`, `user_sessions`, `user_blocks`, `login_attempts`, `token_blacklist`
- **Files:** `file_items`, `folders`, `file_versions`, `file_tags`, `file_tag_assignments`, `file_fingerprints`, `file_optimizations`, `access_logs`, `trash_items`, `upload_tasks`, `upload_chunks`, `download_tokens`, `download_tasks`
- **Security:** `audit_logs`, `sensitive_words`, `gdpr_consents`, `field_permissions`
- **Sharing:** `share_links`, `file_shares`
- **RBAC:** `roles`, `capabilities`, `role_capabilities`, `user_role_assignments`, `permission_audit_logs`
- **System:** `system_settings`, `notifications`, `search_history`, `storage_quotas`
- **Vault:** `vaults`, `vault_items`, `transcode_tasks`, `transcription_tasks`, `storage_plans`

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.26 |
| HTTP Router | Chi v5 |
| Database | PostgreSQL 15+ (sqlx) |
| Cache | Redis 7+ (go-redis) |
| Auth | JWT (golang-jwt), bcrypt |
| Storage | Local FS / S3 / MinIO (minio-go) |
| Config | Viper + .env |
| Encryption | AES-256-GCM (vault) |
| Transcoding | FFmpeg |

## CLI

Stora CLI 是系统管理命令行工具。

```bash
go build -o stora-cli cmd/cli/main.go
```

| 命令 | 说明 |
|------|------|
| `init` | ⚡ 系统初始化向导（交互式，小白首选） |
| `migrate` | 数据库迁移（generate/up/down/history/current） |
| `upgrade` | 系统升级 + 版本管理（check/apply/--version） |
| `config` | 配置管理（show/init/verify） |
| `backup` | 数据库备份 |
| `health` | 系统健康检查 |
| `users` | 用户列表 |

详情见 `docs/cli-usage.md`

## API

完整 API 文档见 [docs/api.md](docs/api.md)。主要端点：

| 分组 | 端点 |
|------|------|
| 认证 | `/api/v2/auth/*` — 登录/注册/2FA/会话管理 |
| 文件 | `/api/v2/files/*` — CRUD/上传/下载/搜索/评论/转码 |
| 分享 | `/api/v2/share/*` — 链接分享/密码/有效期/收集/转存 |
| 团队 | `/api/v2/teams/*` — 团队/成员/共享文件夹 |
| 管理 | `/api/v2/admin/*` — 用户/设置/审计/维护模式 |
| Webhook | `/api/v2/webhooks/*` — 事件通知 |

## 快速开始

```bash
cp .env.example .env
# 编辑 .env，设置 SECRET_KEY 和数据库连接

# 方式 1: 交互式初始化（推荐）
go run ./cmd/cli init

# 方式 2: 手动
go run ./cmd/server
# 访问 http://localhost:9421/admin/ui/
```

## 部署

> 生产部署指南：`docs/deployment-nginx.md`
> OAuth 配置：`docs/oauth-setup.md`



## License

Apache-2.0
