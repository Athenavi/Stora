"""
登录安全 API
提供异常检测、频繁失败锁定、设备指纹识别等功能
"""
import logging
from functools import wraps

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User as UserModel
from shared.services.users.login_security_service import login_security_service
from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import get_current_active_user, admin_required as admin_required_api
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["security"])
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


# 注意：以下旧API端点已弃用，因为登录安全服务已重构为基于数据库的异步实现
# 新的实现在登录流程中自动记录，无需单独调用

# @router.post("/check-lock", summary="检查账户锁定状态")
# async def check_account_lock(...):
#     ...


@router.get("/my-history", summary="获取我的登录历史")
@_catch
async def get_my_login_history(
        limit: int = Query(50, ge=1, le=200, description="返回数量"),
        current_user: UserModel = Depends(get_current_active_user),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取当前用户的登录历史记录
    
    Args:
        limit: 返回数量(1-200)
        
    Returns:
        登录历史列表
    """
    history = await login_security_service.get_login_history_async(
        username=current_user.username,
        limit=limit,
        db=db
    )

    return ok(data={
        'history': history,
        'count': len(history),
    })


@router.get("/my-stats", summary="获取我的安全统计")
@_catch
async def get_my_security_stats(
        current_user: UserModel = Depends(get_current_active_user),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取当前用户的安全统计数据
    
    Returns:
        统计数据
    """
    stats = await login_security_service.get_security_stats_async(
        username=current_user.username,
        db=db
    )

    return ok(data=stats)


# 管理员接口

@router.get("/admin/locked-users", summary="获取被锁定的用户列表")
@_catch
async def get_locked_users(
        current_user: UserModel = Depends(admin_required_api),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取所有当前被锁定的用户
    
    Returns:
        锁定用户列表
    """
    locked_users = await login_security_service.get_locked_users_async(db=db)

    return ok(data={
        'locked_users': locked_users,
        'count': len(locked_users),
    })


# @router.post("/admin/unlock", summary="手动解锁用户")
# async def unlock_user(...):
#     ...


@router.get("/admin/user-history/{username}", summary="查看用户登录历史")
@_catch
async def admin_get_user_history(
        username: str,
        limit: int = Query(50, ge=1, le=200, description="返回数量"),
        current_user: UserModel = Depends(admin_required_api),
        db: AsyncSession = Depends(get_async_db)
):
    """
    管理员查看指定用户的登录历史
    
    Args:
        username: 目标用户名
        limit: 返回数量
        
    Returns:
        登录历史列表
    """
    history = await login_security_service.get_login_history_async(
        username=username,
        limit=limit,
        db=db
    )

    return ok(data={
        'username': username,
        'history': history,
        'count': len(history),
    })


@router.get("/admin/user-stats/{username}", summary="查看用户安全统计")
@_catch
async def admin_get_user_stats(
        username: str,
        current_user: UserModel = Depends(admin_required_api),
        db: AsyncSession = Depends(get_async_db)
):
    """
    管理员查看指定用户的安全统计
    
    Args:
        username: 目标用户名
        
    Returns:
        统计数据
    """
    stats = await login_security_service.get_security_stats_async(
        username=username,
        db=db
    )

    return ok(data=stats)
