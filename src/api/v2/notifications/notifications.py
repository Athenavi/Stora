"""
通知相关API - 处理用户通知功能
"""
from functools import wraps

from fastapi import APIRouter, Request, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.notification import Notification
from shared.models.user import User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db
from src.notification import mark_notification_as_read, get_user_notifications, mark_all_notifications_as_read

router = APIRouter(tags=["notifications"])


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            import traceback
            traceback.print_exc()
            return fail(str(e))
    return wrapper


@router.post("/messages/read")
@_catch
async def read_notification_api(
        nid: int = Query(..., alias="nid"),
        current_user: User = Depends(jwt_required)
):
    """
    标记通知为已读API
    """
    result = await mark_notification_as_read(nid, current_user.id)
    if result:
        return ok(msg="通知已标记为已读")
    return fail("无法标记通知为已读")


@router.get("/messages")
@_catch
async def fetch_message_api(
        current_user: User = Depends(jwt_required)
):
    """
    获取用户通知API
    """
    result = await get_user_notifications(current_user.id)
    return ok(data=result)


@router.post("/messages/read_all")
@_catch
async def mark_all_as_read_api(
        current_user: User = Depends(jwt_required)
):
    """
    标记所有通知为已读API
    """
    result = await mark_all_notifications_as_read(current_user.id)
    return ok(data={"updated_count": result})


@router.delete("/messages/clean")
@_catch
async def clean_notification_api(
        request: Request,
        nid: str = Query("all", alias="nid"),
        current_user: User = Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    清理通知API
    """
    from sqlalchemy import delete
    if nid == 'all':
        stmt = delete(Notification).where(Notification.recipient == current_user.id)
        await db.execute(stmt)
    else:
        stmt = delete(Notification).where(
            Notification.recipient == current_user.id,
            Notification.id == int(nid)
        )
        await db.execute(stmt)
    await db.commit()
    return ok()


@router.patch("/{notification_id}/read")
@_catch
async def mark_notification_as_read_api(
        notification_id: int,
        current_user: User = Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    标记通知为已读API (新格式)
    """
    from src.notification import mark_notification_as_read
    # 注意：mark_notification_as_read 函数的参数顺序是 (notification_id, user_id)
    success = await mark_notification_as_read(notification_id, current_user.id)
    if success:
        return ok(msg="通知已标记为已读")
    else:
        return fail("无法标记通知为已读")


@router.delete("/{notification_id}")
@_catch
async def delete_notification_api(
        notification_id: int,
        current_user: User = Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    删除通知API
    """
    from src.notification import delete_notification
    success = await delete_notification(notification_id, current_user.id)
    if success:
        return ok(msg="通知已删除")
    else:
        return fail("无法删除通知")


@router.get("/")
@_catch
async def get_notifications_api(
        current_user: User = Depends(jwt_required)
):
    """
    获取用户通知API
    """
    notifications = await get_user_notifications(current_user.id)

    # 转换通知数据为前端期望的格式
    notification_list = []
    for notif in notifications:
        notification_list.append({
            "id": notif.id,
            "title": notif.title,
            "content": notif.message,
            "date": notif.created_at.isoformat(),
            "type": notif.type,
            "read": notif.is_read,
            "avatar": f"/avatars/user_{current_user.id}.jpg",
            "sender": "System",
            "recipient": current_user.id
        })

    return ok(data=notification_list)
