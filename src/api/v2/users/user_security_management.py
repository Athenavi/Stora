"""
用户安全管理 API - V2 优化版

字段权限 / 会话管理 / 邮件订阅 三个 CRUD 组
优化: 统一 error decorator, 消除 13 处重复 try/except
"""
from functools import wraps
from typing import Callable

from fastapi import APIRouter
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from src.api.v2._helpers import fail

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
