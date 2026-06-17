"""
RBAC（基于角色的访问控制）服务实现
提供权限检查、角色分配等功能。
所有方法接受显式的 db: AsyncSession 参数。
"""
from datetime import datetime, timezone
from typing import List, Optional

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.rbac import UserRole, RoleCapability, Capability, Role


class RBACService:
    """RBAC 核心服务"""

    async def _resolve_role_ids_with_parents(
        self, db: AsyncSession, role_ids: List[int], max_depth: int = 5
    ) -> List[int]:
        """
        Walk the role hierarchy (parent_id chain) up to max_depth levels
        and return the full set of role IDs including ancestors.
        """
        resolved = set(role_ids)
        current_ids = list(role_ids)
        depth = 0
        while current_ids and depth < max_depth:
            result = await db.execute(
                select(Role.id).where(
                    Role.parent_id.in_(current_ids),
                    Role.parent_id.isnot(None),
                )
            )
            current_ids = [r for r in result.scalars().all() if r not in resolved]
            resolved.update(current_ids)
            depth += 1
        return list(resolved)

    # ------------------------------------------------------------
    # 能力检查（基于 Capability.code）
    # ------------------------------------------------------------

    async def has_capability(
        self, db: AsyncSession, user_id: int, capability_code: str
    ) -> bool:
        """
        检查用户是否拥有特定能力（通过 capability code）。

        用户为 superuser 时直接返回 True。
        """
        # 标准化 separator: 冒号 → 点号以匹配 DB 存储格式
        capability_code = capability_code.replace(':', '.')

        from shared.models.user import User

        # 超级管理员 bypass
        user_result = await db.execute(select(User).where(User.id == user_id))
        user = user_result.scalar_one_or_none()
        if user and user.is_superuser:
            return True

        # 1. 获取用户的所有角色
        user_roles_result = await db.execute(
            select(UserRole).where(UserRole.user_id == user_id)
        )
        role_ids = [ur.role_id for ur in user_roles_result.scalars().all()]
        if not role_ids:
            return False

        # 1b. Resolve role hierarchy (parent roles)
        role_ids = await self._resolve_role_ids_with_parents(db, role_ids)

        # 2. 检查这些角色是否拥有指定能力
        query = (
            select(RoleCapability)
            .join(Capability, RoleCapability.capability_id == Capability.id)
            .where(
                RoleCapability.role_id.in_(role_ids),
                Capability.code == capability_code,
            )
        )
        result = await db.execute(query)
        return result.scalar_one_or_none() is not None

    async def has_permission(
        self, db: AsyncSession, user_id: int, resource: str, action: str
    ) -> bool:
        """
        检查用户是否有指定资源的操作权限。
        底层将 `resource.action` 拼接为 capability code 后委托 has_capability。
        """
        return await self.has_capability(db, user_id, f"{resource}.{action}")

    async def has_any_permission(
        self, db: AsyncSession, user_id: int, permissions: List[tuple]
    ) -> bool:
        """
        检查用户是否拥有任一权限。

        Args:
            permissions: [(resource, action), ...]
        """
        for resource, action in permissions:
            if await self.has_permission(db, user_id, resource, action):
                return True
        return False

    async def has_all_permissions(
        self, db: AsyncSession, user_id: int, permissions: List[tuple]
    ) -> bool:
        """
        检查用户是否拥有所有指定权限。

        Args:
            permissions: [(resource, action), ...]
        """
        for resource, action in permissions:
            if not await self.has_permission(db, user_id, resource, action):
                return False
        return True

    # ------------------------------------------------------------
    # 获取用户权限
    # ------------------------------------------------------------

    async def get_user_capabilities(
        self, db: AsyncSession, user_id: int
    ) -> List[Capability]:
        """获取用户的所有能力列表"""
        from shared.models.user import User

        user_result = await db.execute(select(User).where(User.id == user_id))
        user = user_result.scalar_one_or_none()
        if not user:
            return []

        # 超级管理员返回所有 Capabilities
        if user.is_superuser:
            result = await db.execute(select(Capability))
            return list(result.scalars().all())

        role_ids_result = await db.execute(
            select(UserRole.role_id).where(UserRole.user_id == user_id)
        )
        role_ids = [r for r in role_ids_result.scalars().all()]
        if not role_ids:
            return []

        # Resolve role hierarchy (parent roles)
        role_ids = await self._resolve_role_ids_with_parents(db, role_ids)

        query = (
            select(Capability)
            .join(RoleCapability, RoleCapability.capability_id == Capability.id)
            .where(RoleCapability.role_id.in_(role_ids))
            .distinct()
        )
        result = await db.execute(query)
        return list(result.scalars().all())

    async def get_user_permission_codes(
        self, db: AsyncSession, user_id: int
    ) -> List[str]:
        """获取用户的所有权限代码列表（如 ['article.view', 'article.create']）"""
        caps = await self.get_user_capabilities(db, user_id)
        return [c.code for c in caps if c.code]

    # ------------------------------------------------------------
    # 角色分配
    # ------------------------------------------------------------

    async def assign_role(
        self, db: AsyncSession, user_id: int, role_slug: str, assigned_by: Optional[int] = None
    ) -> bool:
        """为用户分配角色（幂等）"""
        role_result = await db.execute(select(Role).where(Role.slug == role_slug))
        role = role_result.scalar_one_or_none()
        if not role:
            raise ValueError(f"Role '{role_slug}' not found")

        # 检查是否已分配
        existing = await db.execute(
            select(UserRole).where(
                UserRole.user_id == user_id,
                UserRole.role_id == role.id,
            )
        )
        if existing.scalar_one_or_none():
            return True  # 幂等

        user_role = UserRole(
            user_id=user_id,
            role_id=role.id,
            assigned_by=assigned_by,
            created_at=datetime.now(timezone.utc),
        )
        db.add(user_role)
        await db.flush()
        return True

    async def remove_role(self, db: AsyncSession, user_id: int, role_slug: str) -> bool:
        """移除用户的某个角色"""
        role_result = await db.execute(select(Role).where(Role.slug == role_slug))
        role = role_result.scalar_one_or_none()
        if not role:
            return False

        result = await db.execute(
            select(UserRole).where(
                UserRole.user_id == user_id,
                UserRole.role_id == role.id,
            )
        )
        ur = result.scalar_one_or_none()
        if not ur:
            return False

        await db.delete(ur)
        await db.flush()
        return True

    async def get_user_roles(
        self, db: AsyncSession, user_id: int
    ) -> List[Role]:
        """获取用户的所有角色"""
        result = await db.execute(
            select(Role)
            .join(UserRole, UserRole.role_id == Role.id)
            .where(UserRole.user_id == user_id)
        )
        return list(result.scalars().all())

    async def user_has_role(
        self, db: AsyncSession, user_id: int, role_slug: str
    ) -> bool:
        """
        检查用户是否拥有指定角色（支持角色继承：父角色算作拥有子角色）

        Args:
            db: 数据库会话
            user_id: 用户ID
            role_slug: 角色 slug

        Returns:
            是否拥有该角色
        """
        # 先获取直接分配的角色（含父角色解析）
        user_role_ids_result = await db.execute(
            select(UserRole.role_id).where(UserRole.user_id == user_id)
        )
        user_role_ids = [r[0] for r in user_role_ids_result.all()]
        if not user_role_ids:
            return False

        # 解析角色继承链
        all_role_ids = await self._resolve_role_ids_with_parents(db, user_role_ids)

        # 检查是否有匹配的 slug
        result = await db.execute(
            select(Role).where(Role.id.in_(all_role_ids), Role.slug == role_slug)
        )
        return result.scalar_one_or_none() is not None


# 全局单例
rbac_service = RBACService()
