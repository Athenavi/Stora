import json
from functools import wraps
from typing import Dict, Any

from fastapi import APIRouter, Depends, Request, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import SystemSettings, MenuItems, Pages, Menus
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_page_dependency as jwt_required
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
            if isinstance(parsed_value, str) and setting.setting_value.startswith('"') and setting.setting_value.endswith('"'):
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

    # 获取菜单
    from sqlalchemy import select
    menus_query = select(Menus).order_by(Menus.created_at.desc())
    menus_result = await db.execute(menus_query)
    menus = menus_result.scalars().all()

    # 获取所有菜单项并按菜单分组
    menu_items = {}
    for menu in menus:
        items_query = select(MenuItems).where(MenuItems.menu_id == menu.id).order_by(MenuItems.order_index)
        items_result = await db.execute(items_query)
        items = items_result.scalars().all()
        menu_items[menu.id] = items

    # 获取页面
    pages_query = select(Pages).order_by(Pages.created_at.desc())
    pages_result = await db.execute(pages_query)
    pages = pages_result.scalars().all()

    return ok(data={
        "settings": settings_dict,
        "menus": [{
            "id": menu.id,
            "name": menu.name,
            "slug": menu.slug,
            "description": menu.description,
            "is_active": menu.is_active,
            "created_at": menu.created_at.isoformat() if menu.created_at else None,
            "updated_at": menu.updated_at.isoformat() if menu.updated_at else None
        } for menu in menus],
        "menu_items": {
            str(menu_id): [{
                "id": item.id,
                "title": item.title,
                "url": item.url,
                "target": item.target,
                "parent_id": item.parent_id,
                "order_index": item.order_index,
                "created_at": item.created_at.isoformat() if item.created_at else None,
            } for item in items] for menu_id, items in menu_items.items()
        },
        "pages": [{
            "id": page.id,
            "title": page.title,
            "slug": page.slug,
            "content": page.content,
            "excerpt": page.excerpt,
            "template": page.template,
            "status": page.status,
            "parent_id": page.parent_id,
            "order_index": page.order_index,
            "meta_title": page.meta_title,
            "meta_description": page.meta_description,
            "meta_keywords": page.meta_keywords,
            "created_at": page.created_at.isoformat() if page.created_at else None,
        } for page in pages]
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


@router.post("/menus")
@_catch
async def create_menu(
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    创建菜单
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
    name = data.get('name')
    slug = data.get('slug')
    description = data.get('description', '')

    if not name or not slug:
        return fail('菜单名称和标识不能为空')

    from datetime import datetime

    from sqlalchemy import select
    existing_menu_query = select(Menus).where(Menus.slug == slug)
    existing_menu_result = await db.execute(existing_menu_query)
    existing_menu = existing_menu_result.scalar_one_or_none()
    if existing_menu:
        return fail('菜单标识已存在')

    menu = Menus(
        name=name,
        slug=slug,
        description=description,
        created_at=datetime.now(),
        updated_at=datetime.now()
    )
    db.add(menu)
    await db.commit()
    await db.refresh(menu)

    return ok(data={
        'id': menu.id,
        'name': menu.name,
        'slug': menu.slug,
        'description': menu.description,
        'is_active': menu.is_active,
        'created_at': menu.created_at.isoformat() if menu.created_at else None,
        'updated_at': menu.updated_at.isoformat() if menu.updated_at else None
    })


@router.put("/menus/{menu_id}")
@_catch
async def update_menu(
        menu_id: int,
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    更新菜单
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

    from datetime import datetime
    from sqlalchemy import select

    menu_query = select(Menus).where(Menus.id == menu_id)
    menu_result = await db.execute(menu_query)
    menu = menu_result.scalar_one_or_none()
    if not menu:
        return fail('菜单不存在')

    menu.name = data.get('name', menu.name)
    menu.slug = data.get('slug', menu.slug)
    menu.description = data.get('description', menu.description)
    menu.is_active = data.get('is_active', menu.is_active)
    menu.updated_at = datetime.now()

    await db.commit()
    await db.refresh(menu)

    return ok(data={
        'id': menu.id,
        'name': menu.name,
        'slug': menu.slug,
        'description': menu.description,
        'is_active': menu.is_active,
        'created_at': menu.created_at.isoformat() if menu.created_at else None,
        'updated_at': menu.updated_at.isoformat() if menu.updated_at else None
    })


@router.delete("/menus/{menu_id}")
@_catch
async def delete_menu(
        menu_id: int,
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    删除菜单
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

    from sqlalchemy import select

    menu_query = select(Menus).where(Menus.id == menu_id)
    menu_result = await db.execute(menu_query)
    menu = menu_result.scalar_one_or_none()
    if not menu:
        return fail('菜单不存在')

    # 检查菜单项
    menu_items_query = select(MenuItems).where(MenuItems.menu_id == menu_id)
    menu_items_result = await db.execute(menu_items_query)
    menu_items = menu_items_result.scalars().all()
    if menu_items:
        return fail('菜单下存在菜单项，请先删除所有菜单项')

    await db.delete(menu)
    await db.commit()

    return ok(data={"message": "菜单删除成功"})


@router.post("/pages")
@_catch
async def create_page(
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    创建页面
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
    title = data.get('title')
    slug = data.get('slug')

    if not title or not slug:
        return fail('页面标题和别名不能为空')

    from datetime import datetime

    from sqlalchemy import select
    existing_page_query = select(Pages).where(Pages.slug == slug)
    existing_page_result = await db.execute(existing_page_query)
    existing_page = existing_page_result.scalar_one_or_none()
    if existing_page:
        return fail('页面别名已存在')

    page = Pages(
        title=title,
        slug=slug,
        content=data.get('content', ''),
        excerpt=data.get('excerpt', ''),
        template=data.get('template', 'default'),
        status=int(data.get('status', 0)),
        parent_id=data.get('parent_id'),
        order_index=int(data.get('order_index', 0)),
        meta_title=data.get('meta_title'),
        meta_description=data.get('meta_description'),
        meta_keywords=data.get('meta_keywords'),
        author_id=current_user.id,
        created_at=datetime.now(),
        updated_at=datetime.now()
    )
    db.add(page)
    await db.commit()
    await db.refresh(page)

    return ok(data={
        'id': page.id,
        'title': page.title,
        'slug': page.slug,
        'content': page.content,
        'excerpt': page.excerpt,
        'template': page.template,
        'status': page.status,
        'parent_id': page.parent_id,
        'order_index': page.order_index,
        'meta_title': page.meta_title,
        'meta_description': page.meta_description,
        'meta_keywords': page.meta_keywords,
        'author': {
            'username': current_user.username
        },
        'created_at': page.created_at.isoformat() if page.created_at else None,
        'updated_at': page.updated_at.isoformat() if page.updated_at else None
    })


@router.put("/pages/{page_id}")
@_catch
async def update_page(
        page_id: int,
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    更新页面
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

    from datetime import datetime
    from sqlalchemy import select

    page_query = select(Pages).where(Pages.id == page_id)
    page_result = await db.execute(page_query)
    page = page_result.scalar_one_or_none()
    if not page:
        return fail('页面不存在')

    page.title = data.get('title', page.title)
    page.slug = data.get('slug', page.slug)
    page.content = data.get('content', page.content)
    page.excerpt = data.get('excerpt', page.excerpt)
    page.template = data.get('template', page.template)
    page.status = int(data.get('status', page.status or 0))
    page.parent_id = data.get('parent_id', page.parent_id)
    page.order_index = int(data.get('order_index', page.order_index or 0))
    page.meta_title = data.get('meta_title', page.meta_title)
    page.meta_description = data.get('meta_description', page.meta_description)
    page.meta_keywords = data.get('meta_keywords', page.meta_keywords)
    page.updated_at = datetime.now()

    await db.commit()
    await db.refresh(page)

    return ok(data={
        'id': page.id,
        'title': page.title,
        'slug': page.slug,
        'content': page.content,
        'excerpt': page.excerpt,
        'template': page.template,
        'status': page.status,
        'parent_id': page.parent_id,
        'order_index': page.order_index,
        'meta_title': page.meta_title,
        'meta_description': page.meta_description,
        'meta_keywords': page.meta_keywords,
        'author': {
            'username': current_user.username
        },
        'created_at': page.created_at.isoformat() if page.created_at else None,
        'updated_at': page.updated_at.isoformat() if page.updated_at else None
    })


@router.delete("/pages/{page_id}")
@_catch
async def delete_page(
        page_id: int,
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    删除页面
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

    from sqlalchemy import select
    page_query = select(Pages).where(Pages.id == page_id)
    page_result = await db.execute(page_query)
    page = page_result.scalar_one_or_none()
    if not page:
        return fail('页面不存在')

    await db.delete(page)
    await db.commit()

    return ok(data={"message": "页面删除成功"})


@router.post("/menu-items")
@_catch
async def create_menu_item(
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    创建菜单项
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
    title = data.get('title')
    url = data.get('url')
    menu_id = data.get('menu_id')

    if not title or not url or not menu_id:
        return fail('菜单项标题、链接和菜单ID不能为空')

    from datetime import datetime

    menu_item = MenuItems(
        title=title,
        url=url,
        menu_id=menu_id,
        target=data.get('target', '_self'),
        parent_id=data.get('parent_id'),
        order_index=int(data.get('order_index', 0)),
        is_active=bool(data.get('is_active', True)),
        created_at=datetime.now(),
    )
    db.add(menu_item)
    await db.commit()
    await db.refresh(menu_item)

    return ok(data={
        'id': menu_item.id,
        'title': menu_item.title,
        'url': menu_item.url,
        'target': menu_item.target,
        'parent_id': menu_item.parent_id,
        'order_index': menu_item.order_index,
        'is_active': menu_item.is_active,
        'created_at': menu_item.created_at.isoformat() if menu_item.created_at else None,
    })


@router.put("/menu-items/{menu_item_id}")
@_catch
async def update_menu_item(
        menu_item_id: int,
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    更新菜单项
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

    from datetime import datetime

    from sqlalchemy import select
    menu_item_query = select(MenuItems).where(MenuItems.id == menu_item_id)
    menu_item_result = await db.execute(menu_item_query)
    menu_item = menu_item_result.scalar_one_or_none()
    if not menu_item:
        return fail('菜单项不存在')

    menu_item.title = data.get('title', menu_item.title)
    menu_item.url = data.get('url', menu_item.url)
    menu_item.target = data.get('target', menu_item.target)
    menu_item.parent_id = data.get('parent_id', menu_item.parent_id)
    menu_item.order_index = int(data.get('order_index', menu_item.order_index or 0))
    menu_item.is_active = bool(data.get('is_active', menu_item.is_active))
    menu_item.updated_at = datetime.now()

    await db.commit()
    await db.refresh(menu_item)

    return ok(data={
        'id': menu_item.id,
        'title': menu_item.title,
        'url': menu_item.url,
        'target': menu_item.target,
        'parent_id': menu_item.parent_id,
        'order_index': menu_item.order_index,
        'is_active': menu_item.is_active,
        'created_at': menu_item.created_at.isoformat() if menu_item.created_at else None
    })


@router.delete("/menu-items/{menu_item_id}")
@_catch
async def delete_menu_item(
        menu_item_id: int,
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    删除菜单项
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

    from sqlalchemy import select
    menu_item_query = select(MenuItems).where(MenuItems.id == menu_item_id)
    menu_item_result = await db.execute(menu_item_query)
    menu_item = menu_item_result.scalar_one_or_none()
    if not menu_item:
        return fail('菜单项不存在')

    # 检查是否有子菜单项关联到此菜单项
    child_items_query = select(MenuItems).where(MenuItems.parent_id == menu_item_id)
    child_items_result = await db.execute(child_items_query)
    child_items = child_items_result.scalars().all()
    if child_items:
        return fail('菜单项下还有子菜单项，无法删除')

    await db.delete(menu_item)
    await db.commit()

    return ok(data={"message": "菜单项删除成功"})
