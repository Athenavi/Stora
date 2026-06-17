"""
用户安全管理 API - V2 优化版

字段权限 / 会话管理 / 邮件订阅 三个 CRUD 组
优化: 统一 error decorator, 消除 13 处重复 try/except
"""
from datetime import datetime
from functools import wraps
from typing import Any, Callable

from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, delete, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.security import FieldPermission
from shared.models.user import UserSession
from shared.models.notification import EmailSubscription
from src.api.v2._base import ApiResponse
from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import admin_required
from src.utils.database.main import get_async_session as get_async_db

router = APIRouter(tags=["user-security"])


def _with_db(func: Callable) -> Callable:
    """统一错误处理"""
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except Exception as e:
            import traceback
            traceback.print_exc()
            return fail(str(e))
    return wrapper


async def _get_or_404(db: AsyncSession, model, pk: int, msg: str = "记录不存在"):
    """获取对象或返回 404 响应"""
    obj = await db.scalar(select(model).where(model.id == pk))
    if not obj:
        return fail(msg)
    return obj


# ==================== 字段权限 CRUD ====================

@router.get("/field-permissions")
@_with_db
async def list_field_permissions(page: int = Query(1, ge=1), per_page: int = Query(20, ge=1, le=100),
                                  db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    total = await db.scalar(select(func.count(FieldPermission.id))) or 0
    rows = (await db.execute(select(FieldPermission).offset((page-1)*per_page).limit(per_page))).scalars().all()
    return ok(data=[r.to_dict() if hasattr(r, 'to_dict') else {'id': r.id, 'role_id': r.role_id,
                'model_name': r.model_name, 'field_name': r.field_name,
                'can_read': r.can_read, 'can_write': r.can_write} for r in rows],
              msg=f"共{total}条")


@router.get("/field-permissions/{perm_id}")
@_with_db
async def get_field_permission(perm_id: int, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, FieldPermission, perm_id)
    return obj if isinstance(obj, ApiResponse) else ok(data={'id': obj.id, 'role_id': obj.role_id,
        'model_name': obj.model_name, 'field_name': obj.field_name,
        'can_read': obj.can_read, 'can_write': obj.can_write})


@router.post("/field-permissions")
@_with_db
async def create_field_permission(data: dict, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    fp = FieldPermission(**{k: data[k] for k in ('role_id', 'model_name', 'field_name', 'can_read', 'can_write') if k in data})
    db.add(fp)
    await db.commit()
    return ok(data={'id': fp.id}, msg="字段权限创建成功")


@router.put("/field-permissions/{perm_id}")
@_with_db
async def update_field_permission(perm_id: int, data: dict, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, FieldPermission, perm_id)
    if isinstance(obj, ApiResponse):
        return obj
    for k, v in data.items():
        if hasattr(obj, k) and k != 'id':
            setattr(obj, k, v)
    await db.commit()
    return ok(msg="字段权限更新成功")


@router.delete("/field-permissions/{perm_id}")
@_with_db
async def delete_field_permission(perm_id: int, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, FieldPermission, perm_id)
    if isinstance(obj, ApiResponse):
        return obj
    await db.delete(obj)
    await db.commit()
    return ok(msg="字段权限已删除")


# ==================== 会话管理 ====================

@router.get("/sessions")
@_with_db
async def list_sessions(page: int = Query(1, ge=1), per_page: int = Query(20, ge=1, le=100),
                        db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    total = await db.scalar(select(func.count(UserSession.id))) or 0
    rows = (await db.execute(select(UserSession).offset((page-1)*per_page).limit(per_page))).scalars().all()
    return ok(data=[{'id': r.id, 'user_id': r.user_id, 'ip_address': r.ip_address,
                'created_at': r.created_at.isoformat() if r.created_at else None} for r in rows],
              msg=f"共{total}条")


@router.get("/sessions/{session_id}")
@_with_db
async def get_session(session_id: int, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, UserSession, session_id, "会话不存在")
    return obj if isinstance(obj, ApiResponse) else ok(data={'id': obj.id, 'user_id': obj.user_id,
        'ip_address': obj.ip_address, 'created_at': obj.created_at.isoformat() if obj.created_at else None})


@router.put("/sessions/{session_id}/deactivate")
@_with_db
async def deactivate_session(session_id: int, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, UserSession, session_id, "会话不存在")
    if isinstance(obj, ApiResponse):
        return obj
    setattr(obj, 'is_active', False)
    await db.commit()
    return ok(msg="会话已停用")


@router.delete("/sessions/{session_id}")
@_with_db
async def delete_session(session_id: int, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    await db.execute(delete(UserSession).where(UserSession.id == session_id))
    await db.commit()
    return ok(msg="会话已删除")


# ==================== 邮件订阅管理 ====================

@router.get("/email-subscriptions")
@_with_db
async def list_email_subscriptions(page: int = Query(1, ge=1), per_page: int = Query(20, ge=1, le=100),
                                    db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    total = await db.scalar(select(func.count(EmailSubscription.id))) or 0
    rows = (await db.execute(select(EmailSubscription).offset((page-1)*per_page).limit(per_page))).scalars().all()
    return ok(data=[{'id': r.id, 'email': r.email, 'subscribed': r.subscribed,
                'created_at': r.created_at.isoformat() if r.created_at else None} for r in rows])


@router.post("/email-subscriptions")
@_with_db
async def create_email_subscription(data: dict, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    sub = EmailSubscription(**{k: data[k] for k in ('email', 'subscribed') if k in data})
    db.add(sub)
    await db.commit()
    return ok(data={'id': sub.id}, msg="订阅创建成功")


@router.put("/email-subscriptions/{sub_id}")
@_with_db
async def update_email_subscription(sub_id: int, data: dict, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, EmailSubscription, sub_id, "订阅不存在")
    if isinstance(obj, ApiResponse):
        return obj
    if 'subscribed' in data:
        obj.subscribed = bool(data['subscribed'])
    await db.commit()
    return ok(msg="订阅已更新")


@router.delete("/email-subscriptions/{sub_id}")
@_with_db
async def delete_email_subscription(sub_id: int, db: AsyncSession = Depends(get_async_db), _=Depends(admin_required)):
    obj = await _get_or_404(db, EmailSubscription, sub_id, "订阅不存在")
    if isinstance(obj, ApiResponse):
        return obj
    await db.delete(obj)
    await db.commit()
    return ok(msg="订阅已删除")
