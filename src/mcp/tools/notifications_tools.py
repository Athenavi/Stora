"""
MCP 通知/邮件工具处理器 — 通知管理/邮件发送/订阅
"""
from sqlalchemy import select, func, desc
from src.utils.database.main import get_async_session_context
from src.mcp.tools._perms import require_superuser, require_role


@require_role("user")
async def list_notifications(arguments: dict) -> dict:
    """获取当前用户的通知列表"""
    user_id = arguments.get("user_id")
    unread_only = arguments.get("unread_only", False)
    limit = min(arguments.get("limit", 20), 50)

    if not user_id:
        return {"success": False, "error": "请提供用户ID"}

    async with get_async_session_context() as db:
        from shared.models.notification import Notification
        query = select(Notification).where(Notification.recipient == int(user_id))
        if unread_only:
            query = query.where(Notification.is_read == False)

        total = await db.scalar(select(func.count()).select_from(query.subquery())) or 0
        notifications = (await db.execute(
            query.order_by(desc(Notification.created_at)).limit(limit)
        )).scalars().all()

        return {"success": True, "data": {
            "notifications": [{
                "id": n.id, "type": n.type, "title": n.title,
                "message": n.message, "is_read": n.is_read,
                "created_at": str(n.created_at),
            } for n in notifications],
            "total": total, "unread": sum(1 for n in notifications if not n.is_read),
        }}


@require_role("user")
async def mark_notification_read(arguments: dict) -> dict:
    """标记通知为已读"""
    notification_id = arguments.get("notification_id")
    if not notification_id:
        return {"success": False, "error": "请提供通知ID"}

    async with get_async_session_context() as db:
        from shared.models.notification import Notification
        notification = await db.scalar(select(Notification).where(Notification.id == int(notification_id)))
        if not notification:
            return {"success": False, "error": "通知不存在"}
        notification.is_read = True
        await db.commit()
        return {"success": True, "message": "已标记为已读"}


@require_superuser
async def send_test_email(arguments: dict) -> dict:
    """发送测试邮件"""
    to_email = arguments.get("to_email", "")
    if not to_email:
        return {"success": False, "error": "请提供收件人邮箱"}

    try:
        from shared.services.notifications.email_service import email_service
        await email_service.send_email(
            recipients=[to_email],
            subject="FastBlog 测试邮件",
            body="这是一封来自 FastBlog MCP 的测试邮件。",
        )
        return {"success": True, "message": f"测试邮件已发送至 {to_email}"}
    except Exception as e:
        return {"success": False, "error": f"发送失败: {e}"}


@require_superuser
async def send_bulk_notification(arguments: dict) -> dict:
    """向所有用户发送批量通知"""
    title = arguments.get("title", "").strip()
    message = arguments.get("message", "").strip()
    if not title or not message:
        return {"success": False, "error": "请提供通知标题和内容"}

    async with get_async_session_context() as db:
        from shared.models.user import User
        from shared.models.notification import Notification
        from datetime import datetime

        users = (await db.execute(select(User.id).limit(500))).scalars().all()
        sent = 0
        for uid in users:
            db.add(Notification(
                recipient=uid, type="system", title=title,
                message=message, is_read=False,
                created_at=datetime.utcnow(),
            ))
            sent += 1
            if sent % 50 == 0:
                await db.flush()
        await db.commit()
        return {"success": True, "message": f"已向 {sent} 名用户发送通知", "sent": sent}
