"""
屏幕选项(Screen Options) API
提供用户界面偏好的保存和加载
"""
from functools import wraps
from typing import Dict, Any, Optional

from fastapi import APIRouter, Depends, Query, Request, Body, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User
from shared.services.system.screen_options_service import screen_options_service
from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter()


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            return fail(str(e))
    return wrapper


@router.get("/options",
            summary="获取屏幕选项",
            description="获取当前用户的屏幕选项配置",
            response_description="返回选项配置")
@_catch
async def get_screen_options_api(
        request: Request,
        page_name: Optional[str] = Query(None, description="页面名称过滤"),
        db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(jwt_required)
):
    """
    获取屏幕选项API
    """
    if page_name:
        options = await screen_options_service.get_page_options(
            db, current_user.id, page_name
        )
    else:
        options = await screen_options_service.get_all_options(db, current_user.id)

    return ok(data=options)


@router.post("/options",
             summary="保存屏幕选项",
             description="保存用户的屏幕选项配置",
             response_description="返回保存结果")
@_catch
async def save_screen_option_api(
        request: Request,
        data: Dict[str, Any] = Body(...),
        db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(jwt_required)
):
    """
    保存屏幕选项API
    
    Request Body:
    {
        "page_name": "articles",
        "option_key": "columns",
        "value": ["title", "date", "author"]
    }
    """
    page_name = data.get('page_name')
    option_key = data.get('option_key')
    value = data.get('value')

    if not page_name or not option_key:
        return fail('缺少必需参数: page_name 和 option_key')

    await screen_options_service.set_option(
        db, current_user.id, page_name, option_key, value
    )
    await db.commit()

    return ok(
        msg='选项已保存',
        data={
            'page_name': page_name,
            'option_key': option_key,
            'value': value
        }
    )


@router.delete("/options",
               summary="删除屏幕选项",
               description="删除用户的屏幕选项配置",
               response_description="返回删除结果")
@_catch
async def delete_screen_option_api(
        request: Request,
        page_name: str = Query(..., description="页面名称"),
        option_key: str = Query(..., description="选项键名"),
        db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(jwt_required)
):
    """
    删除屏幕选项API
    """
    success = await screen_options_service.delete_option(
        db, current_user.id, page_name, option_key
    )
    await db.commit()

    if success:
        return ok(msg='选项已删除')
    else:
        return fail('选项不存在')


@router.post("/options/batch",
             summary="批量保存屏幕选项",
             description="批量保存多个屏幕选项",
             response_description="返回保存结果")
@_catch
async def batch_save_screen_options_api(
        request: Request,
        data: Dict[str, Any] = Body(...),
        db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(jwt_required)
):
    """
    批量保存屏幕选项API
    
    Request Body:
    {
        "page_name": "articles",
        "options": {
            "columns": ["title", "date", "author"],
            "per_page": 20,
            "sidebar_visible": true
        }
    }
    """
    page_name = data.get('page_name')
    options = data.get('options', {})

    if not page_name:
        return fail('缺少必需参数: page_name')

    saved_count = 0
    for option_key, value in options.items():
        await screen_options_service.set_option(
            db, current_user.id, page_name, option_key, value
        )
        saved_count += 1

    await db.commit()

    return ok(
        msg=f'成功保存{saved_count}个选项',
        data={
            'page_name': page_name,
            'saved_count': saved_count
        }
    )
