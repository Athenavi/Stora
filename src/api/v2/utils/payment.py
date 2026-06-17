"""
支付相关API
"""
from datetime import datetime
from functools import wraps

from fastapi import APIRouter, Depends, Request
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import VIPPlan, VIPSubscription
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.utils.database.main import get_async_session

router = APIRouter(tags=["payment"])


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except Exception as e:
            return fail(str(e))
    return wrapper


@router.post("")
@_catch
async def create_payment(
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_session)
):
    """
    创建支付订单
    """
    # 获取请求数据
    data = await request.json()
    user_id = data.get('user_id')
    plan_id = data.get('plan_id')
    payment_method = data.get('payment_method')  # 'alipay' 或 'wechat'

    # 验证用户ID
    if not user_id or user_id != current_user.id:
        return fail("无效的用户ID")

    # 获取VIP套餐信息
    from sqlalchemy import select
    plan_query = select(VIPPlan).where(VIPPlan.id == plan_id, VIPPlan.is_active == True)
    plan_result = await db.execute(plan_query)
    plan = plan_result.scalar_one_or_none()
    if not plan:
        return fail("无效的套餐ID")

    # 检查用户是否已有有效订阅（含过期检查）
    from sqlalchemy import select
    from datetime import datetime
    existing_subscription_query = select(VIPSubscription).where(
        VIPSubscription.user == current_user.id,
        VIPSubscription.status == 1,
        VIPSubscription.expires_at > datetime.now(),
    )
    existing_subscription_result = await db.execute(existing_subscription_query)
    existing_subscription = existing_subscription_result.scalar_one_or_none()

    if existing_subscription:
        return fail("您已有有效的VIP订阅，无法重复购买")

    # 创建支付订单记录（这里简化处理，实际应根据支付方式调用对应支付网关）
    if payment_method == 'alipay':
        # 模拟支付宝支付URL生成
        pay_url = f"https://openapi.alipay.com/gateway.do?biz_content={{'out_trade_no':'VIP_{current_user.id}_{datetime.now().strftime('%Y%m%d%H%M%S')}', 'total_amount':'{plan.price}', 'subject':'VIP_{plan.name}'}}"
        return ok(data={
            "payment_data": {
                "pay_url": pay_url,
                "order_id": f"VIP_{current_user.id}_{datetime.now().strftime('%Y%m%d%H%M%S')}",
                "amount": float(plan.price),
                "description": f"购买VIP {plan.name}套餐"
            }
        })
    elif payment_method == 'wechat':
        # 模拟微信支付处理
        return ok(data={
            "payment_data": {
                "qr_code": "mock_wechat_qr_code_data",
                "order_id": f"VIP_{current_user.id}_{datetime.now().strftime('%Y%m%d%H%M%S')}",
                "amount": float(plan.price),
                "description": f"购买VIP {plan.name}套餐"
            }
        })
    else:
        return fail("不支持的支付方式")


@router.post("/alipay/notify")
@_catch
async def alipay_notify(
        request: Request,
        db: AsyncSession = Depends(get_async_session)
):
    """
    支付宝异步通知处理
    """
    # 这里应该验证支付宝签名和处理通知数据
    # 为了简化，这里只做模拟处理
    await request.form()

    # 模拟处理支付成功逻辑
    # 1. 验证订单号是否存在
    # 2. 验证金额是否正确
    # 3. 更新订单状态
    # 4. 创建VIP订阅记录

    return {"success": True}


@router.post("/wechat/notify")
@_catch
async def wechat_notify(
        request: Request,
        db: AsyncSession = Depends(get_async_session)
):
    """
    微信异步通知处理
    """
    # 这里应该验证微信支付签名和处理通知数据
    # 为了简化，这里只做模拟处理
    await request.json()

    # 模拟处理支付成功逻辑
    # 1. 验证订单号是否存在
    # 2. 验证金额是否正确
    # 3. 更新订单状态
    # 4. 创建VIP订阅记录

    return {"success": True}
