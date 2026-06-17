"""
V3 通知管理 API

权限要求:
  GET    /notifications/messages    → settings:view
  POST   /notifications/{id}/read   → settings:view
  POST   /notifications/read-all    → settings:view
  DELETE /notifications/{id}        → settings:view
  DELETE /notifications/clean       → settings:view
  POST   /notifications/email/send  → settings:edit
"""
import logging
from datetime import datetime, timezone
from typing import Optional

from fastapi import APIRouter, Body, Depends
from sqlalchemy import select, func, delete as sa_delete
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.notification import Notification
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db, get_current_user
from src.api.v3._permission import Permission
from shared.models.user import User as UserModel

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-notifications"])


@router.get("/notifications/messages", summary="通知列表")
async def list_notifications(
    limit: int = 50,
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    result = await db.execute(
        select(Notification).order_by(Notification.created_at.desc()).limit(limit)
    )
    items = result.scalars().all()
    return ApiResponse(success=True, data={
        "messages": [_notif_dict(n) for n in items],
        "total": len(items),
    })


@router.post("/notifications/{notification_id}/read", summary="标记已读")
async def mark_read(
    notification_id: int,
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    notif = await db.get(Notification, notification_id)
    if not notif:
        return ApiResponse(success=False, error="通知不存在")
    notif.is_read = True
    await db.commit()
    return ApiResponse(success=True, message="已标记已读")


@router.post("/notifications/read-all", summary="全部已读")
async def read_all(
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    await db.execute(
        Notification.__table__.update().values(is_read=True)
    )
    await db.commit()
    return ApiResponse(success=True, message="已全部标记已读")


@router.delete("/notifications/{notification_id}", summary="删除通知")
async def delete_notification(
    notification_id: int,
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    notif = await db.get(Notification, notification_id)
    if not notif:
        return ApiResponse(success=False, error="通知不存在")
    await db.delete(notif)
    await db.commit()
    return ApiResponse(success=True, message="已删除")


@router.delete("/notifications/clean", summary="清空通知")
async def clean_notifications(
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    await db.execute(sa_delete(Notification))
    await db.commit()
    return ApiResponse(success=True, message="已清空全部通知")


@router.post("/notifications/email/send", summary="发送邮件")
async def send_email(
    title: str = Body(...),
    content: str = Body(...),
    type: Optional[str] = Body(None),
    priority: Optional[str] = Body(None),
    to_user_id: Optional[int] = Body(None),
    db: AsyncSession = Depends(get_db),
    current_user: UserModel = Depends(Permission("settings:edit")),
):
    """发送站内通知"""
    now = datetime.now(timezone.utc)
    notif = Notification(
        recipient=to_user_id or current_user.id,
        type=type or 'admin',
        title=title,
        message=content,
        is_read=False,
        created_at=now,
    )
    db.add(notif)
    await db.commit()
    return ApiResponse(success=True, message="通知已发送")


def _notif_dict(n: Notification) -> dict:
    return {
        "id": n.id,
        "recipient": n.recipient,
        "type": n.type,
        "title": n.title,
        "message": n.message,
        "is_read": n.is_read,
        "created_at": n.created_at.isoformat() if n.created_at else None,
    }
