"""
MCP 工作流/协作工具处理器 — 审批/协作/发布
"""
from sqlalchemy import select, func, desc
from src.utils.database.main import get_async_session_context
from src.mcp.tools._perms import require_superuser, require_role


@require_role("editor")
async def list_pending_reviews(arguments: dict) -> dict:
    """列出待审批的内容"""
    page = arguments.get("page", 1)
    limit = min(arguments.get("limit", 20), 50)

    async with get_async_session_context() as db:
        # [REMOVED] collaboration model deleted in Stora refactor
        query = select(ApprovalRecord).where(ApprovalRecord.status == "pending")
        total = await db.scalar(select(func.count()).select_from(query.subquery())) or 0
        offset = (page - 1) * limit
        records = (await db.execute(
            query.order_by(desc(ApprovalRecord.created_at)).offset(offset).limit(limit)
        )).scalars().all()

        return {"success": True, "data": {
            "reviews": [{
                "id": r.id, "content_type": r.content_type,
                "content_id": r.content_id, "requester": r.applicant_id,
                "status": r.status, "created_at": str(r.created_at),
            } for r in records],
            "total": total, "page": page,
        }}


@require_role("editor")
async def approve_content(arguments: dict) -> dict:
    """审批通过内容"""
    review_id = arguments.get("review_id")
    comment = arguments.get("comment", "")
    if not review_id:
        return {"success": False, "error": "请提供审批记录ID"}

    async with get_async_session_context() as db:
        # [REMOVED] collaboration model deleted in Stora refactor
        record = await db.scalar(select(ApprovalRecord).where(ApprovalRecord.id == int(review_id)))
        if not record:
            return {"success": False, "error": "审批记录不存在"}
        record.status = "approved"
        await db.commit()
        return {"success": True, "message": f"审批 #{review_id} 已通过"}


@require_role("editor")
async def reject_content(arguments: dict) -> dict:
    """驳回内容"""
    review_id = arguments.get("review_id")
    reason = arguments.get("reason", "")
    if not review_id:
        return {"success": False, "error": "请提供审批记录ID"}
    if not reason:
        return {"success": False, "error": "请填写驳回原因"}

    async with get_async_session_context() as db:
        # [REMOVED] collaboration model deleted in Stora refactor
        record = await db.scalar(select(ApprovalRecord).where(ApprovalRecord.id == int(review_id)))
        if not record:
            return {"success": False, "error": "审批记录不存在"}
        record.status = "rejected"
        await db.commit()
        return {"success": True, "message": f"审批 #{review_id} 已驳回"}


@require_role("editor")
async def get_workflow_stats(arguments: dict) -> dict:
    """获取工作流统计"""
    async with get_async_session_context() as db:
        # [REMOVED] collaboration model deleted in Stora refactor
        total = await db.scalar(select(func.count(ApprovalRecord.id))) or 0
        pending = await db.scalar(
            select(func.count(ApprovalRecord.id)).where(ApprovalRecord.status == "pending")
        ) or 0
        approved = await db.scalar(
            select(func.count(ApprovalRecord.id)).where(ApprovalRecord.status == "approved")
        ) or 0
        rejected = await db.scalar(
            select(func.count(ApprovalRecord.id)).where(ApprovalRecord.status == "rejected")
        ) or 0

        return {"success": True, "data": {
            "total": total, "pending": pending,
            "approved": approved, "rejected": rejected,
        }}


@require_superuser
async def list_collaborators(arguments: dict) -> dict:
    """列出协作成员"""
    workspace_id = arguments.get("workspace_id")
    async with get_async_session_context() as db:
        # [REMOVED] collaboration model deleted in Stora refactor
        query = select(WorkspaceMember)
        if workspace_id:
            query = query.where(WorkspaceMember.workspace_id == int(workspace_id))
        members = (await db.execute(query.limit(50))).scalars().all()
        return {"success": True, "data": [{
            "id": m.id, "user_id": m.user_id, "role": m.role,
            "joined_at": str(m.joined_at),
        } for m in members]}


@require_superuser
async def batch_publish_articles(arguments: dict) -> dict:
    """批量发布文章"""
    article_ids = arguments.get("article_ids", [])
    schedule_time = arguments.get("schedule_time")
    if not article_ids:
        return {"success": False, "error": "请提供文章ID列表"}

    async with get_async_session_context() as db:
        # [REMOVED] article model deleted in Stora refactor
        from datetime import datetime
        published = 0
        for aid in article_ids:
            article = await db.scalar(select(Article).where(Article.id == int(aid)))
            if article and article.status == 0:
                article.status = 1
                article.updated_at = datetime.utcnow()
                published += 1
        await db.commit()
        return {"success": True, "message": f"已发布 {published} 篇文章", "published": published}
