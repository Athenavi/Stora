"""
V3 仪表盘 API

权限要求:
  GET    /dashboard/stats          → settings:view
  GET    /dashboard/recent-articles → article:view
  GET    /dashboard/traffic        → settings:view
"""
import logging
from datetime import datetime, timezone
from typing import Optional

from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

# [REMOVED] article model deleted in Stora refactor
# [REMOVED] comment model deleted in Stora refactor
from shared.models.user import User
from shared.models.media import Media
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db
from src.api.v3._permission import Permission

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-dashboard"])


@router.get("/dashboard/stats", summary="仪表盘统计")
async def dashboard_stats(
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    """系统核心统计"""
    article_count = await db.scalar(select(func.count(Article.id))) or 0
    user_count = await db.scalar(select(func.count(User.id))) or 0
    comment_count = await db.scalar(select(func.count(Comment.id))) or 0
    media_count = await db.scalar(select(func.count(Media.id))) or 0

    today = datetime.utcnow().date()
    today_articles = await db.scalar(
        select(func.count(Article.id)).where(
            func.date(Article.created_at) == today
        )
    ) or 0

    return ApiResponse(success=True, data={
        "articles": article_count,
        "users": user_count,
        "comments": comment_count,
        "media": media_count,
        "today_articles": today_articles,
    })


@router.get("/dashboard/recent-articles", summary="最近文章")
async def recent_articles(
    limit: int = Query(10, ge=1, le=50),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("article:view")),
):
    result = await db.execute(
        select(Article).order_by(Article.created_at.desc()).limit(limit)
    )
    articles = result.scalars().all()

    return ApiResponse(success=True, data={
        "articles": [{
            "id": a.id,
            "title": a.title,
            "status": a.status,
            "views": a.views or 0,
            "created_at": a.created_at.isoformat() if a.created_at else None,
        } for a in articles],
    })


@router.get("/dashboard/traffic", summary="流量概览")
async def traffic_overview(
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    """流量数据概览（日/周/月维度）"""
    now = datetime.utcnow()
    # [REMOVED] analytics model deleted in Stora refactor

    day_views = await db.scalar(
        select(func.count(PageView.id)).where(
            func.date(PageView.created_at) == now.date()
        )
    ) or 0

    return ApiResponse(success=True, data={
        "today_views": day_views,
    })
