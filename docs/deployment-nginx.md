# Stora Nginx 反向代理部署指南

> 生产环境推荐使用 Nginx 作为反向代理，提供 HTTPS 终止、速率限制、CORS 加固等安全层。  
> **原则**：安全策略在反向代理层做（Nginx），而非应用层（Go），便于运维统一管理。

---

## 目录

1. [基础反向代理](#1-基础反向代理)
2. [HTTPS 强制](#2-https-强制)
3. [Cookie 安全加固](#3-cookie-安全加固)
4. [CORS 加固](#4-cors-加固)
5. [速率限制](#5-速率限制)
6. [完整配置示例](#6-完整配置示例)
7. [安全检查清单](#7-安全检查清单)

---

## 1. 基础反向代理

```nginx
server {
    listen 80;
    server_name stora.example.com;

    location / {
        proxy_pass http://127.0.0.1:9421;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 大文件上传支持
        client_max_body_size 10G;
        proxy_request_buffering off;
        proxy_buffering off;

        # WebSocket 支持（用于 MCP/通知等）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 超时（大文件上传/下载需要较长超时）
        proxy_connect_timeout 60s;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # 静态资源直接由 Nginx 服务
    location /assets/ {
        root /path/to/stora/static;
        expires 7d;
        add_header Cache-Control "public, immutable";
    }
}
```

---

## 2. HTTPS 强制

```nginx
server {
    listen 80;
    server_name stora.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name stora.example.com;

    ssl_certificate     /etc/letsencrypt/live/stora.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/stora.example.com/privkey.pem;

    # 现代 TLS 配置
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    # HSTS（告诉浏览器始终使用 HTTPS）
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    # 其余配置同基础反向代理 ...
}
```

---

## 3. Cookie 安全加固

Nginx 层为所有 Cookie 添加安全标志（覆盖应用层遗漏的情况）：

```nginx
# 在 server 或 location 块中添加
proxy_cookie_path / "/; HttpOnly; Secure; SameSite=Strict";
```

此指令为所有经 Nginx 转发的 Set-Cookie 响应头附加：
- `HttpOnly` — 禁止 JavaScript 访问
- `Secure` — 仅通过 HTTPS 发送
- `SameSite=Strict` — 防止 CSRF（同站请求才携带 cookie）

> **注意**：应用层已设置 `HttpOnly` 和 `SameSite=Lax`。Nginx 层作为兜底，确保即使应用层遗漏也能覆盖。

---

## 4. CORS 加固

### 4.1 应用层 CORS 配置

在 `.env` 中设置允许的域名：

```env
# 生产环境：限定具体域名，不要使用通配符
CORS_ORIGINS=https://stora.example.com,https://admin.example.com
```

应用层 `AllowedOrigins` 从环境变量读取，避免 `*` + `AllowCredentials: true` 的安全问题。

### 4.2 Nginx 层 CORS 头覆盖

若无法修改应用代码，可在 Nginx 层重写 CORS 头：

```nginx
location /api/ {
    # ... proxy_pass 配置 ...

    # 限定额外的 CORS 头（覆盖应用层的宽松设置）
    if ($request_method = 'OPTIONS') {
        add_header Access-Control-Allow-Origin "https://stora.example.com" always;
        add_header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, PATCH, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Accept, Authorization, Content-Type, X-CSRF-Token" always;
        add_header Access-Control-Allow-Credentials "true" always;
        add_header Access-Control-Max-Age "300" always;
        add_header Content-Length "0" always;
        return 204;
    }

    add_header Access-Control-Allow-Origin "https://stora.example.com" always;
    add_header Access-Control-Allow-Credentials "true" always;
}
```

> **推荐**：在应用层 `Config` 中增加 `CORSOrigins` 字段，运行时读取，避免在 Nginx 中硬编码。

---

## 5. 速率限制

保护登录、注册、公开分享下载等端点免受暴力破解和滥用。

### 5.1 基于 IP 的限流

```nginx
http {
    # 定义限流区域
    limit_req_zone $binary_remote_addr zone=login:10m rate=5r/m;
    limit_req_zone $binary_remote_addr zone=register:10m rate=2r/m;
    limit_req_zone $binary_remote_addr zone=share_dl:10m rate=30r/m;
    limit_req_zone $binary_remote_addr zone=api:10m rate=100r/s;

    # 突发流量缓存
    limit_req_zone $binary_remote_addr zone=login_burst:10m rate=5r/m;

    server {
        # 登录端点：每分钟最多 5 次
        location /api/v2/auth/login {
            limit_req zone=login burst=2 nodelay;
            proxy_pass http://127.0.0.1:9421;
        }

        # 注册端点：每分钟最多 2 次
        location /api/v2/auth/register {
            limit_req zone=register burst=1 nodelay;
            proxy_pass http://127.0.0.1:9421;
        }

        # 公开分享下载：每分钟最多 30 次
        location /api/v2/share/ {
            limit_req zone=share_dl burst=10 nodelay;
            proxy_pass http://127.0.0.1:9421;
        }

        # 一般 API：每秒最多 100 次
        location /api/ {
            limit_req zone=api burst=50 nodelay;
            proxy_pass http://127.0.0.1:9421;
        }
    }
}
```

### 5.2 速率限制被触发时的响应

当客户端超过速率限制时，Nginx 返回 `503 Service Unavailable`。可自定义：

```nginx
location /api/v2/auth/login {
    limit_req zone=login burst=2 nodelay;
    limit_req_status 429;
    # 自定义错误页面
    error_page 429 /429.json;
    proxy_pass http://127.0.0.1:9421;
}

# 在 server 块中
location = /429.json {
    default_type application/json;
    return 429 '{"success":false,"message":"请求过于频繁，请稍后再试"}';
}
```

---

## 6. 完整配置示例

一份可直接使用的最小安全配置，整合了上述所有要点：

```nginx
# /etc/nginx/sites-available/stora
upstream stora_backend {
    server 127.0.0.1:9421;
    keepalive 64;
}

# HTTP → HTTPS 重定向
server {
    listen 80;
    server_name stora.example.com;
    return 301 https://$server_name$request_uri;
}

# HTTPS 服务器
server {
    listen 443 ssl http2;
    server_name stora.example.com;

    # ── TLS ──
    ssl_certificate     /etc/letsencrypt/live/stora.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/stora.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    # ── 安全头 ──
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # ── Cookie 安全兜底 ──
    proxy_cookie_path / "/; HttpOnly; Secure; SameSite=Strict";

    # ── 请求大小（大文件上传） ──
    client_max_body_size 10G;

    # ── Gzip ──
    gzip on;
    gzip_types application/json text/plain text/css application/javascript;

    # ── 速率限制区域 ──
    limit_req_zone $binary_remote_addr zone=login:10m rate=5r/m;
    limit_req_zone $binary_remote_addr zone=register:10m rate=2r/m;
    limit_req_zone $binary_remote_addr zone=share_dl:10m rate=30r/m;
    limit_req_zone $binary_remote_addr zone=api:10m rate=100r/s;

    # ── 健康检查（不限速） ──
    location /api/health {
        proxy_pass http://stora_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # ── 登录（严格限速） ──
    location /api/v2/auth/login {
        limit_req zone=login burst=2 nodelay;
        limit_req_status 429;
        proxy_pass http://stora_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # ── 注册（严格限速） ──
    location /api/v2/auth/register {
        limit_req zone=register burst=1 nodelay;
        limit_req_status 429;
        proxy_pass http://stora_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # ── 公开分享（适度限速） ──
    location /api/v2/share/ {
        limit_req zone=share_dl burst=10 nodelay;
        limit_req_status 429;
        proxy_pass http://stora_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # ── 主 API（通用限速） ──
    location /api/ {
        limit_req zone=api burst=50 nodelay;
        limit_req_status 429;
        proxy_pass http://stora_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 大文件上传不缓冲
        proxy_request_buffering off;
        proxy_buffering off;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # ── 静态资源 ──
    location /assets/ {
        root /var/www/stora/static;
        expires 7d;
        add_header Cache-Control "public, immutable";
        access_log off;
    }

    # ── 管理页面 ──
    location /admin/ui/ {
        proxy_pass http://stora_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        # 管理页面可能需要较长时间
        proxy_read_timeout 120s;
    }

    # ── 日志 ——
    access_log /var/log/nginx/stora.access.log;
    error_log  /var/log/nginx/stora.error.log warn;
}
```

---

## 7. 安全检查清单

部署后逐个验证：

| # | 检查项 | 验证方法 |
|---|--------|---------|
| 1 | HTTPS 生效 | `curl -I https://stora.example.com` → 返回 200 |
| 2 | HTTP 强制跳转 | `curl -I http://stora.example.com` → 301 到 HTTPS |
| 3 | HSTS 头 | `curl -sI https://... \| grep Strict-Transport` |
| 4 | Cookie Secure 标志 | 浏览器 DevTools → Application → Cookies → Secure ✓ |
| 5 | CORS 限制 | 用 `Origin: https://evil.com` 发请求 → 应被阻止 |
| 6 | 登录限流 | 快速连续登录 6 次 → 第 6 次返回 429 |
| 7 | 大文件上传 | 上传 1GB 文件 → 成功完成，无超时 |
| 8 | SSL 评级 | 访问 [SSLLabs](https://www.ssllabs.com/ssltest/) 测试 → A+ |
| 9 | 公开分享密码保护 | 无密码直接访问 `/api/v2/share/{code}/download` → 被拒 |
| 10 | CORS `*` 移除 | 应用层 `AllowedOrigins` 不是 `*`（或已被 Nginx 覆盖） |

---

## 附：.env 新增配置项

在 `config.go` 中增加 `CORSOrigins` 支持后，`.env` 示例如下：

```env
# 安全
SECRET_KEY=your-256-bit-secret-here-change-in-production
CORS_ORIGINS=https://stora.example.com,https://admin.stora.example.com

# 服务器
PORT=9421
HOST=127.0.0.1              # 生产环境只监听本地，由 Nginx 转发

# 数据库
DB_HOST=localhost
DB_PORT=5432
DB_NAME=stora
DB_USER=stora
DB_PASSWORD=strong-password

# Redis（用于缓存/限流/会话）
REDIS_HOST=localhost
REDIS_PORT=6379

# 存储
STORAGE_DRIVER=local
STORAGE_OBJECTS_DIR=storage/objects

# 文件上传
TEMP_FOLDER=temp/upload
```

---

> **参见**：[README.md](../README.md) — 快速开始  
> **参见**：[gap-analysis.md](gap-analysis.md) — 功能差距分析
