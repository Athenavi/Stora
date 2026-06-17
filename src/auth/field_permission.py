"""
字段级权限控制装饰器
基于 FieldPermission 模型检查用户对模型字段的读写权限
"""

from functools import wraps
from typing import List, Optional

from fastapi import Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.security import FieldPermission
from shared.models.rbac import UserRole
from src.auth.auth_deps import jwt_required_dependency as jwt_required
from src.utils.database.main import get_async_session as get_async_db
from src.utils.database.main import get_async_session_context


async def get_field_permissions(
    db: AsyncSession,
    role_ids: List[int],
    model_name: str,
    field_name: str
) -> tuple[bool, bool]:
    """
    获取指定角色对某模型字段的读写权限

    Returns:
        (can_read, can_write)
    """
    if not role_ids:
        return True, False

    result = await db.execute(
        select(FieldPermission)
        .where(
            FieldPermission.role_id.in_(role_ids),
            FieldPermission.model_name == model_name,
            FieldPermission.field_name == field_name,
        )
    )
    permissions = result.scalars().all()

    if not permissions:
        # 无明确配置时: 可读、不可写
        return True, False

    can_read = any(p.can_read for p in permissions)
    can_write = any(p.can_write for p in permissions)
    return can_read, can_write


def check_field_permission(model_name: str, field_name: str, action: str = "read"):
    """
    字段级权限检查装饰器

    Args:
        model_name: 模型名称 (如 "User", "Article")
        field_name: 字段名称 (如 "email", "phone")
        action: "read" 或 "write"

    Usage:
        @router.get("/users/{id}")
        @check_field_permission("User", "email", "read")
        async def get_user(...):
            ...
    """
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            # 获取当前用户
            request = kwargs.get("request")
            current_user = kwargs.get("current_user")

            if not current_user and request:
                current_user = getattr(request.state, "user", None)

            if not current_user:
                raise HTTPException(status_code=401, detail="Authentication required")

            # 获取用户角色 — 使用提供或创建的 session
            db_session = kwargs.get("db")
            own_session = False
            if db_session is None:
                db_session = get_async_session_context()
                own_session = True

            if own_session:
                async with db_session as session:
                    await _check_field_permission(session, current_user, model_name, field_name, action)
            else:
                await _check_field_permission(db_session, current_user, model_name, field_name, action)

            return await func(*args, **kwargs)
        return wrapper
    return decorator


async def _check_field_permission(
    db: AsyncSession, current_user, model_name: str, field_name: str, action: str
):
    """统一的字段权限检查逻辑"""
    result = await db.execute(
        select(UserRole).where(UserRole.user_id == current_user.id)
    )
    user_roles = result.scalars().all()
    role_ids = [ur.role_id for ur in user_roles]

    can_read, can_write = await get_field_permissions(
        db, role_ids, model_name, field_name
    )

    if action == "read" and not can_read:
        raise HTTPException(status_code=403, detail=f"No read permission on {model_name}.{field_name}")
    if action == "write" and not can_write:
        raise HTTPException(status_code=403, detail=f"No write permission on {model_name}.{field_name}")


async def filter_fields_by_permission_async(
    user_id: int,
    model_name: str,
    fields: dict,
    action: str = "read"
) -> dict:
    """
    根据用户权限异步过滤字段（推荐使用此函数）
    """
    from src.utils.database.unified_manager import db_manager

    async with db_manager.get_session() as db:
        result = await db.execute(
            select(UserRole).where(UserRole.user_id == user_id)
        )
        user_roles = result.scalars().all()
        role_ids = [ur.role_id for ur in user_roles]

        filtered = {}
        for field_name, value in fields.items():
            can_read, _ = await get_field_permissions(db, role_ids, model_name, field_name)
            if can_read:
                filtered[field_name] = value
        return filtered


def filter_fields_by_permission(
    user_id: int,
    model_name: str,
    fields: dict,
    action: str = "read"
) -> dict:
    """
    根据用户权限过滤字段（同步版本，从当前事件循环运行）
    """
    import asyncio
    try:
        loop = asyncio.get_running_loop()
        if loop.is_running():
            # 从已运行的循环中调用 — 使用 run_coroutine_threadsafe 或创建新循环
            # 退化：在原循环中创建一个任务并等待
            import concurrent.futures
            # TODO: 当前使用 ThreadPoolExecutor + asyncio.run() 可能导致事件循环死锁。
            #       应改用 loop.create_task() + asyncio.wait() 或
            #       asyncio.run_coroutine_threadsafe() + concurrent.futures.Future
            #       以安全地在已有事件循环的线程中运行协程。
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                future = pool.submit(asyncio.run, filter_fields_by_permission_async(user_id, model_name, fields, action))
                return future.result()
    except RuntimeError:
        pass  # 没有运行中的循环

    loop = asyncio.new_event_loop()
    try:
        return loop.run_until_complete(
            filter_fields_by_permission_async(user_id, model_name, fields, action)
        )
    finally:
        loop.close()
