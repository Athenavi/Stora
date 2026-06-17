"""
MCP 文章工具处理�?
"""
from datetime import datetime

from sqlalchemy import select

# [REMOVED] article model deleted in Stora refactor
from src.utils.database.main import get_async_session_context


async def create_article(arguments: dict) -> dict:
    """创建新文�?""
    title = (arguments.get("title") or "").strip()
    content = (arguments.get("content") or "").strip()
    if not title:
        raise ValueError("文章标题不能为空")
    if not content:
        raise ValueError("文章内容不能为空")

    now = datetime.utcnow()
    slug = title.lower().replace(" ", "-")[:200]
    status_str = arguments.get("status", "draft")

    async with get_async_session_context() as db:
        try:
            # 从上下文获取当前用户（MCP 调用方），回退到默认�?
            from src.mcp._context import get_user_ctx
            ctx = get_user_ctx()
            author_id = ctx.id if ctx else None
            article = Article(
                title=title, slug=slug, excerpt=content[:200], user=author_id or 1,
                category=arguments.get("category_id"), tags_list=arguments.get("tags", ""),
                status=1 if status_str == "published" else 0, created_at=now, updated_at=now,
            )
            db.add(article)
            await db.flush()

            db.add(ArticleContent(article=article.id, content=content, created_at=now, updated_at=now))
            await db.commit()

            return {"success": True, "message": f"文章「{title}」创建成�?,
                    "article_id": article.id, "status": status_str}
        except Exception as e:
            await db.rollback()
            raise ValueError(f"创建文章失败: {e}")


async def update_article(arguments: dict) -> dict:
    """更新文章"""
    article_id = arguments.get("article_id")
    if not article_id:
        raise ValueError("文章ID不能为空")

    now = datetime.utcnow()
    async with get_async_session_context() as db:
        article = await db.scalar(select(Article).where(Article.id == int(article_id)))
        if not article:
            raise ValueError(f"文章 #{article_id} 不存�?)

        if "title" in arguments:
            article.title = arguments["title"].strip()
        if "status" in arguments:
            article.status = 1 if arguments["status"] == "published" else 0
        if "content" in arguments:
            text = arguments["content"].strip()
            ac = await db.scalar(select(ArticleContent).where(ArticleContent.article == int(article_id)))
            if ac:
                ac.content = text
                ac.updated_at = now
            else:
                db.add(ArticleContent(article=int(article_id), content=text, created_at=now, updated_at=now))

        article.updated_at = now
        await db.commit()
        return {"success": True, "message": f"文章 #{article_id} 更新成功", "article_id": article_id}


async def delete_article(arguments: dict) -> dict:
    """软删除文�?""
    article_id = arguments.get("article_id")
    if not article_id:
        raise ValueError("文章ID不能为空")

    async with get_async_session_context() as db:
        article = await db.scalar(select(Article).where(Article.id == int(article_id)))
        if not article:
            raise ValueError(f"文章 #{article_id} 不存�?)

        article.status = -1
        article.updated_at = datetime.utcnow()
        await db.commit()
        return {"success": True, "message": f"文章 #{article_id} 已删�?, "article_id": article_id}


async def search_articles(arguments: dict) -> list:
    """搜索文章（优�?MeiliSearch，回退数据�?LIKE�?""
    query_text = (arguments.get("query") or "").strip()
    limit = min(arguments.get("limit", 10), 50)
    if not query_text:
        raise ValueError("搜索关键词不能为�?)

    try:
        # [REMOVED].meilisearch_service import meilisearch_service
        result = await meilisearch_service.search(query=query_text, page=1, per_page=limit)
        if result and 'articles' in result:
            return [{"id": h.get("id"), "title": h.get("title", ""),
                     "excerpt": h.get("excerpt", ""), "slug": h.get("slug", ""),
                     "category_name": h.get("category_name", ""), "author_name": h.get("author_name", "")}
                    for h in result['articles']]
    except Exception:
        pass

    async with get_async_session_context() as db:
        pattern = f"%{query_text}%"
        articles = (await db.execute(
            select(Article).where(Article.status == 1)
            .where(Article.title.ilike(pattern) | Article.excerpt.ilike(pattern))
            .order_by(Article.views.desc()).limit(limit)
        )).scalars().all()
        return [{"id": a.id, "title": a.title, "excerpt": a.excerpt or "", "slug": a.slug or ""}
                for a in articles]
