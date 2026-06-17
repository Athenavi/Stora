"""
角色权限管理 API
提供细粒度权限控制、自定义角色、权限继承和审计功能
"""
import json
import logging
from datetime import datetime, timezone
from functools import wraps
from typing import Optional, List

from fastapi import APIRouter, Depends, HTTPException, Query, Body
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import PermissionAuditLog
from shared.models.rbac import Role, Capability, RoleCapability, UserRole
from shared.services.security.rbac_service import rbac_service
from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db
from src.api.v3._permission import invalidate_permission_cache

router = APIRouter(tags=["rbac"])
logger = logging.getLogger(__name__)


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"[{func.__name__}] {e}")
            return fail(str(e))
    return wrapper


# ==================== 角色管理 ====================

@router.post("/roles", summary="创建自定义角色")
@_catch
async def create_role(
        name: str = Body(..., description="角色名称"),
        slug: str = Body(..., description="角色标识"),
        description: Optional[str] = Body(None, description="描述"),
        permission_codes: Optional[List[str]] = Body(None, description="权限代码列表"),
        parent_role_id: Optional[int] = Body(None, description="父角色ID（用于继承）"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """创建自定义角色"""
    # 权限检查
    if not await rbac_service.has_permission(db, current_user.id, 'user', 'manage_roles'):
        return fail("Insufficient permissions")

    now = datetime.now(timezone.utc)
    role = Role(
        name=name,
        slug=slug,
        description=description or '',
        is_system=False,
        parent_id=parent_role_id,
        is_active=True,
        created_at=now,
        updated_at=now,
    )
    db.add(role)
    await db.flush()
    await db.refresh(role)

    # 分配权限
    if permission_codes:
        cap_stmt = select(Capability).where(Capability.code.in_(permission_codes))
        cap_result = await db.execute(cap_stmt)
        role.capabilities = list(cap_result.scalars().all())

    # 审计日志
    await _log_audit(db, current_user.id, 'create_role', 'role', role.id,
                     {'name': name, 'slug': slug})
    await db.commit()

    return ok(data={
        'id': role.id,
        'name': role.name,
        'slug': role.slug,
        'description': role.description,
        'is_system': role.is_system,
        'parent_id': role.parent_id,
        'permission_count': len(role.capabilities),
    }, msg="Role created successfully")


@router.get("/roles", summary="获取角色列表")
@_catch
async def get_roles(
        include_system: bool = Query(True, description="是否包含系统角色"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """获取所有角色列表（含用户计数）"""
    query = select(Role)
    if not include_system:
        query = query.where(Role.is_system == False)

    result = await db.execute(query)
    roles = result.scalars().all()

    # 每个角色的用户数
    count_result = await db.execute(
        select(UserRole.role_id, func.count(UserRole.user_id).label('cnt'))
        .group_by(UserRole.role_id)
    )
    user_counts = {row.role_id: row.cnt for row in count_result}

    roles_list = []
    for role in roles:
        roles_list.append({
            'id': role.id,
            'name': role.name,
            'slug': role.slug,
            'description': role.description,
            'is_system': role.is_system,
            'parent_id': role.parent_id,
            'permission_count': len(role.capabilities),
            'user_count': user_counts.get(role.id, 0),
            'created_at': role.created_at.isoformat() if role.created_at else None,
        })

    return ok(data={'roles': roles_list, 'total': len(roles_list)})


@router.put("/roles/{role_id}/permissions", summary="更新角色权限")
@_catch
async def update_role_permissions(
        role_id: int,
        permission_codes: List[str] = Body(..., description="权限代码列表"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """更新角色权限"""
    if not await rbac_service.has_permission(db, current_user.id, 'user', 'manage_roles'):
        return fail("Insufficient permissions")

    role = await db.get(Role, role_id)
    if not role:
        return fail("Role not found")
    if role.is_system:
        return fail("Cannot modify system role permissions")

    # 清空后重新添加
    role.capabilities = []
    if permission_codes:
        cap_stmt = select(Capability).where(Capability.code.in_(permission_codes))
        cap_result = await db.execute(cap_stmt)
        role.capabilities = list(cap_result.scalars().all())

    role.updated_at = datetime.now(timezone.utc)

    await _log_audit(db, current_user.id, 'update_role_permissions', 'role', role_id,
                     {'permission_codes': permission_codes})
    await db.commit()

    # 失效该角色所有用户的权限缓存
    affected_urs = await db.execute(
        select(UserRole).where(UserRole.role_id == role_id)
    )
    for ur in affected_urs.scalars().all():
        await invalidate_permission_cache(ur.user_id)

    return ok(msg="Role permissions updated successfully")


@router.delete("/roles/{role_id}", summary="删除角色")
@_catch
async def delete_role(
        role_id: int,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """删除角色（仅自定义角色）"""
    if not await rbac_service.has_permission(db, current_user.id, 'user', 'manage_roles'):
        return fail("Insufficient permissions")

    role = await db.get(Role, role_id)
    if not role:
        return fail("Role not found")
    if role.is_system:
        return fail("Cannot delete system role")

    # 移除所有用户的该角色关联
    user_roles = await db.execute(
        select(UserRole).where(UserRole.role_id == role_id)
    )
    affected_uids = [ur.user_id for ur in user_roles.scalars().all()]
    for ur in user_roles.scalars().all():
        await db.delete(ur)

    await db.delete(role)

    await _log_audit(db, current_user.id, 'delete_role', 'role', role_id)
    await db.commit()

    # 失效受影响用户的权限缓存
    for uid in affected_uids:
        await invalidate_permission_cache(uid)

    return ok(msg="Role deleted successfully")


# ==================== 权限管理 ====================

@router.get("/permissions", summary="获取权限列表")
@_catch
async def get_permissions(
        resource_type: Optional[str] = Query(None, description="资源类型过滤"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """获取所有权限列表"""
    query = select(Capability).where(Capability.is_active == True)
    if resource_type:
        query = query.where(Capability.resource_type == resource_type)

    result = await db.execute(query)
    capabilities = result.scalars().all()

    perms_list = [{
        'id': cap.id,
        'name': cap.name,
        'code': cap.code,
        'description': cap.description,
        'resource_type': cap.resource_type,
        'action': cap.action,
    } for cap in capabilities]

    return ok(data={'permissions': perms_list, 'total': len(perms_list)})


# ==================== 用户角色分配 ====================

@router.post("/users/{user_id}/roles", summary="为用户分配角色")
@_catch
async def assign_role_to_user(
        user_id: int,
        role_id: int = Body(..., description="角色ID"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """为用户分配角色"""
    if not await rbac_service.has_permission(db, current_user.id, 'user', 'manage_roles'):
        return fail("Insufficient permissions")

    role = await db.get(Role, role_id)
    if not role:
        return fail("Role not found")

    await rbac_service.assign_role(db, user_id, role.slug, assigned_by=current_user.id)

    await _log_audit(db, current_user.id, 'assign_role', 'user', user_id,
                     {'role_id': role_id, 'role_slug': role.slug})
    await db.commit()

    # 失效该用户的权限缓存
    await invalidate_permission_cache(user_id)

    return ok(msg="Role assigned successfully")


@router.delete("/users/{user_id}/roles/{role_id}", summary="从用户移除角色")
@_catch
async def remove_role_from_user(
        user_id: int,
        role_id: int,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """从用户移除角色"""
    if not await rbac_service.has_permission(db, current_user.id, 'user', 'manage_roles'):
        return fail("Insufficient permissions")

    role = await db.get(Role, role_id)
    if not role:
        return fail("Role not found")

    await rbac_service.remove_role(db, user_id, role.slug)

    await _log_audit(db, current_user.id, 'remove_role', 'user', user_id,
                     {'role_id': role_id, 'role_slug': role.slug})
    await db.commit()

    # 失效该用户的权限缓存
    await invalidate_permission_cache(user_id)

    return ok(msg="Role removed successfully")


@router.get("/users/{user_id}/permissions", summary="获取用户权限")
@_catch
async def get_user_permissions(
        user_id: int,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """获取用户的所有权限代码列表"""
    codes = await rbac_service.get_user_permission_codes(db, user_id)

    return ok(data={
        'user_id': user_id,
        'permissions': codes,
        'total': len(codes),
    })


@router.post("/check-permission", summary="检查权限")
@_catch
async def check_permission(
        user_id: int = Body(..., description="用户ID"),
        permission_code: str = Body(..., description="权限代码 (格式: resource.action)"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """检查用户是否有指定权限"""
    has = await rbac_service.has_capability(db, user_id, permission_code)

    return ok(data={
        'user_id': user_id,
        'permission_code': permission_code,
        'has_permission': has,
    })


# ==================== 审计日志 ====================

@router.get("/audit-logs", summary="获取权限审计日志")
@_catch
async def get_audit_logs(
        user_id: Optional[int] = Query(None, description="用户ID过滤"),
        action: Optional[str] = Query(None, description="操作类型过滤"),
        page: int = Query(1, ge=1, description="页码"),
        per_page: int = Query(20, ge=1, le=100, description="每页数量"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """获取权限变更审计日志"""
    query = select(PermissionAuditLog)

    if user_id:
        query = query.where(PermissionAuditLog.user_id == user_id)
    if action:
        query = query.where(PermissionAuditLog.action == action)

    # 总数
    total_result = await db.execute(
        select(func.count()).select_from(query.subquery())
    )
    total = total_result.scalar()

    # 分页
    offset = (page - 1) * per_page
    query = query.offset(offset).limit(per_page).order_by(
        PermissionAuditLog.created_at.desc()
    )

    result = await db.execute(query)
    logs = result.scalars().all()

    logs_list = [{
        'id': log.id,
        'user_id': log.user_id,
        'action': log.action,
        'resource_type': log.resource_type,
        'resource_id': log.resource_id,
        'details': log.details,
        'ip_address': log.ip_address,
        'created_at': log.created_at.isoformat() if log.created_at else None,
    } for log in logs]

    return ok(data={
        'logs': logs_list,
        'pagination': {
            'current_page': page,
            'per_page': per_page,
            'total': total,
            'total_pages': (total + per_page - 1) // per_page,
        },
    })


# ==================== 辅助函数 ====================

async def _log_audit(
    db: AsyncSession, user_id: int, action: str,
    resource_type: str, resource_id: int, details: Optional[dict] = None,
):
    """写入权限审计日志"""
    log = PermissionAuditLog(
        user_id=user_id,
        action=action,
        resource_type=resource_type,
        resource_id=str(resource_id),
        details=json.dumps(details, ensure_ascii=False) if details else None,
        created_at=datetime.now(timezone.utc),
    )
    db.add(log)
    # 注意：不在这里 commit，由调用方统一 commit
