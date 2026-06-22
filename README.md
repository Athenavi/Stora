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

Stora CLI 是系统管理命令行工具，支持初始化、迁移、升级、备份等操作。

```bash
# 构建
go build -o stora-cli cmd/cli/main.go

# 或直接运行
go run ./cmd/cli <command>
```

### 命令一览

| 命令 | 说明 | 适用场景 |
|------|------|----------|
| `init` | ⚡ 系统初始化向导 | **小白首选** — 交互式完成环境检查、配置生成、建表、创建管理员 |
| `migrate` | 数据库版本迁移管理 | 开发/运维 — 基于 YAML 声明式 schema |
| `upgrade` | 系统升级 + 版本管理 | 生产环境 — 检查/应用 GitHub Release 更新 |
| `config` | 配置管理 | 生成/验证 .env 文件 |
| `backup` | 数据库备份 | 生产环境 |
| `health` | 系统健康检查 | 快速诊断 |
| `users` | 用户管理 | 列出所有用户 |
| `serve` | 启动管理 Web UI | 浏览器访问 `/admin/ui/` |

### 详细用法

#### `stora-cli init` — 系统初始化

5 步交互式向导，适合首次部署：

```
Step 1: 环境检查   → 检测 Go/PostgreSQL 是否安装
Step 2: 配置生成   → 交互填写 .env（密钥自动生成）
Step 3: 数据库建表  → 从 config/models.yaml 自动建表
Step 4: 创建管理员  → 自定义用户名/密码（或随机生成）
Step 5: 完成摘要   → 显示管理地址和登录凭据
```

#### `stora-cli migrate` — 数据库迁移（Alembic 风格）

工作流：编辑 `config/models.yaml` → 生成迁移 → 执行迁移

```bash
# 从 config/models.yaml 生成迁移（UPGRADE + DOWNGRADE 双段）
stora-cli migrate generate --description "add_tags_table"

# 执行待迁移
stora-cli migrate up

# 回退 1 个迁移
stora-cli migrate down

# 回退 N 个迁移
stora-cli migrate down 3

# 列出迁移历史
stora-cli migrate history

# 显示当前 revision
stora-cli migrate current
```

迁移文件格式（`migrations/` 目录）：
```sql
-- UPGRADE
CREATE TABLE IF NOT EXISTS tags (...);

-- DOWNGRADE
DROP TABLE IF EXISTS tags CASCADE;
```

#### `stora-cli upgrade` — 系统升级

```bash
# 显示当前版本
stora-cli upgrade --version

# 检查 GitHub 新版本
stora-cli upgrade check

# 应用更新（迁移 + 版本文件更新）
stora-cli upgrade apply

# 仅执行数据库迁移（向后兼容）
stora-cli upgrade --dry-run
```

版本定义在 `version.ini` 的 `[stora]` 段：
```ini
[stora]
version = V0.1.260622.01
release_date = 2026-06-22
channel = stable
```

#### `stora-cli config` — 配置管理

```bash
# 显示当前配置摘要
stora-cli config show

# 从 .env.example 生成 .env
stora-cli config init

# 验证配置完整性
stora-cli config verify
```

#### `stora-cli backup` — 数据库备份

```bash
# 自动命名备份文件
stora-cli backup

# 指定输出文件
stora-cli backup -o mybackup.sql
```

#### `stora-cli health` — 健康检查

```bash
stora-cli health
# ✅ 数据库: 已连接
# ✅ 系统: 运行正常
```

#### `stora-cli users` — 用户管理

```bash
stora-cli users
# ID    用户名    邮箱    管理员  状态
# --    ------    ----    ----   ----
# 1     admin     ...     ✓      ✓
```

## License

Apache-2.0
