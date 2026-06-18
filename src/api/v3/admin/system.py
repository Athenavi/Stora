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

    return ApiResponse(success=True, data={
        "settings": {s.setting_key: s.setting_value for s in settings},
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
