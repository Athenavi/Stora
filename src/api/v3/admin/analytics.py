"""
V3 分析数据 API

权限要求:
  GET    /analytics/overview   → settings:view
  GET    /analytics/popular    → article:view
"""
import logging
from datetime import datetime, timedelta

from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

# [REMOVED] article model deleted in Stora refactor
# [REMOVED] analytics model deleted in Stora refactor
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db
from src.api.v3._permission import Permission

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-analytics"])


@router.get("/analytics/overview", summary="分析概览")
async def analytics_overview(
    days: int = Query(30, ge=1, le=365),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    since = datetime.utcnow() - timedelta(days=days)

    article_count = await db.scalar(
        select(func.count(Article.id)).where(Article.created_at >= since)
    ) or 0

    page_views = await db.scalar(
        select(func.count(PageView.id)).where(PageView.created_at >= since)
    ) or 0

    return ApiResponse(success=True, data={
        "period_days": days,
        "articles": article_count,
        "page_views": page_views,
    })


@router.get("/analytics/popular", summary="热门文章")
async def popular_articles(
    limit: int = Query(10, ge=1, le=50),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("article:view")),
):
    result = await db.execute(
        select(Article).order_by(Article.views.desc().nullslast()).limit(limit)
    )
    articles = result.scalars().all()

    return ApiResponse(success=True, data={
        "articles": [{
            "id": a.id,
            "title": a.title,
            "views": a.views or 0,
            "likes": a.likes or 0,
        } for a in articles],
    })
