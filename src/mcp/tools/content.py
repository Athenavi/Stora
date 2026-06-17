"""
MCP 内容管理工具处理器 — 分类/标签/评论
"""
import re
from datetime import datetime

from sqlalchemy import select

# [REMOVED] article model deleted in Stora refactor
# [REMOVED] category model deleted in Stora refactor
# [REMOVED] comment model deleted in Stora refactor
from src.utils.database.main import get_async_session_context


# ─── 分类 ───

async def list_categories(arguments: dict) -> list:
    """获取分类列表"""
    async with get_async_session_context() as db:
        cats = (await db.execute(
            select(Category).order_by(Category.sort_order.asc(), Category.id.asc())
        )).scalars().all()
        return [{"id": c.id, "name": c.name, "slug": c.slug or "",
                 "description": c.description or "", "articles_count": getattr(c, 'articles_count', 0)}
                for c in cats]


async def create_category(arguments: dict) -> dict:
    """创建分类"""
    name = (arguments.get("name") or "").strip()
    if not name:
        raise ValueError("分类名称不能为空")
    slug = arguments.get("slug") or name.lower().replace(" ", "-")
    async with get_async_session_context() as db:
        try:
            cat = Category(name=name, slug=slug, description=arguments.get("description", ""),
                           created_at=datetime.utcnow(), updated_at=datetime.utcnow())
            db.add(cat)
            await db.commit()
            return {"success": True, "message": f"分类「{name}」创建成功", "category_id": cat.id}
        except Exception as e:
            await db.rollback()
            raise ValueError(f"创建分类失败: {e}")


async def update_category(arguments: dict) -> dict:
    """更新分类"""
    cid = arguments.get("category_id")
    if not cid:
        raise ValueError("分类ID不能为空")
    async with get_async_session_context() as db:
        cat = await db.scalar(select(Category).where(Category.id == int(cid)))
        if not cat:
            raise ValueError(f"分类 #{cid} 不存在")
        if "name" in arguments:
            cat.name = arguments["name"].strip()
        if "description" in arguments:
            cat.description = arguments["description"].strip()
        cat.updated_at = datetime.utcnow()
        await db.commit()
        return {"success": True, "message": f"分类 #{cid} 已更新", "category_id": cid}


async def delete_category(arguments: dict) -> dict:
    """删除分类"""
    cid = arguments.get("category_id")
    if not cid:
        raise ValueError("分类ID不能为空")
    async with get_async_session_context() as db:
        cat = await db.scalar(select(Category).where(Category.id == int(cid)))
        if not cat:
            raise ValueError(f"分类 #{cid} 不存在")
        # 先将引用此分类的文章置空
        from sqlalchemy import update
        await db.execute(
            update(Article).where(Article.category == int(cid)).values(category=None)
        )
        await db.delete(cat)
        await db.commit()
        return {"success": True, "message": f"分类 #{cid} 已删除"}


async def list_tags(arguments: dict) -> dict:
    """聚合获取所有标签"""
    async with get_async_session_context() as db:
        rows = (await db.execute(
            select(Article.tags_list).where(Article.tags_list.isnot(None))
        )).scalars().all()
        all_tags = sorted({t.strip() for row in rows if row for t in re.split(r'[,;]', row) if t.strip()})
        return {"success": True, "tags": [{"name": t, "articles_count": 0} for t in all_tags],
                "total": len(all_tags)}


# ─── 评论 ───

async def list_comments(arguments: dict) -> dict:
    """获取评论列表（支持状态筛选）"""
    status = arguments.get("status", "").strip().lower()
    limit = min(arguments.get("limit", 20), 50)
    async with get_async_session_context() as db:
        q = select(Comment).order_by(Comment.created_at.desc()).limit(limit)
        if status == "pending":
            q = q.where(Comment.is_approved == False)
        elif status == "approved":
            q = q.where(Comment.is_approved == True)

        comments = (await db.execute(q)).scalars().all()
        return {"success": True, "comments": [{
            "id": c.id, "article_id": c.article_id,
            "author": c.author_name or f"用户 #{c.user_id}" if c.user_id else "匿名",
            "content": (c.content or "")[:500], "is_approved": c.is_approved,
            "likes": c.likes or 0, "created_at": c.created_at.isoformat() if c.created_at else "",
        } for c in comments], "total": len(comments)}


async def approve_comment(arguments: dict) -> dict:
    """审核通过评论"""
    cid = arguments.get("comment_id")
    if not cid:
        raise ValueError("评论ID不能为空")
    async with get_async_session_context() as db:
        comment = await db.scalar(select(Comment).where(Comment.id == int(cid)))
        if not comment:
            raise ValueError(f"评论 #{cid} 不存在")
        comment.is_approved = True
        comment.updated_at = datetime.utcnow()
        await db.commit()
        return {"success": True, "message": f"评论 #{cid} 已审核通过", "comment_id": cid}


async def reject_comment(arguments: dict) -> dict:
    """拒绝评论"""
    cid = arguments.get("comment_id")
    if not cid:
        raise ValueError("评论ID不能为空")
    async with get_async_session_context() as db:
        comment = await db.scalar(select(Comment).where(Comment.id == int(cid)))
        if not comment:
            raise ValueError(f"评论 #{cid} 不存在")
        comment.is_approved = False
        comment.updated_at = datetime.utcnow()
        await db.commit()
        return {"success": True, "message": f"评论 #{cid} 已拒绝", "comment_id": cid}


async def delete_comment(arguments: dict) -> dict:
    """删除评论"""
    cid = arguments.get("comment_id")
    if not cid:
        raise ValueError("评论ID不能为空")
    async with get_async_session_context() as db:
        comment = await db.scalar(select(Comment).where(Comment.id == int(cid)))
        if not comment:
            raise ValueError(f"评论 #{cid} 不存在")
        await db.delete(comment)
        await db.commit()
        return {"success": True, "message": f"评论 #{cid} 已删除", "comment_id": cid}
