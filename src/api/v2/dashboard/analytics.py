"""
数据分析 API
"""

from functools import wraps

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession

from shared.services.articles.analytics import create_analytics_service
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["analytics"])


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            import traceback
            traceback.print_exc()
            raise HTTPException(status_code=500, detail=str(e))
    return wrapper


@router.get("/overview")
@_catch
async def get_overview_stats(
        days: int = Query(30, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取概览统计数据
    
    Args:
        days: 统计天数
        db: 数据库会话
        
    Returns:
        概览数据
    """
    service = create_analytics_service(db)
    stats = await service.get_overview_stats(days)

    return {
        'success': True,
        'data': stats,
    }


@router.get("/article-views-trend")
@_catch
async def get_article_views_trend(
        days: int = Query(30, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取文章浏览量趋势
    
    Args:
        days: 统计天数
        db: 数据库会话
        
    Returns:
        每日浏览量列表
    """
    service = create_analytics_service(db)
    trend = await service.get_article_views_trend(days)

    return {
        'success': True,
        'data': trend,
    }


@router.get("/popular-articles")
@_catch
async def get_popular_articles(
        limit: int = Query(10, ge=1, le=100, description="返回数量"),
        days: int = Query(7, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取热门文章
    
    Args:
        limit: 返回数量
        days: 统计天数
        db: 数据库会话
        
    Returns:
        热门文章列表
    """
    service = create_analytics_service(db)
    articles = await service.get_popular_articles(limit, days)

    return {
        'success': True,
        'data': articles,
    }


@router.get("/category-distribution")
@_catch
async def get_category_distribution(
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取分类分布
    
    Args:
        db: 数据库会话
        
    Returns:
        分类统计列表
    """
    service = create_analytics_service(db)
    distribution = await service.get_category_distribution()

    return {
        'success': True,
        'data': distribution,
    }


@router.get("/user-activity")
@_catch
async def get_user_activity(
        days: int = Query(30, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取用户活动统计
    
    Args:
        days: 统计天数
        db: 数据库会话
        
    Returns:
        用户活动数据
    """
    service = create_analytics_service(db)
    activity = await service.get_user_activity(days)

    return {
        'success': True,
        'data': activity,
    }


@router.get("/content-performance")
@_catch
async def get_content_performance(
        days: int = Query(30, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取内容表现分析
    
    Args:
        days: 统计天数
        db: 数据库会话
        
    Returns:
        内容表现数据
    """
    service = create_analytics_service(db)
    performance = await service.get_content_performance(days)

    return {
        'success': True,
        'data': performance,
    }


@router.get("/traffic-sources")
@_catch
async def get_traffic_sources(
        days: int = Query(30, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取流量来源分析
    
    Args:
        days: 统计天数
        db: 数据库会话
        
    Returns:
        流量来源列表
    """
    service = create_analytics_service(db)
    sources = await service.get_traffic_sources(days)

    return {
        'success': True,
        'data': sources,
    }


@router.get("/device-stats")
@_catch
async def get_device_stats(
        days: int = Query(30, ge=1, le=365, description="统计天数"),
        db: AsyncSession = Depends(get_async_db)
):
    """
    获取设备统计
    
    Args:
        days: 统计天数
        db: 数据库会话
        
    Returns:
        设备分布数据
    """
    service = create_analytics_service(db)
    stats = await service.get_device_stats(days)

    return {
        'success': True,
        'data': stats,
    }
