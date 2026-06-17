"""
RBAC 权限中间件
为 API 请求提供全局权限检查
"""

import re
from typing import List, Optional, Dict

from fastapi import Request, HTTPException
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import Response

from shared.services.security.rbac_service import rbac_service
from src.unified_logger import default_logger as logger


# 路由-权限映射配置
# 格式: (method_regex, path_regex, resource, action, [exempt_methods])
ROUTE_PERMISSION_MAP = [
    # RBAC 管理 (V2 + V3)
    (r"POST", r"/api/v2/security/roles", "user", "manage_roles"),
    (r"PUT", r"/api/v2/security/roles/\d+", "user", "manage_roles"),
    (r"DELETE", r"/api/v2/security/roles/\d+", "user", "manage_roles"),
    (r"POST", r"/api/v3/admin/roles", "user", "manage_roles"),
    (r"PUT", r"/api/v3/admin/roles/\d+/permissions", "user", "manage_roles"),
    (r"DELETE", r"/api/v3/admin/roles/\d+", "user", "manage_roles"),
    # 用户管理 (V2 + V3)
    (r"POST", r"/api/v2/users/\d+/roles", "user", "manage_roles"),
    (r"DELETE", r"/api/v2/users/\d+/roles/\d+", "user", "manage_roles"),
    (r"POST", r"/api/v2/users", "user", "create"),
    (r"PUT", r"/api/v2/users/\d+", "user", "update"),
    (r"DELETE", r"/api/v2/users/\d+", "user", "delete"),
    (r"POST", r"/api/v3/admin/users", "user", "create"),
    (r"PUT", r"/api/v3/admin/users/\d+", "user", "update"),
    (r"DELETE", r"/api/v3/admin/users/\d+", "user", "delete"),
    (r"POST", r"/api/v3/admin/users/\d+/roles", "user", "manage_roles"),
    # 用户安全管理 (V2)
    (r"POST", r"/api/v2/users/security/field-permissions", "user", "manage_field_permissions"),
    (r"PUT", r"/api/v2/users/security/field-permissions/\d+", "user", "manage_field_permissions"),
    (r"DELETE", r"/api/v2/users/security/field-permissions/\d+", "user", "manage_field_permissions"),
    (r"POST", r"/api/v2/users/security/two-factor", "user", "manage_2fa"),
    # 文章管理 (V2 + V3)
    (r"POST", r"/api/v2/articles", "article", "create"),
    (r"PUT", r"/api/v2/articles/\d+", "article", "update"),
    (r"DELETE", r"/api/v2/articles/\d+", "article", "delete"),
    (r"POST", r"/api/v2/articles/batch-operation", "article", "batch"),
    (r"POST", r"/api/v2/articles/\d+/sticky", "article", "edit"),
    (r"POST", r"/api/v2/articles/reorder", "article", "edit"),
    (r"POST", r"/api/v2/articles/admin/clean-expired-sticky", "article", "edit"),
    (r"POST", r"/api/v3/admin/articles", "article", "create"),
    (r"PUT", r"/api/v3/admin/articles/\d+", "article", "update"),
    (r"DELETE", r"/api/v3/admin/articles/\d+", "article", "delete"),
    (r"POST", r"/api/v3/admin/articles/\d+/publish", "article", "publish"),
    # 文章修订/注释 (V2)
    (r"POST", r"/api/v2/articles/\d+/revisions", "article", "edit"),
    (r"DELETE", r"/api/v2/articles/\d+/revisions/\d+", "article", "edit"),
    (r"POST", r"/api/v2/articles/\d+/revisions/\d+/rollback", "article", "edit"),
    (r"POST", r"/api/v2/articles/notes", "article", "edit"),
    (r"PUT", r"/api/v2/articles/notes/\d+", "article", "edit"),
    (r"DELETE", r"/api/v2/articles/notes/\d+", "article", "edit"),
    # 定时发布 (V2)
    (r"POST", r"/api/v2/articles/scheduler/schedule", "article", "publish"),
    (r"POST", r"/api/v2/articles/scheduler/cancel/\d+", "article", "publish"),
    # 分类管理 (V2 + V3)
    (r"POST", r"/api/v2/cms/categories", "category", "create"),
    (r"PUT", r"/api/v2/cms/categories/\d+", "category", "update"),
    (r"DELETE", r"/api/v2/cms/categories/\d+", "category", "delete"),
    (r"POST", r"/api/v2/cms/categories/reorder", "category", "edit"),
    (r"POST", r"/api/v3/admin/categories", "category", "create"),
    (r"PUT", r"/api/v3/admin/categories/\d+", "category", "update"),
    (r"DELETE", r"/api/v3/admin/categories/\d+", "category", "delete"),
    # 评论管理 (V2 + V3)
    (r"POST", r"/api/v2/comments/\d+/approve", "comment", "approve"),
    (r"POST", r"/api/v2/comments/\d+/reject", "comment", "reject"),
    (r"DELETE", r"/api/v2/comments/\d+", "comment", "delete"),
    (r"POST", r"/api/v3/admin/comments/\d+/approve", "comment", "approve"),
    (r"POST", r"/api/v3/admin/comments/\d+/reject", "comment", "reject"),
    (r"DELETE", r"/api/v3/admin/comments/\d+", "comment", "delete"),
    # 块编辑器 (V2 CMS)
    (r"POST", r"/api/v2/cms/editor/validate", "content", "edit"),
    (r"POST", r"/api/v2/cms/editor/render", "content", "edit"),
    (r"POST", r"/api/v2/cms/editor/convert", "content", "edit"),
    (r"POST", r"/api/v2/cms/editor/duplicate", "content", "edit"),
    (r"POST", r"/api/v2/cms/editor/merge", "content", "edit"),
    (r"POST", r"/api/v2/cms/editor/split", "content", "edit"),
    (r"POST", r"/api/v2/cms/editor/export", "content", "export"),
    (r"POST", r"/api/v2/cms/editor/import", "content", "import"),
    # 块模式 (V2 CMS)
    (r"POST", r"/api/v2/cms/pattern", "content", "edit"),
    (r"PUT", r"/api/v2/cms/pattern/\d+", "content", "edit"),
    (r"DELETE", r"/api/v2/cms/pattern/\d+", "content", "delete"),
    # 页面构建器 (V2 CMS)
    (r"POST", r"/api/v2/cms/page-builder", "page", "create"),
    (r"PUT", r"/api/v2/cms/page-builder/\d+", "page", "update"),
    (r"DELETE", r"/api/v2/cms/page-builder/\d+", "page", "delete"),
    (r"POST", r"/api/v2/cms/page-builder/\d+/publish", "page", "publish"),
    (r"POST", r"/api/v2/cms/page-builder/reorder", "page", "edit"),
    # 自定义文章类型 (V2 CMS)
    (r"POST", r"/api/v2/cms/post-types", "content", "create"),
    (r"PUT", r"/api/v2/cms/post-types/\d+", "content", "update"),
    (r"DELETE", r"/api/v2/cms/post-types/\d+", "content", "delete"),
    # 短代码管理 (V2 CMS)
    (r"POST", r"/api/v2/cms/shortcodes", "content", "edit"),
    (r"PUT", r"/api/v2/cms/shortcodes/\d+", "content", "edit"),
    (r"DELETE", r"/api/v2/cms/shortcodes/\d+", "content", "delete"),
    # 小部件管理 (V2 CMS)
    (r"POST", r"/api/v2/cms/widgets", "widget", "create"),
    (r"PUT", r"/api/v2/cms/widgets/\d+", "widget", "update"),
    (r"DELETE", r"/api/v2/cms/widgets/\d+", "widget", "delete"),
    (r"POST", r"/api/v2/cms/widgets/reorder", "widget", "edit"),
    # 媒体管理 (V2 + V3)
    (r"POST", r"/api/v2/media/upload", "media", "upload"),
    (r"DELETE", r"/api/v2/media/\d+", "media", "delete"),
    (r"DELETE", r"/api/v2/media/batch-delete", "media", "delete"),
    (r"POST", r"/api/v2/media/batch", "media", "edit"),
    (r"POST", r"/api/v2/media/\d+/edit", "media", "edit"),
    (r"POST", r"/api/v2/media/edit", "media", "edit"),
    (r"POST", r"/api/v2/media/edit/process", "media", "edit"),
    (r"POST", r"/api/v2/media/edit/crop", "media", "edit"),
    (r"POST", r"/api/v2/media/edit/rotate", "media", "edit"),
    (r"POST", r"/api/v2/media/edit/resize", "media", "edit"),
    (r"POST", r"/api/v2/media/folders", "media_folder", "create"),
    (r"PUT", r"/api/v2/media/folders/\d+", "media_folder", "update"),
    (r"DELETE", r"/api/v2/media/folders/\d+", "media_folder", "delete"),
    (r"POST", r"/api/v2/media/folders/move-media", "media_folder", "edit"),
    (r"POST", r"/api/v2/media/folders/copy-media", "media_folder", "edit"),
    (r"POST", r"/api/v2/media/\d+/tags", "media", "edit"),
    (r"DELETE", r"/api/v2/media/\d+/tags", "media", "edit"),
    (r"POST", r"/api/v2/media/\d+/remove-exif", "media", "edit"),
    (r"POST", r"/api/v2/media/optimize/\d+", "media", "edit"),
    (r"POST", r"/api/v2/media/convert-webp/\d+", "media", "edit"),
    (r"POST", r"/api/v2/media/settings/upload", "settings", "update"),
    (r"POST", r"/api/v2/media/batch/optimize", "media", "edit"),
    (r"POST", r"/api/v2/media/offline-download/tasks", "media", "download"),
    (r"POST", r"/api/v2/media/offline-download/tasks/\d+/cancel", "media", "download"),
    (r"POST", r"/api/v2/media/offline-download/tasks/\d+/retry", "media", "download"),
    (r"POST", r"/api/v3/admin/media/upload", "media", "upload"),
    (r"DELETE", r"/api/v3/admin/media/\d+", "media", "delete"),
    # 系统设置 (V2 + V3)
    (r"PUT", r"/api/v2/settings", "settings", "update"),
    (r"POST", r"/api/v2/settings", "settings", "update"),
    (r"PUT", r"/api/v3/admin/settings", "settings", "update"),
    (r"POST", r"/api/v2/backup", "system", "backup"),
    (r"POST", r"/api/v2/backup/\d+/delete", "system", "backup"),
    (r"POST", r"/api/v3/admin/backup", "system", "backup"),
    # CDN 配置 (V2)
    (r"PUT", r"/api/v2/performance/cdn", "settings", "update"),
    (r"POST", r"/api/v2/performance/cdn/purge", "settings", "update"),
    # 缓存管理 (V2)
    (r"POST", r"/api/v2/admin/cache/clear", "system", "maintenance"),
    (r"POST", r"/api/v2/performance/cache/purge", "system", "maintenance"),
    # 插件管理
    (r"POST", r"/api/v2/plugins", "plugin", "manage"),
    (r"PUT", r"/api/v2/plugins/\w+", "plugin", "manage"),
    (r"DELETE", r"/api/v2/plugins/\w+", "plugin", "manage"),
    (r"POST", r"/api/v3/admin/plugins/\w+/activate", "plugin", "manage"),
    (r"POST", r"/api/v3/admin/plugins/\w+/deactivate", "plugin", "manage"),
    (r"PUT", r"/api/v3/admin/plugins/\w+/settings", "plugin", "manage"),
    (r"DELETE", r"/api/v3/admin/plugins/\w+", "plugin", "manage"),
    # 主题管理
    (r"POST", r"/api/v2/themes", "theme", "manage"),
    (r"PUT", r"/api/v2/themes/\w+", "theme", "manage"),
    (r"POST", r"/api/v3/admin/themes/\w+/activate", "theme", "manage"),
    (r"PUT", r"/api/v3/admin/themes/\w+/config", "theme", "manage"),
    (r"DELETE", r"/api/v3/admin/themes/\w+", "theme", "manage"),
    # 协作 (V2)
    (r"POST", r"/api/v2/collaboration", "content", "edit"),
    (r"PUT", r"/api/v2/collaboration/comment/\d+", "content", "edit"),
    (r"DELETE", r"/api/v2/collaboration/comment/\d+", "content", "delete"),
    # OAuth 集成管理 (V2)
    (r"POST", r"/api/v2/integrations/oauth/providers", "settings", "update"),
    (r"PUT", r"/api/v2/integrations/oauth/providers/\w+", "settings", "update"),
    # 会员管理 (V2)
    (r"POST", r"/api/v2/membership/plans", "vip", "manage"),
    (r"PUT", r"/api/v2/membership/plans/\d+", "vip", "manage"),
    (r"DELETE", r"/api/v2/membership/plans/\d+", "vip", "manage"),
    # SEO 管理 (V2)
    (r"POST", r"/api/v2/seo/sitemap/generate", "settings", "update"),
    (r"POST", r"/api/v2/seo/robots", "settings", "update"),
    # 通知管理 (V2)
    (r"POST", r"/api/v2/notifications/templates", "notification", "create"),
    (r"PUT", r"/api/v2/notifications/templates/\d+", "notification", "update"),
    (r"DELETE", r"/api/v2/notifications/templates/\d+", "notification", "delete"),
]


class RBACMiddleware(BaseHTTPMiddleware):
    """RBAC 权限中间件 - 基于路由映射表检查操作权限"""

    # 豁免路径前缀（无需权限检查）
    EXEMPT_PREFIXES = [
        "/api/v2/auth/",
        "/api/v2/users/me",
        "/api/v2/articles/home/",
        "/api/v2/articles/p/",
        "/api/v2/articles/detail",
        "/api/v2/categories",
        "/api/v2/comments/article/",
        "/api/v2/membership/",
        "/api/v2/search/",
        "/api/v2/media/files",
        "/api/v2/media/upload",
        "/api/v2/shortcodes/",
        "/api/v2/widgets/",
        "/api/v2/articles/preview/",
    ]

    # 豁免方法（只读操作）
    EXEMPT_METHODS = {"GET", "HEAD", "OPTIONS"}

    async def dispatch(self, request: Request, call_next):
        # 只检查 API 路由
        path = request.url.path
        if not path.startswith("/api/"):
            return await call_next(request)

        # 检查豁免路径
        for prefix in self.EXEMPT_PREFIXES:
            if path.startswith(prefix):
                return await call_next(request)

        # 只对写操作检查权限
        if request.method in self.EXEMPT_METHODS:
            return await call_next(request)

        # 检查路由映射表
        for method_pattern, path_pattern, resource, action in ROUTE_PERMISSION_MAP:
            if re.match(method_pattern, request.method) and re.search(path_pattern, path):
                # 需要检查权限
                user = getattr(request.state, "user", None)
                if not user or not hasattr(user, "id"):
                    # 未认证用户 — 跳过 RBAC 检查，交由路由层 Depends(jwt_required) 处理
                    continue

                try:
                    from src.utils.database.unified_manager import db_manager
                    async with db_manager.get_session() as db:
                        has_perm = await rbac_service.has_permission(db, user.id, resource, action)
                        if not has_perm:
                            logger.warning(f"Permission denied: user={user.id} resource={resource} action={action} path={path}")
                            raise HTTPException(status_code=403, detail=f"Insufficient permissions: {resource}:{action}")
                except HTTPException:
                    raise
                except Exception as e:
                    logger.error(f"RBAC check error: {e}")
                    # 权限检查失败时保守处理 - 拒绝访问
                    raise HTTPException(status_code=403, detail="Permission check failed")
                break

        return await call_next(request)
