"""
MCP 用户管理工具处理器 — CRUD / 角色 / 状态
"""
from sqlalchemy import select, func

from shared.models.file import FileItem
from src.utils.database.main import get_async_session_context
from src.mcp.tools._perms import require_superuser, require_self_or_admin


@require_superuser
async def list_users(arguments: dict) -> dict:
    """获取用户列表（管理员）"""
    page = arguments.get("page", 1)
    limit = min(arguments.get("limit", 20), 100)
    offset = (page - 1) * limit

    async with get_async_session_context() as db:
        from shared.models.user import User
        total = await db.scalar(select(func.count(User.id))) or 0
        users = (await db.execute(
            select(User).order_by(User.id.desc()).offset(offset).limit(limit)
        )).scalars().all()

        return {"success": True, "data": {
            "users": [{
                "id": u.id, "username": u.username, "email": u.email,
                "is_active": getattr(u, 'is_active', True),
                "is_superuser": getattr(u, 'is_superuser', False),
                "vip_level": getattr(u, 'vip_level', 0),
                "created_at": str(u.date_joined) if hasattr(u, 'date_joined') else None,
            } for u in users],
            "total": total, "page": page, "limit": limit,
        }}


@require_superuser
async def create_user(arguments: dict) -> dict:
    """创建新用户（管理员）"""
    username = arguments.get("username", "").strip()
    email = arguments.get("email", "").strip()
    password = arguments.get("password", "")
    if not username or not password:
        return {"success": False, "error": "用户名和密码不能为空"}

    async with get_async_session_context() as db:
        from shared.models.user import User
        from shared.services.users.user_manager.user_service import create_user_account

        existing = (await db.execute(
            select(User).where((User.username == username) | (User.email == email))
        )).scalars().all()
        if existing:
            return {"success": False, "error": "用户名或邮箱已存在"}

        user = await create_user_account(
            db=db, username=username, email=email,
            password=password,
            is_superuser=arguments.get("is_superuser", False),
        )
        return {"success": True, "message": f"用户 {username} 创建成功", "user_id": user.id}


@require_superuser
async def update_user_role(arguments: dict) -> dict:
    """更新用户角色"""
    user_id = arguments.get("user_id")
    role = arguments.get("role", "user")
    if not user_id:
        return {"success": False, "error": "用户ID不能为空"}

    async with get_async_session_context() as db:
        from shared.models.user import User
        user = await db.scalar(select(User).where(User.id == int(user_id)))
        if not user:
            return {"success": False, "error": "用户不存在"}

        setattr(user, 'is_superuser', role == "superuser")
        setattr(user, 'role', role)
        await db.commit()
        return {"success": True, "message": f"用户 {user.username} 角色已更新为 {role}"}


@require_superuser
async def ban_user(arguments: dict) -> dict:
    """封禁/解封用户"""
    user_id = arguments.get("user_id")
    ban = arguments.get("ban", True)
    reason = arguments.get("reason", "")
    if not user_id:
        return {"success": False, "error": "用户ID不能为空"}

    async with get_async_session_context() as db:
        from shared.models.user import User
        user = await db.scalar(select(User).where(User.id == int(user_id)))
        if not user:
            return {"success": False, "error": "用户不存在"}
        setattr(user, 'is_active', not ban)
        await db.commit()
        action = "已封禁" if ban else "已解封"
        return {"success": True, "message": f"用户 {user.username} {action}"}


@require_self_or_admin("user_id")
async def get_user_stats(arguments: dict) -> dict:
    """获取用户统计数据"""
    user_id = arguments.get("user_id")
    async with get_async_session_context() as db:
        # [REMOVED] article model deleted in Stora refactor
        # [REMOVED] comment model deleted in Stora refactor
        from sqlalchemy import func

        #articles = await db.scalar(select(func.count(Article.id)).where(Article.user == int(user_id))) or 0
        #comments = await db.scalar(select(func.count(Comment.id)).where(Comment.user_id == int(user_id))) or 0
        media_count = await db.scalar(select(func.count(FileItem.id)).where(FileItem.user == int(user_id))) or 0

        return {"success": True, "data": {
            "user_id": user_id,
            "media_files": media_count,
        }}


@require_self_or_admin("user_id")
async def list_user_activity(arguments: dict) -> dict:
    """获取用户最近活动"""
    user_id = arguments.get("user_id")
    limit = min(arguments.get("limit", 10), 50)

    async with get_async_session_context() as db:
        from shared.models.system import AuditLog
        from sqlalchemy import desc

        logs = (await db.execute(
            select(AuditLog).where(AuditLog.user_id == int(user_id))
            .order_by(desc(AuditLog.created_at)).limit(limit)
        )).scalars().all()

        return {"success": True, "data": [{
            "id": log.id, "action": log.action, "details": log.details,
            "ip_address": log.ip_address, "created_at": str(log.created_at),
        } for log in logs]}
