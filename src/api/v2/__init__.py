"""
Stora API v2 路由规范配置

保留的模块:
- auth: 用户认证
- users: 用户资料
- home: 首页
- dashboard: 仪表板
- media: 文件管理（核心）
- search: 全文搜索
- system: 系统管理
- security: 安全管理
- notifications: 通知
- performance: 性能监控
- compliance: GDPR 合规
- admin: 缓存管理
- mcp: AI MCP 代理
"""

# v2 路由注册表：(模块路径, v2前缀, 标签列表, 是否必需)
ROUTE_REGISTRY_V2 = [
    # ==================== 核心模块 ====================
    ("src.api.v2.home", "/api/v2", ["home"], True),
    ("src.api.v2.dashboard", "/api/v2", ["dashboard-v2"], True),

    # ==================== 搜索 ====================
    ("src.api.v2.search", "/api/v2/search", ["search-v2"], True),

    # ==================== 通知 ====================
    ("src.api.v2.notifications", "/api/v2/notifications", ["notifications-v2"], True),

    # ==================== 文件管理 ====================
    ("src.api.v2.media", "/api/v2/media", ["media"], True),
    ("src.api.v2.files", "/api/v2", ["files"], True),
    ("src.api.v2.files.webdav", "/api/v2", ["webdav"], False),

    # ==================== 安全与权限 ====================
    ("src.api.v2.security", "/api/v2/security", ["security-v2"], True),

    # ==================== 认证模块 ====================
    ("src.api.v2.auth", "/api/v2/auth", ["auth"], True),

    # ==================== 用户管理 ====================
    ("src.api.v2.users", "/api/v2", ["users-v2"], True),

    # ==================== 性能监控与优化 ====================
    ("src.api.v2.performance", "/api/v2/performance", ["performance-v2"], True),

    # ==================== 系统管理 ====================
    ("src.api.v2.system", "/api/v2/system", ["system-v2"], True),

    # ==================== GDPR 合规 ====================
    ("src.api.v2.compliance.compliance_api", "/api/v2", ["compliance-management-v2"], True),

    # ==================== 审计日志 ====================
    ("src.api.v2.system.audit", "/api/v2/audit-logs", ["audit-logs"], False),
    ("src.api.v2.admin.cache_management", "/api/v2/admin/caches", ["cache-admin"], False),

    # ==================== MCP AI 代理 ====================
    ("src.api.v2.mcp", "/api/v2", ["mcp-proxy-v2"], False),
]
