"""
FastBlog API v3

分层结构:
  /api/v3/mobile/*  — 移动端 API（现有，不变）
  /api/v3/admin/*   — 管理端 API（新增，权限驱动）
"""

from fastapi import APIRouter

# v3 路由注册表：(模块路径, v3前缀, 标签列表, 是否必需)
ROUTE_REGISTRY_V3 = [
    # ==================== 认证模块 ====================
    ("src.api.v3.mobile.auth", "/api/v3/auth", ["mobile-auth"], True),

    # ==================== 文章模块 ====================
    ("src.api.v3.mobile.articles", "/api/v3/articles", ["mobile-articles"], True),

    # ==================== 评论模块 ====================
    ("src.api.v3.mobile.comments", "/api/v3/comments", ["mobile-comments"], True),

    # ==================== 用户模块 ====================
    ("src.api.v3.mobile.users", "/api/v3/users", ["mobile-users"], True),

    # ==================== 媒体模块 ====================
    ("src.api.v3.mobile.media", "/api/v3/media", ["mobile-media"], True),

    # ==================== 分类模块 ====================
    ("src.api.v3.mobile.categories", "/api/v3/categories", ["mobile-categories"], True),

    # ==================== 管理端 API ====================
    ("src.api.v3.admin.users", "/api/v3/admin", ["admin-users"], False),
    ("src.api.v3.admin.articles", "/api/v3/admin", ["admin-articles"], False),
    ("src.api.v3.admin.media", "/api/v3/admin", ["admin-media"], False),
    ("src.api.v3.admin.system", "/api/v3/admin", ["admin-system"], False),
    ("src.api.v3.admin.roles", "/api/v3/admin", ["admin-roles"], False),
    ("src.api.v3.admin.dashboard", "/api/v3/admin", ["admin-dashboard"], False),
    ("src.api.v3.admin.comments", "/api/v3/admin", ["admin-comments"], False),
    ("src.api.v3.admin.plugins", "/api/v3/admin", ["admin-plugins"], False),
    ("src.api.v3.admin.themes", "/api/v3/admin", ["admin-themes"], False),
    ("src.api.v3.admin.seo", "/api/v3/admin", ["admin-seo"], False),
    ("src.api.v3.admin.backup", "/api/v3/admin", ["admin-backup"], False),
    ("src.api.v3.admin.permission", "/api/v3/admin", ["admin-permission"], False),
    ("src.api.v3.admin.categories", "/api/v3/admin", ["admin-categories"], False),
    ("src.api.v3.admin.analytics", "/api/v3/admin", ["admin-analytics"], False),
    ("src.api.v3.admin.notifications", "/api/v3/admin", ["admin-notifications"], False),
    ("src.api.v3.admin.search_analytics", "/api/v3/admin", ["admin-search-analytics"], False),
    ("src.api.v3.admin.webhooks", "/api/v3/admin", ["admin-webhooks"], False),
    ("src.api.v3.admin.widgets", "/api/v3/admin", ["admin-widgets"], False),
]


def register_v3_routes(app):
    """注册 v3 API 路由"""
    from importlib import import_module

    for module_path, prefix, tags, required in ROUTE_REGISTRY_V3:
        try:
            module = import_module(module_path)
            if hasattr(module, 'router'):
                app.include_router(module.router, prefix=prefix, tags=tags)
                print(f"[V3 API] Registered: {prefix}")
            else:
                print(f"[V3 API] Warning: No router found in {module_path}")
        except Exception as e:
            if required:
                print(f"[V3 API] Error: Failed to load required module {module_path}: {e}")
                raise
            else:
                print(f"[V3 API] Warning: Failed to load optional module {module_path}: {e}")


# 创建主路由器
router = APIRouter(prefix="/api/v3", tags=["mobile-api-v3"])
