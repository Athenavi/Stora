"""
V3 系统管理 API

权限要求:
  GET    /settings       → settings:view
  PUT    /settings       → settings:edit
  POST   /backup         → backup:create
  POST   /backup/restore → backup:restore
  DELETE /backup/{id}    → backup:delete

路由函数内无权限查询。
"""
import logging
from datetime import datetime, timezone
from typing import Optional

from fastapi import APIRouter, Body, Depends
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.system import SystemSettings
# [REMOVED] menu model deleted in Stora refactor
# [REMOVED] menu model deleted in Stora refactor
# [REMOVED] page model deleted in Stora refactor
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db, get_current_user
from src.api.v3._permission import Permission

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-system"])


# ============================================================
# 设置查看
# ============================================================

@router.get("/settings", summary="查看设置")
async def get_settings(
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    """获取所有系统设置、菜单和独立页面"""
    # 系统设置
    settings_result = await db.execute(select(SystemSettings))
    settings = settings_result.scalars().all()

    # 菜单
    menus_result = await db.execute(select(Menus).order_by(Menus.created_at.desc()))
    menus = menus_result.scalars().all()

    # 菜单项（按菜单分组）
    menu_items = {}
    for menu in menus:
        items_result = await db.execute(
            select(MenuItems).where(MenuItems.menu_id == menu.id).order_by(MenuItems.order_index)
        )
        menu_items[str(menu.id)] = items_result.scalars().all()

    # 独立页面
    pages_result = await db.execute(select(Pages).order_by(Pages.created_at.desc()))
    pages = pages_result.scalars().all()

    return ApiResponse(success=True, data={
        "settings": {s.setting_key: s.setting_value for s in settings},
        "menus": [{
            "id": m.id,
            "name": m.name,
            "slug": m.slug,
            "description": m.description,
            "is_active": m.is_active,
            "created_at": m.created_at.isoformat() if m.created_at else None,
            "updated_at": m.updated_at.isoformat() if m.updated_at else None,
        } for m in menus],
        "menu_items": {
            menu_id: [{
                "id": item.id,
                "title": item.title,
                "url": item.url,
                "target": item.target,
                "parent_id": item.parent_id,
                "order_index": item.order_index,
                "is_active": item.is_active,
                "menu_id": item.menu_id,
                "created_at": item.created_at.isoformat() if item.created_at else None,
            } for item in items] for menu_id, items in menu_items.items()
        },
        "pages": [{
            "id": p.id,
            "title": p.title,
            "slug": p.slug,
            "content": p.content,
            "excerpt": p.excerpt,
            "template": p.template,
            "status": p.status,
            "parent_id": p.parent_id,
            "order_index": p.order_index,
            "meta_title": p.meta_title,
            "meta_description": p.meta_description,
            "meta_keywords": p.meta_keywords,
            "author": None,
            "author_id": p.author_id,
            "created_at": p.created_at.isoformat() if p.created_at else None,
            "updated_at": p.updated_at.isoformat() if p.updated_at else None,
        } for p in pages],
    })


# ============================================================
# 设置编辑
# ============================================================

@router.put("/settings", summary="编辑设置")
async def update_settings(
    settings: dict = Body(..., description="键值对设置"),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:edit")),
):
    """更新系统设置（全量替换）"""
    now = datetime.now(timezone.utc)

    for key, value in settings.items():
        result = await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key == key)
        )
        existing = result.scalar_one_or_none()
        if existing:
            existing.setting_value = str(value)
            existing.updated_at = now
        else:
            db.add(SystemSettings(
                setting_key=key,
                setting_value=str(value),
                created_at=now,
                updated_at=now,
            ))

    await db.commit()
    return ApiResponse(success=True, message="设置已更新")


# ============================================================
