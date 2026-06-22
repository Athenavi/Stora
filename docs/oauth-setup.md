# OAuth 第三方登录配置指南

Stora 支持 GitHub 和 Google OAuth 第三方登录，配置完成后用户可一键登录。

---

## GitHub OAuth

### 1. 创建 OAuth App

1. 访问 https://github.com/settings/developers → **OAuth Apps** → **New OAuth App**
2. 填写：
   - **Application name**: Stora
   - **Homepage URL**: `https://your-domain.com`
   - **Authorization callback URL**: `https://your-domain.com/api/v2/auth/oauth/github/callback`
3. 点击 **Register application**
4. 记录 **Client ID** 和 **Client Secret**

### 2. 配置 .env

```env
GITHUB_CLIENT_ID=your_client_id
GITHUB_CLIENT_SECRET=your_client_secret
```

### 3. 验证

1. 启动 Stora 服务
2. 访问登录页，点击 GitHub 按钮
3. 跳转到 GitHub 授权页面 → 授权后自动登录

---

## Google OAuth

### 1. 创建 OAuth 凭据

1. 访问 https://console.cloud.google.com/apis/credentials
2. 创建项目 → **OAuth 同意屏幕** → 配置应用信息
3. **凭据** → **创建凭据** → **OAuth 客户端 ID**
4. 应用类型选择 **Web 应用**
5. 添加授权重定向 URI：
   - `https://your-domain.com/api/v2/auth/oauth/google/callback`
6. 记录 **Client ID** 和 **Client Secret**

### 2. 配置 .env

```env
GOOGLE_CLIENT_ID=your_client_id
GOOGLE_CLIENT_SECRET=your_client_secret
```

### 3. 验证

1. 启动 Stora 服务
2. 访问登录页，点击 Google 按钮
3. 跳转到 Google 授权页面 → 授权后自动登录

---

## 环境变量汇总

```env
# OAuth
GITHUB_CLIENT_ID=
GITHUB_CLIENT_SECRET=
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=

# 登录成功后的默认跳转地址
OAUTH_REDIRECT_URL=/drive
```

---

## 排错

| 问题 | 解决 |
|------|------|
| 回调 404 | 检查回调 URL 是否与注册的完全一致（含域名） |
| redirect_uri 不匹配 | GitHub/Google 控制台中的回调 URL 必须与请求完全一致 |
| 登录后空白页 | 检查 `OAUTH_REDIRECT_URL` 是否有效 |
| 403 Forbidden | 检查 Client Secret 是否正确 |
