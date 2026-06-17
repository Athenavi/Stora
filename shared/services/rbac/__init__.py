"""
RBAC 权限检查服务
提供用户权限查询、验证、角色继承等功能
"""
from typing import Dict, List, Optional, Set, Any

from sqlalchemy import select, or_
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.rbac import Role, Capability, RoleCapability, UserRole
from src.unified_logger import default_logger as logger


async def get_user_capabilities(
    db: AsyncSession,
    user_id: int,
) -> List[Dict[str, Any]]:
    """
    获取用户的所有权限能力（含角色继承）

    Args:
        db: 数据库会话
        user_id: 用户 ID

    Returns:
        权限能力列表
    """
    # 获取用户的角色 ID 列表（含继承链）
    all_role_ids = await _get_user_role_ids_with_inheritance(db, user_id)
    if not all_role_ids:
        return []

    # 查询所有关联的 Capability
    query = (
        select(Capability)
        .join(RoleCapability, Capability.id == RoleCapability.capability_id)
        .where(
            RoleCapability.role_id.in_(all_role_ids),
            Capability.is_active == True,
        )
        .distinct()
    )
    result = await db.execute(query)
    capabilities = result.scalars().all()

    return [c.to_dict() for c in capabilities]


async def check_user_permission(
    db: AsyncSession,
    user_id: int,
    required_capability: str,
) -> bool:
    """
    检查用户是否拥有指定权限

    Args:
        db: 数据库会话
        user_id: 用户 ID
        required_capability: 权限代码，如 "article:create"

    Returns:
        是否拥有权限
    """
    try:
        all_role_ids = await _get_user_role_ids_with_inheritance(db, user_id)
        if not all_role_ids:
            return False

        query = (
            select(Capability)
            .join(RoleCapability, Capability.id == RoleCapability.capability_id)
            .where(
                RoleCapability.role_id.in_(all_role_ids),
                Capability.code == required_capability,
                Capability.is_active == True,
            )
        )
        result = await db.execute(query)
        return result.scalar_one_or_none() is not None

    except Exception as e:
        logger.error(f"权限检查失败: {e}", exc_info=True)
        return False


async def check_any_permission(
    db: AsyncSession,
    user_id: int,
    resource_type: str,
    action: str,
) -> bool:
    """
    按资源类型和操作检查权限

    Args:
        db: 数据库会话
        user_id: 用户 ID
        resource_type: 资源类型（article, user, category 等）
        action: 操作类型（create, read, update, delete）

    Returns:
        是否拥有权限
    """
    try:
        all_role_ids = await _get_user_role_ids_with_inheritance(db, user_id)
        if not all_role_ids:
            return False

        query = (
            select(Capability)
            .join(RoleCapability, Capability.id == RoleCapability.capability_id)
            .where(
                RoleCapability.role_id.in_(all_role_ids),
                Capability.resource_type == resource_type,
                Capability.action == action,
                Capability.is_active == True,
            )
        )
        result = await db.execute(query)
        return result.scalar_one_or_none() is not None

    except Exception as e:
        logger.error(f"权限检查失败: {e}", exc_info=True)
        return False


async def get_user_permission_codes(db: AsyncSession, user_id: int) -> Set[str]:
    """
    获取用户的权限代码集合（用于前端缓存/判断）

    Args:
        db: 数据库会话
        user_id: 用户 ID

    Returns:
        权限代码集合
    """
    all_role_ids = await _get_user_role_ids_with_inheritance(db, user_id)
    if not all_role_ids:
        return set()

    query = (
        select(Capability.code)
        .join(RoleCapability, Capability.id == RoleCapability.capability_id)
        .where(
            RoleCapability.role_id.in_(all_role_ids),
            Capability.is_active == True,
        )
        .distinct()
    )
    result = await db.execute(query)
    return {row[0] for row in result.all() if row[0]}


async def assign_role_to_user(
    db: AsyncSession,
    user_id: int,
    role_id: int,
) -> bool:
    """
    为用户分配角色

    Args:
        db: 数据库会话
        user_id: 用户 ID
        role_id: 角色 ID

    Returns:
        是否成功
    """
    try:
        from datetime import datetime, timezone

        # 检查是否已分配
        result = await db.execute(
            select(UserRole).where(
                UserRole.user_id == user_id,
                UserRole.role_id == role_id,
            )
        )
        if result.scalar_one_or_none():
            return True  # 已分配，视为成功

        assignment = UserRole(
            user_id=user_id,
            role_id=role_id,
            created_at=datetime.now(timezone.utc),
        )
        db.add(assignment)
        await db.commit()
        return True

    except Exception as e:
        await db.rollback()
        logger.error(f"分配角色失败: {e}", exc_info=True)
        return False


async def remove_role_from_user(
    db: AsyncSession,
    user_id: int,
    role_id: int,
) -> bool:
    """
    移除用户的角色

    Args:
        db: 数据库会话
        user_id: 用户 ID
        role_id: 角色 ID
    """
    try:
        from sqlalchemy import delete as sa_delete
        await db.execute(
            sa_delete(UserRole).where(
                UserRole.user_id == user_id,
                UserRole.role_id == role_id,
            )
        )
        await db.commit()
        return True

    except Exception as e:
        await db.rollback()
        logger.error(f"移除角色失败: {e}", exc_info=True)
        return False


async def _get_user_role_ids_with_inheritance(
    db: AsyncSession,
    user_id: int,
) -> Set[int]:
    """
    获取用户的角色 ID 集合（含角色继承链）

    递归遍历角色继承树，收集所有父角色。
    """
    # 直接分配的角色
    result = await db.execute(
        select(UserRole).where(
            UserRole.user_id == user_id,
        )
    )
    user_roles = result.scalars().all()
    direct_role_ids = {ur.role_id for ur in user_roles}

    if not direct_role_ids:
        return set()

    # 收集所有祖先角色（含自身）
    all_role_ids = set(direct_role_ids)
    to_process = list(direct_role_ids)

    # 限制最大深度避免死循环
    max_depth = 20
    depth = 0
    while to_process and depth < max_depth:
        current_id = to_process.pop(0)
        # 查询此角色的父角色
        result = await db.execute(
            select(Role.parent_id).where(
                Role.id == current_id,
                Role.parent_id.isnot(None),
            )
        )
        parent_id = result.scalar()
        if parent_id and parent_id not in all_role_ids:
            all_role_ids.add(parent_id)
            to_process.append(parent_id)
        depth += 1

    return all_role_ids


__all__ = [
    'get_user_capabilities',
    'check_user_permission',
    'check_any_permission',
    'get_user_permission_codes',
    'assign_role_to_user',
    'remove_role_from_user',
]
