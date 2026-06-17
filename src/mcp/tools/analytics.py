"""
MCP 分析/统计/SEO 工具处理器
"""
from datetime import datetime, timedelta

from sqlalchemy import select, func

# [REMOVED] article model deleted in Stora refactor
# [REMOVED] category model deleted in Stora refactor
# [REMOVED] comment model deleted in Stora refactor
from shared.models.user import User
from src.utils.database.main import get_async_session_context


async def get_analytics(arguments: dict) -> dict:
    """获取博客分析概况"""
    async with get_async_session_context() as db:
        published = await db.scalar(select(func.count(Article.id)).where(Article.status == 1)) or 0
        draft = await db.scalar(select(func.count(Article.id)).where(Article.status == 0)) or 0
        users = await db.scalar(select(func.count(User.id))) or 0
        total_comments = await db.scalar(select(func.count(Comment.id))) or 0
        pending = await db.scalar(select(func.count(Comment.id)).where(Comment.is_approved == False)) or 0
        categories = await db.scalar(select(func.count(Category.id))) or 0
        total_views = await db.scalar(select(func.coalesce(func.sum(Article.views), 0)).where(Article.status == 1)) or 0

        return {"success": True, "data": {
            "articles": {"published": published, "draft": draft, "total": published + draft},
            "users": users, "comments": {"total": total_comments, "pending_approval": pending},
            "categories": categories, "total_views": total_views,
        }}


async def get_trending_articles(arguments: dict) -> dict:
    """获取热门文章排行"""
    limit = min(arguments.get("limit", 10), 30)
    days = arguments.get("days", 7)
    cutoff = datetime.utcnow() - timedelta(days=days)

    async with get_async_session_context() as db:
        articles = (await db.execute(
            select(Article).where(Article.status == 1, Article.created_at >= cutoff)
            .order_by(Article.views.desc()).limit(limit)
        )).scalars().all()

        return {"success": True, "articles": [{
            "id": a.id, "title": a.title, "slug": a.slug or "",
            "views": a.views or 0, "likes": a.likes or 0,
            "created_at": a.created_at.isoformat() if a.created_at else "",
        } for a in articles]}


async def get_system_stats(arguments: dict) -> dict:
    """获取系统统计信息"""
    async with get_async_session_context() as db:
        published = await db.scalar(select(func.count(Article.id)).where(Article.status == 1)) or 0
        draft = await db.scalar(select(func.count(Article.id)).where(Article.status == 0)) or 0
        users = await db.scalar(select(func.count(User.id))) or 0
        categories = await db.scalar(select(func.count(Category.id))) or 0
        views = await db.scalar(select(func.coalesce(func.sum(Article.views), 0)).where(Article.status == 1)) or 0

        return {"published_articles": published, "draft_articles": draft,
                "total_articles": published + draft, "total_users": users,
                "total_categories": categories, "total_views": views}


async def generate_seo_description(arguments: dict) -> dict:
    """生成 SEO 描述"""
    article_id = arguments.get("article_id")
    if not article_id:
        raise ValueError("文章ID不能为空")

    async with get_async_session_context() as db:
        article = await db.scalar(select(Article).where(Article.id == int(article_id)))
        if not article:
            raise ValueError(f"文章 #{article_id} 不存在")

        title = article.title or ""
        excerpt = article.excerpt or ""
        meta_desc = excerpt[:160] or title[:160]

        keywords = []
        if title:
            keywords.extend(w.strip() for w in title.replace(" ", ",").split(",") if w.strip())
        if article.tags_list:
            keywords.extend(t.strip() for t in article.tags_list.split(",") if t.strip())

        return {"success": True, "article_id": article_id,
                "seo_description": meta_desc, "keywords": list(set(keywords))[:8], "title": title}
