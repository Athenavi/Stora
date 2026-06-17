"""
Web Push 推送通知 API
提供订阅管理和推送发送功能
"""

from functools import wraps
from typing import Optional, List

from fastapi import APIRouter, Depends, HTTPException, Body, Request
from pydantic import BaseModel

from shared.models.user import User as UserModel
from shared.services.chat.web_push_service import web_push_service
from src.api.v2._helpers import ok, fail
from src.auth import get_current_active_user

router = APIRouter(tags=["push-notifications"])


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


class SubscriptionRequest(BaseModel):
    """订阅请求模型"""
    endpoint: str
    keys: dict
    user_agent: Optional[str] = None


class PushNotificationRequest(BaseModel):
    """推送通知请求模型"""
    title: str
    body: str
    data: Optional[dict] = None
    icon: Optional[str] = None
    badge: Optional[str] = None


@router.post("/subscribe", summary="订阅推送通知")
@_catch
async def subscribe_push(
        request: Request,
        subscription: SubscriptionRequest,
        current_user: UserModel = Depends(get_current_active_user)
):
    """
    用户订阅Web推送通知

    需要提供Service Worker生成的订阅信息。

    Args:
        subscription: 订阅数据(endpoint和keys)

    Returns:
        订阅结果
    """
    subscription_data = {
        'endpoint': subscription.endpoint,
        'keys': subscription.keys,
        'user_agent': subscription.user_agent or request.headers.get('User-Agent', ''),
    }

    success = web_push_service.subscribe_user(current_user.id, subscription_data)

    if success:
        return ok(
            msg='订阅成功',
            data={
                'subscribed': True,
            }
        )
    else:
        return fail('订阅失败')


@router.post("/unsubscribe", summary="取消订阅推送通知")
@_catch
async def unsubscribe_push(
        endpoint: Optional[str] = Body(None, embed=True, description="要取消的endpoint"),
        current_user: UserModel = Depends(get_current_active_user)
):
    """
    用户取消Web推送通知订阅

    Args:
        endpoint: 要取消的endpoint(可选,不提供则取消所有)

    Returns:
        取消结果
    """
    success = web_push_service.unsubscribe_user(current_user.id, endpoint)

    if success:
        return ok(
            msg='取消订阅成功',
            data={
                'subscribed': False,
            }
        )
    else:
        return fail('取消订阅失败')


@router.get("/subscription-status", summary="获取订阅状态")
@_catch
async def get_subscription_status(
        current_user: UserModel = Depends(get_current_active_user)
):
    """
    获取当前用户的推送订阅状态

    Returns:
        订阅状态和列表
    """
    subscriptions = web_push_service.get_user_subscriptions(current_user.id)

    return ok(data={
        'subscribed': len(subscriptions) > 0,
        'subscription_count': len(subscriptions),
        'subscriptions': subscriptions,
    })


@router.post("/send-notification", summary="发送推送通知")
@_catch
async def send_push_notification(
        notification: PushNotificationRequest,
        current_user: UserModel = Depends(get_current_active_user)
):
    """
    向当前用户发送推送通知

    Args:
        notification: 通知内容(title, body, data等)

    Returns:
        发送结果
    """
    result = web_push_service.send_push_notification(
        user_id=current_user.id,
        title=notification.title,
        body=notification.body,
        data=notification.data,
        icon=notification.icon,
        badge=notification.badge,
    )

    return ok(
        msg=result['message'],
        data={
            'sent_count': result['sent_count'],
            'total_subscriptions': result['total_subscriptions'],
        }
    )


@router.post("/send-to-users", summary="批量发送推送通知")
@_catch
async def send_push_to_users(
        notification: PushNotificationRequest,
        user_ids: List[int] = Body(..., embed=True, description="目标用户ID列表"),
        current_user: UserModel = Depends(get_current_active_user)
):
    """
    批量发送推送通知给指定用户

    需要管理员权限。

    Args:
        notification: 通知内容
        user_ids: 目标用户ID列表

    Returns:
        批量发送结果
    """
    if not getattr(current_user, 'is_superuser', False):
        raise HTTPException(status_code=403, detail="需要管理员权限")

    result = web_push_service.send_to_multiple_users(
        user_ids=user_ids,
        title=notification.title,
        body=notification.body,
        data=notification.data,
    )

    return ok(
        msg=f'发送给 {result["successful"]} 个用户成功',
        data=result
    )


@router.get("/stats", summary="获取推送统计")
@_catch
async def get_push_stats(
        current_user: UserModel = Depends(get_current_active_user)
):
    """
    获取推送通知统计信息

    Returns:
        统计数据
    """
    if not getattr(current_user, 'is_superuser', False):
        raise HTTPException(status_code=403, detail="需要管理员权限")

    stats = web_push_service.get_subscription_stats()

    return ok(data=stats)


@router.get("/vapid-public-key", summary="获取VAPID公钥")
@_catch
async def get_vapid_public_key():
    """
    获取VAPID公钥用于前端订阅

    Returns:
        VAPID公钥
    """
    public_key = web_push_service.vapid_config.get('public_key', '')

    if not public_key:
        return fail('VAPID密钥未配置,请联系管理员')

    return ok(data={
        'public_key': public_key,
    })
