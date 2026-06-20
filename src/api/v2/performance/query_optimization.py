"""
查询优化 API
提供查询分析、性能监控和优化建议功能
"""
from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Body
from sqlalchemy.ext.asyncio import AsyncSession

from shared.services.performance.query_monitor import query_monitor_service
from shared.services.performance.query_optimizer import query_optimizer
from src.api.v2._helpers import ok, fail, _catch
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter()


def _check_admin(user):
    is_admin = getattr(user, 'is_superuser', False) or getattr(user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="Permission denied")


@router.get("/analysis", summary="查询分析", description="分析最近的数据库查询，检测性能问题")
@_catch
async def analyze_queries(
        limit: int = Query(100, ge=1, le=1000, description="分析的查询数量"),
        current_user=Depends(jwt_required),
):
    """分析数据库查询"""
    _check_admin(current_user)
    recent_queries = query_monitor_service.queries[-limit:]
    if not recent_queries:
        return ok(data={'message': 'No queries to analyze', 'total_queries': 0})
    analysis = query_optimizer.analyze_queries(recent_queries)
    return ok(data=analysis)


@router.get("/monitor/stats", summary="查询监控统计", description="获取查询监控的统计信息")
@_catch
async def get_query_stats(current_user=Depends(jwt_required)):
    """获取查询监控统计"""
    _check_admin(current_user)
    stats = query_monitor_service.get_summary()
    return ok(data=stats)


@router.post("/optimize/article-query", summary="优化文章查询", description="使用优化的方式查询文章列表")
@_catch
async def optimize_article_query(
        page: int = Body(1, ge=1, description="页码"),
        per_page: int = Body(10, ge=1, le=100, description="每页数量"),
        category_id: Optional[int] = Body(None, description="分类ID"),
        status: Optional[int] = Body(None, description="状态"),
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db),
):
    """优化的文章查询示例"""
    # [REMOVED] article model deleted in Stora refactor
    filters = {}
    if category_id is not None:
        filters['category'] = category_id
    if status is not None:
        filters['status'] = status
    articles, total = await query_optimizer.optimize_article_query(
        db=db, page=page, per_page=per_page, **filters
    )
    articles_data = [{
        'id': a.id, 'title': a.title, 'slug': a.slug, 'excerpt': a.excerpt,
        'author': {'id': a.author.id, 'username': a.author.username} if hasattr(a, 'author') and a.author else None,
        'category': {'id': a.category.id, 'name': a.category.name} if hasattr(a, 'category') and a.category else None,
        'created_at': a.created_at.isoformat() if a.created_at else None,
    } for a in articles]
    return ok(data={
        'articles': articles_data,
        'pagination': {
            'current_page': page, 'per_page': per_page,
            'total': total, 'total_pages': (total + per_page - 1) // per_page if per_page > 0 else 0,
        }
    })


@router.get("/recommendations", summary="优化建议", description="获取数据库优化建议")
@_catch
async def get_optimization_recommendations(current_user=Depends(jwt_required)):
    """获取数据库优化建议"""
    _check_admin(current_user)
    recommendations = [
        {'type': 'index', 'priority': 'high', 'title': '添加索引',
         'description': '为频繁查询的字段添加索引',
         'suggestions': ['为 articles.status 字段添加索引', '为 articles.category_id 字段添加索引',
                         '为 articles.created_at 字段添加索引', '为 user_activities.user_id 字段添加索引']},
        {'type': 'eager_loading', 'priority': 'high', 'title': '使用预加载',
         'description': '使用joinedload或selectinload避免N+1查询',
         'example': 'query = select(Article).options(joinedload(Article.author))'},
        {'type': 'caching', 'priority': 'medium', 'title': '使用缓存',
         'description': '对频繁访问的数据使用多级缓存',
         'suggestions': ['缓存热门文章列表', '缓存分类信息', '缓存用户权限数据', '使用Redis作为L2缓存']},
        {'type': 'batch_operations', 'priority': 'medium', 'title': '批量操作',
         'description': '使用批量查询和更新减少数据库往返'},
        {'type': 'pagination', 'priority': 'low', 'title': '优化分页',
         'description': '对于大数据集使用游标分页替代偏移量分页',
         'suggestion': '当数据量超过10万条时，考虑使用基于ID或时间戳的游标分页'},
    ]
    return ok(data=recommendations)


@router.post("/reset-monitor", summary="重置监控", description="重置查询监控数据")
@_catch
async def reset_query_monitor(current_user=Depends(jwt_required)):
    """重置查询监控"""
    _check_admin(current_user)
    query_monitor_service.queries.clear()
    return ok(data=None, message="Query monitor reset successfully")
