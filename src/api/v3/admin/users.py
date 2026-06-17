"""
V3 用户管理 API

权限要求:
  GET    /users           → user:view
  POST   /users           → user:create
  PUT    /users/{id}      → user:edit
  DELETE /users/{id}      → user:delete
  POST   /users/{id}/roles → user:manage_roles

路由函数内无权限查询 — 全部由 Depends(Permission) 在函数执行前完成。
"""
import logging
from datetime import datetime, timezone
from typing import Optional

from fastapi import APIRouter, Body, Depends, HTTPException, Query, status
from sqlalchemy import select, func, delete as sa_delete
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User
from shared.models.rbac import UserRole, Role
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db, get_current_user
from src.api.v3._permission import Permission, invalidate_permission_cache

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-users"])


# ============================================================
# 列表
# ============================================================

@router.get("/users", summary="用户列表")
async def list_users(
    page: int = Query(1, ge=1),
    per_page: int = Query(20, ge=1, le=100),
    search: Optional[str] = Query(None),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:view")),
):
    """获取用户列表（分页、搜索）"""
    query = select(User)

    if search:
        query = query.where(
            User.username.ilike(f"%{search}%") |
            User.email.ilike(f"%{search}%")
        )

    # 总数
    total = await db.scalar(
        select(func.count()).select_from(query.subquery())
    ) or 0

    # 分页
    offset = (page - 1) * per_page
    result = await db.execute(
        query.order_by(User.date_joined.desc()).offset(offset).limit(per_page)
    )
    users = result.scalars().all()

    return ApiResponse(success=True, data={
        "users": [_user_to_dict(u) for u in users],
        "pagination": {
            "page": page,
            "per_page": per_page,
            "total": total,
            "total_pages": (total + per_page - 1) // per_page,
        },
    })


# ============================================================
# 详情
# ============================================================

@router.get("/users/{user_id}", summary="用户详情")
async def get_user(
    user_id: int,
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:view")),
):
    user = await db.get(User, user_id)
    if not user:
        return ApiResponse(success=False, error="用户不存在")

    # 获取角色
    roles_result = await db.execute(
        select(Role).join(UserRole, UserRole.role_id == Role.id)
        .where(UserRole.user_id == user_id)
    )
    roles = [{"id": r.id, "name": r.name, "slug": r.slug} for r in roles_result.scalars().all()]

    data = _user_to_dict(user)
    data["roles"] = roles
    return ApiResponse(success=True, data=data)


# ============================================================
# 创建
# ============================================================

@router.post("/users", summary="创建用户", status_code=201)
async def create_user(
    username: str = Body(...),
    email: str = Body(...),
    password: str = Body(...),
    is_active: bool = Body(True),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:create")),
):
    # 检查重复
    existing = await db.execute(
        select(User).where((User.username == username) | (User.email == email))
    )
    if existing.scalar_one_or_none():
        return ApiResponse(success=False, error="用户名或邮箱已存在")

    from src.utils.security.password_validator import hash_password

    now = datetime.now(timezone.utc)
    user = User(
        username=username,
        email=email,
        password=hash_password(password),
        is_active=is_active,
        is_superuser=False,
        is_staff=False,
        date_joined=now,
        locale="zh_CN",
    )
    db.add(user)
    await db.commit()
    await db.refresh(user)

    return ApiResponse(success=True, data=_user_to_dict(user), message="用户创建成功")


# ============================================================
# 编辑
# ============================================================

@router.put("/users/{user_id}", summary="编辑用户")
async def update_user(
    user_id: int,
    username: Optional[str] = Body(None),
    email: Optional[str] = Body(None),
    is_active: Optional[bool] = Body(None),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("user:edit")),
):
    user = await db.get(User, user_id)
    if not user:
        return ApiResponse(success=False, error="用户不存在")

    if username is not None:
        user.username = username
    if email is not None:
        user.email = email
    if is_active is not None:
        user.is_active = is_active

    user.updated_at = datetime.now(timezone.utc)
    await db.commit()
    await db.refresh(user)

    return ApiResponse(success=True, data=_user_to_dict(user), message="用户更新成功")


# ============================================================
# 删除
# ============================================================

@router.delete("/users/{user_id}", summary="删除用户")
async def delete_user(
    user_id: int,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(Permission("user:delete")),
):
    if user_id == current_user.id:
        return ApiResponse(success=False, error="不能删除自己")

    user = await db.get(User, user_id)
    if not user:
        return ApiResponse(success=False, error="用户不存在")

    # 删除用户角色关联
    await db.execute(
        sa_delete(UserRole).where(UserRole.user_id == user_id)
    )
    await db.delete(user)
    await db.commit()

    return ApiResponse(success=True, message="用户已删除")


# ============================================================
# 角色分配
# ============================================================

@router.post("/users/{user_id}/roles", summary="分配角色")
async def assign_roles(
    user_id: int,
    role_ids: list[int] = Body(...),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(Permission("user:manage_roles")),
):
    user = await db.get(User, user_id)
    if not user:
        return ApiResponse(success=False, error="用户不存在")

    # 先添加新角色（检查重复），再移除不在此次分配中的旧角色
    now = datetime.now(timezone.utc)
    new_role_ids = set(role_ids)

    # 获取当前已分配的角色的 ID
    existing_result = await db.execute(
        select(UserRole.role_id).where(UserRole.user_id == user_id)
    )
    existing_role_ids = {row[0] for row in existing_result.fetchall()}

    # 1. 添加新角色（跳过已存在的）
    for rid in new_role_ids:
        if rid not in existing_role_ids:
            role = await db.get(Role, rid)
            if role:
                db.add(UserRole(
                    user_id=user_id,
                    role_id=rid,
                    assigned_by=current_user.id,
                    created_at=now,
                ))

    # 2. 移除不再是目标角色的旧角色
    roles_to_remove = existing_role_ids - new_role_ids
    if roles_to_remove:
        await db.execute(
            sa_delete(UserRole).where(
                UserRole.user_id == user_id,
                UserRole.role_id.in_(roles_to_remove)
            )
        )

    await db.commit()

    # 失效权限缓存
    await invalidate_permission_cache(user_id)

    return ApiResponse(success=True, message="角色分配成功")


# ============================================================
# 辅助函数
# ============================================================

def _user_to_dict(u: User) -> dict:
    return {
        "id": u.id,
        "username": u.username,
        "email": u.email,
        "is_active": u.is_active,
        "is_superuser": u.is_superuser,
        "is_staff": u.is_staff,
        "vip_level": getattr(u, 'vip_level', 0),
        "profile_picture": u.profile_picture,
        "date_joined": u.date_joined.isoformat() if u.date_joined else None,
        "last_login_at": u.last_login_at.isoformat() if u.last_login_at else None,
    }
