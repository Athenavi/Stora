# Collabora Online 集成部署指南

> 利用 Stora 的 WOPI 接口对接 Collabora Online，实现 Office 文档在线编辑。

---

## 架构

```
用户浏览器 → Collabora Online (Docker) → Stora WOPI API (/api/v2/wopi/...)
```

## 前提

- Docker 20+
- 域名（Collabora 需要有效 HTTPS）

## 部署步骤

### 1. 启动 Collabora

```bash
docker run -d --name collabora -p 9980:9980 \
  -e "aliasgroup1=https://<STORA_DOMAIN>:443" \
  -e "username=admin" -e "password=YOUR_ADMIN_PASSWORD" \
  -e "extra_params=--o:ssl.enable=false" \
  collabora/code
```

### 2. 配置 .env

```env
COLLABORA_URL=http://localhost:9980
```

### 3. 验证

打开 .docx/.xlsx/.pptx 文件 → 点击"在 Office 中编辑" → 进入 Collabora 编辑界面。

## 文件格式

| 格式 | 可查看 | 可编辑 |
|------|--------|--------|
| .docx .xlsx .pptx | ✅ | ✅ |
| .doc .xls .ppt | ✅ | ✅ |
| .odt .ods .odp | ✅ | ✅ |
| .txt | ✅ | ✅ |
