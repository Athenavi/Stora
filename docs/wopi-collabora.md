# Collabora Online 集成部署指南

> 利用 Stora 已有的 WOPI 协议接口，对接 Collabora Online 实现 Office 文档在线编辑。

---

## 架构

```
用户浏览器
  ↕ iframe POST (Collabora Online JS)
Collabora Online (Docker: collabora/code)
  ↕ WOPI REST API
Stora WOPI 端点 (/api/v2/wopi/...)
  ↕
文件存储系统
```

---

## 前提条件

- Docker 20+（运行 Collabora Online 容器）
- Stora 已部署运行（alpha 分支，含 WOPI 端点）
- 域名（Collabora Online 需要有效的 HTTPS 连接）

---

## 1. 部署 Collabora Online

```bash
docker run -d \
  --name=collabora \
  -p 9980:9980 \
  -e "aliasgroup1=https://<STORA_DOMAIN>:443" \
  -e "username=admin" \
  -e "password=YOUR_ADMIN_PASSWORD" \
  -e "extra_params=--o:ssl.enable=false" \
  collabora/code
```

> 开发环境可禁用 SSL（`ssl.enable=false`），生产环境建议使用反向代理配置 HTTPS。  
> Collabora Online 默认监听端口 `9980`。

---

## 2. 配置 Stora

编辑 `.env` 文件：

```env
# Collabora Online 服务器地址（不含 WOPISrc）
COLLABORA_URL=http://localhost:9980
```

---

## 3. 验证

1. 启动 Collabora 容器
2. 在 Stora 中打开一个 Office 文档（.docx/.xlsx/.pptx）
3. 点击预览页的"在 Office 中编辑"按钮
4. 浏览器打开 Collabora Online 编辑界面
5. 编辑文档 → 自动保存回 Stora（通过 WOPI PutFile）

---

## 4. 文件格式支持

| 格式 | 可查看 | 可编辑 |
|------|--------|--------|
| .docx | ✅ | ✅ |
| .xlsx | ✅ | ✅ |
| .pptx | ✅ | ✅ |
| .doc | ✅ | ✅ |
| .xls | ✅ | ✅ |
| .ppt | ✅ | ✅ |
| .odt | ✅ | ✅ |
| .ods | ✅ | ✅ |
| .odp | ✅ | ✅ |
| .txt | ✅ | ✅ |

---

## 5. 生产环境推荐

```bash
# 使用 nginx 反向代理 Collabora（启用 SSL 后）
docker run -d \
  --name=collabora \
  -p 127.0.0.1:9980:9980 \
  -e "aliasgroup1=https://collabora.example.com:443" \
  -e "username=admin" \
  -e "password=STRONG_PASSWORD" \
  -e "server_name=collabora.example.com" \
  -e "dictionaries=en_ZH" \
  collabora/code
```

### nginx 配置

```nginx
server {
    listen 443 ssl;
    server_name collabora.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:9980;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
    }
}
```

---

## 6. 排错

| 问题 | 解决 |
|------|------|
| iframe 白屏 | 检查 `COLLABORA_URL` 是否可访问；检查浏览器控制台有无跨域错误 |
| 无法保存文档 | 检查 Stora WOPI `PutFile` 端点是否正确；检查文件路径可写 |
| 404 错误 | 检查 `aliasgroup1` 是否包含 Stora 的访问域名 |
| 证书错误 | Collabora 容器需要有效 SSL 证书或设置 `ssl.enable=false` |
