"""
V3 搜索分析 API

权限要求:
  GET /search-analytics/popular    → settings:view
  GET /search-analytics/zero-result → settings:view
  GET /search-analytics/trend      → settings:view
"""
import logging
from datetime import datetime, timedelta

from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.search import SearchHistory
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db
from src.api.v3._permission import Permission

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-search-analytics"])


@router.get("/search-analytics/popular", summary="热门搜索词")
async def popular_searches(
    days: int = Query(30, ge=1, le=365),
    limit: int = Query(20, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    """热门搜索关键词排行"""
    since = datetime.utcnow() - timedelta(days=days)
    result = await db.execute(
        select(SearchHistory.keyword, func.count(SearchHistory.id).label("count"))
        .where(SearchHistory.created_at >= since, SearchHistory.keyword.isnot(None))
        .group_by(SearchHistory.keyword)
        .order_by(func.count(SearchHistory.id).desc())
        .limit(limit)
    )
    rows = result.all()
    return ApiResponse(success=True, data={
        "keywords": [{"keyword": r.keyword, "count": r.count} for r in rows],
    })


@router.get("/search-analytics/zero-result", summary="零结果搜索")
async def zero_result_searches(
    days: int = Query(30, ge=1, le=365),
    limit: int = Query(20, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    """返回零结果的搜索词（可能需要补充内容）"""
    since = datetime.utcnow() - timedelta(days=days)
    result = await db.execute(
        select(SearchHistory.keyword, func.count(SearchHistory.id).label("count"))
        .where(
            SearchHistory.created_at >= since,
            SearchHistory.keyword.isnot(None),
            SearchHistory.results_count == 0,
        )
        .group_by(SearchHistory.keyword)
        .order_by(func.count(SearchHistory.id).desc())
        .limit(limit)
    )
    rows = result.all()
    return ApiResponse(success=True, data={
        "keywords": [{"keyword": r.keyword, "count": r.count} for r in rows],
    })


@router.get("/search-analytics/trend", summary="搜索趋势")
async def search_trend(
    days: int = Query(7, ge=1, le=90),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("settings:view")),
):
    """每日搜索量趋势"""
    since = datetime.utcnow() - timedelta(days=days)
    result = await db.execute(
        select(
            func.date(SearchHistory.created_at).label("date"),
            func.count(SearchHistory.id).label("count"),
        )
        .where(SearchHistory.created_at >= since)
        .group_by(func.date(SearchHistory.created_at))
        .order_by(func.date(SearchHistory.created_at))
    )
    rows = result.all()
    return ApiResponse(success=True, data={
        "trend": [{"date": str(r.date), "count": r.count} for r in rows],
    })
