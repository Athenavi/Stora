# Stora — Store everything. Recall everything.

> 存你所存，想你所想。

**Stora** 是一个高性能、自部署的网盘应用，基于 FastAPI + Qwik 构建。

## 技术栈

| 分层 | 技术 |
|------|------|
| **后端** | Python 3.14+ · FastAPI · SQLAlchemy 2.0 · PostgreSQL |
| **前端** | Qwik · Tailwind CSS v4 · TypeScript |
| **存储** | 本地存储 / S3 兼容 (MinIO, S3, R2) |
| **搜索** | Meilisearch（可选） |
| **缓存** | Redis（可选） |

## 快速开始

### 前置要求

- Python 3.14+
- PostgreSQL 16+
- Node.js 20+

### 1. 克隆并安装后端

```bash
git clone https://github.com/athenavi/stora.git
cd stora

python -m venv .venv
.venv\Scripts\activate   # Windows
# source .venv/bin/activate  # Linux/Mac

pip install -r requirements.txt
```

### 2. 配置数据库

```bash
# 创建数据库
createdb stora

# 配置环境变量
cp .env.example .env
# 编辑 .env 设置数据库连接
```

### 3. 运行迁移

```bash
alembic upgrade head
```

### 4. 启动后端

```bash
uvicorn main:app --reload --port 8000
```

### 5. 启动前端

```bash
cd frontend-qwik
npm install
npm run dev
```

## API 文档

启动后端后访问 `http://localhost:8000/docs` 查看交互式 API 文档。

### 核心端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v2/files` | GET | 文件列表（分页/搜索/筛选） |
| `/api/v2/files/upload` | POST | 单文件上传（支持秒传） |
| `/api/v2/files/upload/init` | POST | 初始化分片上传 |
| `/api/v2/files/upload/chunk` | POST | 上传分片 |
| `/api/v2/files/upload/complete` | POST | 合并分片 |
| `/api/v2/files/folders` | POST | 创建文件夹 |
| `/api/v2/files/folders/tree` | GET | 文件夹树 |
| `/api/v2/files/{id}/download` | GET | 下载文件 |
| `/api/v2/files/search` | GET | 搜索文件 |
| `/api/v2/files/shares` | POST | 创建分享链接 |
| `/api/v2/auth/login` | POST | 登录 |
| `/api/v2/auth/register` | POST | 注册 |

## 核心功能

### ✅ 已完成

- [x] 文件/文件夹 CRUD
- [x] 单文件上传 + 分片上传
- [x] 文件秒传（SHA256 去重）
- [x] 文件下载（含一次性令牌）
- [x] 文件夹树形导航
- [x] 文件搜索
- [x] 分享链接（密码保护/过期时间/下载限制）
- [x] 用户认证（JWT）
- [x] RBAC 角色权限
- [x] 双因素认证 (2FA)
- [x] 存储配额管理
- [x] 回收站
- [x] 文件版本历史
- [x] 文件标签
- [x] 审计日志
- [x] 操作通知
- [x] GDPR 合规

### 🚧 开发中

- [ ] 文件预览（图片/视频/文档/音频）
- [ ] S3/MinIO 存储驱动
- [ ] 在线文档编辑
- [ ] WebDAV 协议支持
- [ ] 移动客户端

## 项目结构

```
stora/
├── config/               # 配置 (models.yaml)
├── shared/               # 共享层
│   ├── models/           # SQLAlchemy ORM 模型 (33 个)
│   ├── services/         # 业务服务 (12 个模块)
│   └── defs/             # 自定义方法定义
├── src/                  # 后端源码
│   ├── api/v2/           # API v2 路由 (16 个模块)
│   ├── api/v3/           # API v3 移动端路由
│   ├── auth/             # 认证模块
│   └── middleware/        # 中间件
├── frontend-qwik/        # Qwik 前端
├── scripts/              # 工具脚本
├── alembic_migrations/   # 数据库迁移
└── main.py               # 应用入口
```

## 许可证

Apache 2.0

---

> **Stora** 由 [FastBlog](https://github.com/Athenavi/fast_blog) v0.5.26.0617 重构而来。
> 源仓库: [https://github.com/Athenavi/fast_blog/releases/tag/V0.5.26.0617](https://github.com/Athenavi/fast_blog/releases/tag/V0.5.26.0617)
