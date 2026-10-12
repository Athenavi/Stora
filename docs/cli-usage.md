# Stora CLI 使用说明

系统管理命令行工具。服务端运行时**不需要**它；它用于部署、迁移与运维。

```bash
go build -o stora-cli ./cmd/cli
./stora-cli <command> [args...]
```

所有命令都读取 `.env`（通过 `pkg/config` 的同一套环境变量），因此请在项目根目录或已配置环境变量的位置执行。

---

## 命令总览

| 命令 | 说明 |
|---|---|
| `init` | ⚡ 交互式初始化向导（推荐首次部署使用） |
| `migrate` | 数据库迁移管理（generate / up / down / history / current） |
| `upgrade` | 版本升级（check / apply） |
| `config` | 配置管理（show / init / verify） |
| `health` | 检查数据库连通性 |
| `users` | 列出用户 |
| `backup` | 数据库备份 |
| `serve` | 启动管理 Web UI |

不带参数运行会打印命令列表。

---

## init — 初始化向导

```bash
./stora-cli init
```

按顺序执行：检查依赖（Go、PostgreSQL、Redis 可用性）→ 交互式收集配置 → 写入 `.env` →
从 `config/models.yaml` 生成并应用初始 schema → 创建管理员账户 → 打印摘要。

适合首次部署；已初始化的环境请改用 `migrate`。

---

## migrate — 数据库迁移

迁移文件位于 `migrations/`，命名格式为 `YYYYMMDD_HHMMSS_描述.sql`，文件内用
`-- UPGRADE` / `-- DOWNGRADE` 两个标记分段；没有标记时整个文件视为 UPGRADE。

已应用的迁移记录在 `schema_version` 表（`revision` 为主键），**每个迁移在单个事务中执行**，
失败会回滚。

| 子命令 | 说明 |
|---|---|
| `migrate generate` | 读取 `config/models.yaml`，生成一个包含 UPGRADE + DOWNGRADE 的迁移文件 |
| `migrate up` | 按 revision 升序应用所有未执行的迁移 |
| `migrate down [n]` | 回退最近 n 个迁移（默认 1）。回退会**先删除 `schema_version` 记录再执行 DOWNGRADE SQL**，以便 DOWNGRADE 能安全地删表 |
| `migrate history`（别名 `log`） | 列出迁移历史与应用状态 |
| `migrate current` | 显示当前 revision（`version.ini` 与数据库各一份） |

```bash
./stora-cli migrate generate          # 改完 models.yaml 后生成迁移
./stora-cli migrate up                # 应用
./stora-cli migrate history           # 查看状态
./stora-cli migrate down 2            # 回退两步
```

> 声明式工作流的顺序是：改 `config/models.yaml` → `migrate generate` → 人工检查生成的 SQL → `migrate up`。
> 手写迁移同样支持，放在 `migrations/` 下即可。

---

## upgrade — 版本升级

| 子命令 | 说明 |
|---|---|
| `upgrade check` | 检查是否有新版本可用 |
| `upgrade apply [--version <v>]` | 应用升级（迁移 + 数据调整） |
| `upgrade --version <v>` | 显示指定版本信息 |

---

## config — 配置管理

| 子命令 | 说明 |
|---|---|
| `config show` | 显示当前生效的配置（密钥以 `****` 掩码） |
| `config init` | 生成 `.env` 模板 |
| `config verify` | 校验配置完整性（如 `SECRET_KEY` 是否存在） |

---

## health / users / backup / serve

```bash
./stora-cli health          # 打印数据库连接状态
./stora-cli users           # 列出用户
./stora-cli backup          # 备份数据库
./stora-cli serve           # 启动管理 Web UI
```

`serve` 提供的是与主服务同一套 Go 模板管理页面；生产环境通常直接使用主服务的
`/admin/ui`（需登录且为超级用户）。

---

## 相关文档

- 环境变量清单：`.env.example`
- 对象存储（RustFS）：`docs/deployment-rustfs.md`
- 反向代理部署：`docs/deployment-nginx.md`
