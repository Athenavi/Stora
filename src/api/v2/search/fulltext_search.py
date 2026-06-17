"""
全文搜索 API 端点
提供基于 Meilisearch 的高性能搜索功能
"""
import re
from datetime import datetime
from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, Query, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession

from shared.services.integrations.meilisearch_service import meilisearch_service
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["fulltext-search"])


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
            return fail(str(e))
    return wrapper


@router.get("/articles")
@_catch
async def search_articles(
        q: str = Query(..., min_length=1, description="搜索关键词"),
        category_id: Optional[int] = Query(None, description="分类ID"),
        author_id: Optional[int] = Query(None, description="作者ID"),
        date_from: Optional[str] = Query(None, description="起始日期 (YYYY-MM-DD)"),
        date_to: Optional[str] = Query(None, description="结束日期 (YYYY-MM-DD)"),
        status: str = Query("published", description="文章状态"),
        page: int = Query(1, ge=1, description="页码"),
        per_page: int = Query(20, ge=1, le=100, description="每页数量"),
        sort_by: str = Query("relevance", enum=["relevance", "date", "views"], description="排序方式"),
):
    """
    使用 Meilisearch 搜索文章

    Args:
        q: 搜索关键词
        category_id: 分类ID过滤
        author_id: 作者ID过滤
        date_from: 起始日期
        date_to: 结束日期
        status: 文章状态
        page: 页码
        per_page: 每页数量
        sort_by: 排序方式

    Returns:
        搜索结果
    """
    date_from_dt = None
    date_to_dt = None

    if date_from:
        try:
            date_from_dt = datetime.strptime(date_from, "%Y-%m-%d")
        except ValueError:
            raise HTTPException(status_code=400, detail="Invalid date_from format. Use YYYY-MM-DD")

    if date_to:
        try:
            date_to_dt = datetime.strptime(date_to, "%Y-%m-%d")
            date_to_dt = date_to_dt.replace(hour=23, minute=59, second=59)
        except ValueError:
            raise HTTPException(status_code=400, detail="Invalid date_to format. Use YYYY-MM-DD")

    result = await meilisearch_service.search(
        query=q,
        category_id=category_id,
        author_id=author_id,
        date_from=date_from_dt,
        date_to=date_to_dt,
        status=status,
        page=page,
        per_page=per_page,
        sort_by=sort_by
    )

    return ok(data=result)


@router.get("/suggestions")
@_catch
async def get_search_suggestions(
        q: str = Query(..., min_length=1, description="搜索前缀"),
        limit: int = Query(5, ge=1, le=10, description="返回数量")
):
    """
    获取搜索建议（自动完成）

    Args:
        q: 搜索前缀
        limit: 返回数量

    Returns:
        搜索建议列表
    """
    suggestions = await meilisearch_service.get_search_suggestions(
        query=q,
        limit=limit
    )

    return ok(data={
        'suggestions': suggestions,
        'query': q
    })


@router.post("/rebuild-index")
@_catch
async def rebuild_search_index(
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    重建搜索索引（仅管理员）

    重新索引所有已发布的文章

    Returns:
        重建结果
    """
    if not current_user.is_superuser:
        return fail("Permission denied. Admin access required.")

    # [REMOVED] article model deleted in Stora refactor
    # [REMOVED] article model deleted in Stora refactor
    # [REMOVED] category model deleted in Stora refactor
    from shared.models.user import User
    from sqlalchemy import select

    stmt = (
        select(Article, ArticleContent, Category.name.label('category_name'), User.username.label('author_name'))
        .outerjoin(ArticleContent, Article.id == ArticleContent.article)
        .outerjoin(Category, Article.category == Category.id)
        .outerjoin(User, Article.user == User.id)
        .where(Article.status == 1)
    )

    result = await db.execute(stmt)
    rows = result.all()

    articles = []
    for row in rows:
        article, content, category_name, author_name = row

        article_data = {
            'id': article.id,
            'title': article.title,
            'slug': article.slug,
            'excerpt': article.excerpt or '',
            'content': content.content if content else '',
            'cover_image': article.cover_image or '',
            'category_id': article.category,
            'category_name': category_name or '',
            'author_id': article.user,
            'author_name': author_name or '',
            'tags': [t.strip() for t in re.split(r'[,;]', article.tags_list) if
                     t.strip()] if article.tags_list else [],
            'views': article.views,
            'likes': article.likes,
            'status': 'published',
            'is_featured': article.is_featured,
            'created_at': int(article.created_at.timestamp()) if article.created_at else 0,
            'updated_at': int(article.updated_at.timestamp()) if article.updated_at else 0,
        }

        articles.append(article_data)

    success = await meilisearch_service.rebuild_index(articles)

    if success:
        return ok(data={
            'message': f'Search index rebuilt successfully with {len(articles)} articles',
            'indexed_count': len(articles)
        })
    else:
        return fail('Failed to rebuild search index')


@router.get("/stats")
@_catch
async def get_search_stats(
        current_user=Depends(jwt_required)
):
    """
    获取搜索索引统计信息（仅管理员）

    Returns:
        统计信息
    """
    if not current_user.is_superuser:
        return fail("Permission denied. Admin access required.")

    stats = await meilisearch_service.get_index_stats()

    return ok(data=stats)


@router.post("/sync-article/{article_id}")
@_catch
async def sync_article_to_index(
        article_id: int,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """
    手动同步单篇文章到搜索索引

    Args:
        article_id: 文章ID

    Returns:
        同步结果
    """
    # [REMOVED] article model deleted in Stora refactor
    # [REMOVED] article model deleted in Stora refactor
    # [REMOVED] category model deleted in Stora refactor
    from shared.models.user import User
    from sqlalchemy import select

    stmt = (
        select(Article, ArticleContent, Category.name.label('category_name'), User.username.label('author_name'))
        .outerjoin(ArticleContent, Article.id == ArticleContent.article)
        .outerjoin(Category, Article.category == Category.id)
        .outerjoin(User, Article.user == User.id)
        .where(Article.id == article_id)
    )

    result = await db.execute(stmt)
    row = result.first()

    if not row:
        return fail(f'Article {article_id} not found')

    article, content, category_name, author_name = row

    article_data = {
        'id': article.id,
        'title': article.title,
        'slug': article.slug,
        'excerpt': article.excerpt or '',
        'content': content.content if content else '',
        'cover_image': article.cover_image or '',
        'category_id': article.category,
        'category_name': category_name or '',
        'author_id': article.user,
        'author_name': author_name or '',
        'tags': [t.strip() for t in re.split(r'[,;]', article.tags_list) if t.strip()] if article.tags_list else [],
        'views': article.views,
        'likes': article.likes,
        'status': 'published' if article.status == 1 else 'draft',
        'is_featured': article.is_featured,
        'created_at': int(article.created_at.timestamp()) if article.created_at else 0,
        'updated_at': int(article.updated_at.timestamp()) if article.updated_at else 0,
    }

    if article.status == 1:
        success = await meilisearch_service.index_article(article_data)
    else:
        success = await meilisearch_service.delete_article(article_id)

    if success:
        return ok(data={'message': f'Article {article_id} synced successfully'})
    else:
        return fail('Failed to sync article')
