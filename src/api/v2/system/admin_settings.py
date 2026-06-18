import json
from functools import wraps
from typing import Dict, Any

from fastapi import APIRouter, Depends, Request, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.system import SystemSettings
from src.auth import jwt_required
from src.api.v2._helpers import ok, fail
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["admin-settings"])


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


async def get_system_settings_dict(db: AsyncSession) -> Dict[str, str]:
    """
    获取系统设置字典
    """
    from sqlalchemy import select

    settings_query = select(SystemSettings)
    settings_result = await db.execute(settings_query)
    settings = settings_result.scalars().all()
    settings_dict = {}
    for setting in settings:
        try:
            parsed_value = json.loads(setting.setting_value)
            if isinstance(parsed_value, str) and setting.setting_value.startswith(
                '"') and setting.setting_value.endswith('"'):
                settings_dict[setting.setting_key] = parsed_value
            else:
                settings_dict[setting.setting_key] = parsed_value
        except (json.JSONDecodeError, TypeError):
            settings_dict[setting.setting_key] = setting.setting_value
    return settings_dict


async def update_system_setting(key: str, value: Any, db: AsyncSession) -> None:
    """
    更新或创建系统设置
    """
    if isinstance(value, (str, int, float)):
        serialized_value = str(value)
    elif isinstance(value, bool):
        serialized_value = json.dumps(value, ensure_ascii=False)
    elif value is None:
        serialized_value = json.dumps(value, ensure_ascii=False)
    else:
        try:
            serialized_value = json.dumps(value, ensure_ascii=False)
        except (TypeError, ValueError):
            serialized_value = str(value)

    from sqlalchemy import select
    existing_setting_query = select(SystemSettings).where(SystemSettings.setting_key == key)
    existing_setting_result = await db.execute(existing_setting_query)
    existing_setting = existing_setting_result.scalar_one_or_none()

    if existing_setting:
        from datetime import datetime
        existing_setting.setting_value = serialized_value
        existing_setting.updated_at = datetime.now()
        await db.commit()
    else:
        from datetime import datetime
        new_setting = SystemSettings(
            setting_key=key,
            setting_value=serialized_value,
            description=get_setting_description(key),
            updated_at=datetime.now()
        )
        db.add(new_setting)
        await db.commit()


def get_setting_description(key: str) -> str:
    """
    获取设置键的描述
    """
    descriptions = {
        'site_title': '站点标题',
        'site_img': '站点图像URL',
        'site_description': '站点描述',
        'site_domain': '站点域名',
        'site_beian': '备案号',
        'site_keywords': '站点关键词',
        'user_registration': '允许用户注册',
        'menu_slug': '当前使用的菜单标识',
        'home_hero_title': '首页英雄区域标题',
        'home_hero_subtitle': '首页英雄区域副标题',
        'home_hero_cta_text': '首页英雄区域CTA按钮文本',
        'home_hero_cta_link': '首页英雄区域CTA按钮链接',
        'home_cta_target': '首页CTA按钮跳转方式',
        'home_featured_count': '首页特色文章数量',
        'home_hero_background_image': '首页英雄区域背景图片',
        'home_featured_title': '首页特色文章区域标题',
        'home_featured_empty_title': '首页特色文章区域空状态标题',
        'home_featured_empty_subtitle': '首页特色文章区域空状态副标题',
        'home_main_title': '首页主要内容区域标题',
        'home_main_filter_buttons': '首页主要内容区域过滤按钮',
        'home_main_empty_title': '首页主要内容区域空状态标题',
        'home_main_empty_subtitle': '首页主要内容区域空状态副标题',
        'home_newsletter_title': '首页新闻通讯区域标题',
        'home_newsletter_subtitle': '首页新闻通讯区域副标题',
        'home_newsletter_placeholder': '首页新闻通讯区域输入框占位符',
        'home_newsletter_button_text': '首页新闻通讯区域按钮文本',
        'home_no_category_msg': '首页无分类消息',
        'home_unknown_author_msg': '首页未知作者消息',
        'home_no_summary_msg': '首页无摘要消息',
    }
    return descriptions.get(key, f'{key} 设置')


@router.get("/")
@_catch
async def get_settings(
    request: Request,
    current_user=Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db)
):
    """
    获取所有系统设置
    """
    # 检查用户权限 - 只有超级用户才能访问
    from starlette.responses import RedirectResponse
    if isinstance(current_user, RedirectResponse):
        return current_user
    if not current_user.is_superuser:
        raise HTTPException(
            status_code=403,
            detail="The user doesn't have enough privileges"
        )

    # 获取系统设置
    settings_dict = await get_system_settings_dict(db)

    return ok(data={
        "settings": settings_dict,
    })


@router.post("/")
@_catch
async def update_settings(
    request: Request,
    current_user=Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db)
):
    """
    更新系统设置
    """
    # 检查用户权限 - 只有超级用户才能访问
    from starlette.responses import RedirectResponse
    if isinstance(current_user, RedirectResponse):
        return current_user
    if not current_user.is_superuser:
        raise HTTPException(
            status_code=403,
            detail="The user doesn't have enough privileges"
        )

    data = await request.json()
    settings = data.get('settings', {})
    action = data.get('action', 'update_settings')

    if action == 'update_settings':
        for key, value in settings.items():
            await update_system_setting(key, value, db)

        return ok(data={"message": "设置更新成功"})
    else:
        return fail("Invalid action")
