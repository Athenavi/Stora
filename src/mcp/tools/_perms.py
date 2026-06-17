"""
MCP 工具权限校验 — contextvars 机制

使用 contextvars 在 API 层注入当前用户信息，
工具处理器可在任意异步上下文中读取并校验权限。
"""
import logging
from typing import Optional

from src.mcp._context import get_user_ctx, UserCtx

logger = logging.getLogger("mcp.perms")

# ─── 权限级别 ────────────────────────────────────────────────

ROLE_HIERARCHY = {
    "superuser": 100,
    "admin": 80,
    "editor": 60,
    "author": 40,
    "user": 20,
    "guest": 0,
}


def get_current_user() -> Optional[UserCtx]:
    """工具处理器获取当前用户身份（从 server.py 的 contextvar）"""
    return get_user_ctx()


def require_role(min_role: str = "user"):
    """装饰器/包装器：要求用户角色不低于 min_role"""
    def decorator(func):
        async def wrapper(arguments: dict) -> dict:
            user = get_current_user()
            if not user:
                return {"success": False, "error": "未登录，无法执行操作",
                        "code": "UNAUTHORIZED"}

            user_level = ROLE_HIERARCHY.get(user.role, 0)
            required_level = ROLE_HIERARCHY.get(min_role, 0)

            if user.is_superuser:
                return await func(arguments)

            if user_level < required_level:
                return {"success": False, "error": f"权限不足：需要 {min_role} 角色，当前 {user.role}",
                        "code": "FORBIDDEN"}

            return await func(arguments)
        return wrapper
    return decorator


def require_superuser(func):
    """要求超级管理员权限"""
    async def wrapper(arguments: dict) -> dict:
        user = get_current_user()
        if not user:
            return {"success": False, "error": "未登录", "code": "UNAUTHORIZED"}
        if not user.is_superuser:
            return {"success": False, "error": "需要超级管理员权限", "code": "FORBIDDEN"}
        return await func(arguments)
    return wrapper


def require_self_or_admin(user_id_key: str = "user_id"):
    """要求操作本人或管理员"""
    def decorator(func):
        async def wrapper(arguments: dict) -> dict:
            user = get_current_user()
            if not user:
                return {"success": False, "error": "未登录", "code": "UNAUTHORIZED"}
            if user.is_superuser or user.role in ("admin", "superuser"):
                return await func(arguments)
            target_id = arguments.get(user_id_key)
            if target_id is not None and int(target_id) == user.id:
                return await func(arguments)
            return {"success": False, "error": "只能操作自己的数据", "code": "FORBIDDEN"}
        return wrapper
    return decorator
