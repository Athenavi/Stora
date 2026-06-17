"""
V3 角色权限管理 API

权限要求:
  GET    /roles                    → user:manage_roles
  POST   /roles                    → user:manage_roles
  PUT    /roles/{id}/permissions   → user:manage_roles
  DELETE /roles/{id}               → user:manage_roles
  GET    /permissions              → user:view

路由函数内无权限查询，全部由 Depends(Permission) 提前完成。
"""
import logging
from datetime import datetime, timezone
from typing import List, Optional

from fastapi import APIRouter, Body, Depends, Query
from sqlalchemy import select, func, delete as sa_delete
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.rbac import Role, Capability, RoleCapability, UserRole
from shared.models import PermissionAuditLog
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db, get_current_user
from src.api.v3._permission import Permission, invalidate_permission_cache

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-roles"])


# ============================================================
# 角色列表
# ============================================================

@router.get("/roles", summary="角色列表")
async def list_roles(
    include_system: bool = Query(True),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:manage_roles")),
):
    query = select(Role)
    if not include_system:
        query = query.where(Role.is_system == False)

    result = await db.execute(query)
    roles = result.scalars().all()

    # 每个角色的用户数
    counts = await db.execute(
        select(UserRole.role_id, func.count(UserRole.user_id).label("cnt"))
        .group_by(UserRole.role_id)
    )
    user_counts = {row.role_id: row.cnt for row in counts}

    return ApiResponse(success=True, data={
        "roles": [_role_to_dict(r, user_counts.get(r.id, 0)) for r in roles],
    })


# ============================================================
# 创建角色
# ============================================================

@router.post("/roles", summary="创建角色", status_code=201)
async def create_role(
    name: str = Body(...),
    slug: str = Body(...),
    description: str = Body(""),
    permission_codes: Optional[List[str]] = Body(None),
    db: AsyncSession = Depends(get_db),
    current_user=Depends(get_current_user),
    _=Depends(Permission("user:manage_roles")),
):
    now = datetime.now(timezone.utc)
    role = Role(
        name=name,
        slug=slug,
        description=description,
        is_system=False,
        is_active=True,
        created_at=now,
        updated_at=now,
    )
    db.add(role)
    await db.flush()
    await db.refresh(role)

    if permission_codes:
        caps = await db.execute(
            select(Capability).where(Capability.code.in_(permission_codes))
        )
        role.capabilities = list(caps.scalars().all())

    await db.commit()
    # 审计日志
    audit = PermissionAuditLog(
        user_id=current_user.id, action='create_role', resource_type='role',
        resource_id=role.id, description=f'创建角色: {role.name}',
        created_at=datetime.now(timezone.utc),
    )
    db.add(audit)
    await db.commit()
    return ApiResponse(success=True, data=_role_to_dict(role), message="角色创建成功")


# ============================================================
# 更新角色权限
# ============================================================

@router.put("/roles/{role_id}/permissions", summary="更新角色权限")
async def update_role_permissions(
    role_id: int,
    permission_codes: List[str] = Body(...),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:manage_roles")),
):
    role = await db.get(Role, role_id)
    if not role:
        return ApiResponse(success=False, error="角色不存在")
    if role.is_system:
        return ApiResponse(success=False, error="系统角色不可修改权限")

    role.capabilities = []
    if permission_codes:
        caps = await db.execute(
            select(Capability).where(Capability.code.in_(permission_codes))
        )
        role.capabilities = list(caps.scalars().all())

    role.updated_at = datetime.now(timezone.utc)
    await db.commit()

    # 失效该角色所有用户的权限缓存
    affected = await db.execute(
        select(UserRole.user_id).where(UserRole.role_id == role_id)
    )
    for row in affected:
        await invalidate_permission_cache(row.user_id)

    return ApiResponse(success=True, message="角色权限已更新")


# ============================================================
# 删除角色
# ============================================================

@router.delete("/roles/{role_id}", summary="删除角色")
async def delete_role(
    role_id: int,
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:manage_roles")),
):
    role = await db.get(Role, role_id)
    if not role:
        return ApiResponse(success=False, error="角色不存在")
    if role.is_system:
        return ApiResponse(success=False, error="系统角色不可删除")

    # 查询受影响的用户
    affected = await db.execute(
        select(UserRole.user_id).where(UserRole.role_id == role_id)
    )
    user_ids = [row.user_id for row in affected]

    # 删除关联
    await db.execute(sa_delete(UserRole).where(UserRole.role_id == role_id))
    await db.delete(role)
    await db.commit()

    # 失效受影响用户的权限缓存
    for uid in user_ids:
        await invalidate_permission_cache(uid)

    return ApiResponse(success=True, message="角色已删除")


# ============================================================
# 权限列表
# ============================================================

@router.get("/permissions", summary="所有权限")
async def list_permissions(
    resource_type: Optional[str] = Query(None),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:view")),
):
    query = select(Capability).where(Capability.is_active == True)
    if resource_type:
        query = query.where(Capability.resource_type == resource_type)

    result = await db.execute(query.order_by(Capability.resource_type, Capability.action))
    caps = result.scalars().all()

    return ApiResponse(success=True, data={
        "permissions": [{
            "id": c.id,
            "code": c.code,
            "name": c.name,
            "resource_type": c.resource_type,
            "action": c.action,
        } for c in caps],
    })


# ============================================================
# 辅助函数
# ============================================================

def _role_to_dict(r: Role, user_count: int = 0) -> dict:
    return {
        "id": r.id,
        "name": r.name,
        "slug": r.slug,
        "description": r.description,
        "is_system": r.is_system,
        "is_active": r.is_active,
        "user_count": user_count,
        "permission_count": len(r.capabilities),
        "created_at": r.created_at.isoformat() if r.created_at else None,
    }
