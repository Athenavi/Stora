"""
MCP 安全管理工具处理器 — 审计日志/敏感词/IP封禁/2FA
"""
from sqlalchemy import select, func, desc
from src.utils.database.main import get_async_session_context
from src.mcp.tools._perms import require_superuser, require_role


@require_superuser
async def query_audit_log(arguments: dict) -> dict:
    """查询审计日志"""
    page = arguments.get("page", 1)
    limit = min(arguments.get("limit", 20), 100)
    user_id = arguments.get("user_id")
    action = arguments.get("action")
    level = arguments.get("level")

    async with get_async_session_context() as db:
        from shared.models.system import AuditLog
        query = select(AuditLog)
        if user_id:
            query = query.where(AuditLog.user_id == int(user_id))
        if action:
            query = query.where(AuditLog.action.ilike(f"%{action}%"))
        if level:
            query = query.where(AuditLog.level == level.upper())

        total = await db.scalar(select(func.count()).select_from(query.subquery())) or 0
        offset = (page - 1) * limit
        logs = (await db.execute(query.order_by(desc(AuditLog.created_at)).offset(offset).limit(limit))).scalars().all()

        return {"success": True, "data": {
            "logs": [{
                "id": log.id, "user_id": log.user_id, "action": log.action,
                "details": log.details, "ip_address": log.ip_address,
                "level": log.level, "created_at": str(log.created_at),
            } for log in logs],
            "total": total, "page": page, "limit": limit,
        }}


@require_superuser
async def export_audit_log(arguments: dict) -> dict:
    """导出审计日志为 CSV"""
    import csv, io
    days = arguments.get("days", 7)

    async with get_async_session_context() as db:
        from shared.models.system import AuditLog
        from datetime import datetime, timedelta
        cutoff = datetime.utcnow() - timedelta(days=days)
        logs = (await db.execute(
            select(AuditLog).where(AuditLog.created_at >= cutoff).order_by(desc(AuditLog.created_at))
        )).scalars().all()

        output = io.StringIO()
        writer = csv.writer(output)
        writer.writerow(["ID", "用户ID", "操作", "详情", "IP", "级别", "时间"])
        for log in logs:
            writer.writerow([log.id, log.user_id, log.action, log.details,
                           log.ip_address, log.level, str(log.created_at)])

        return {"success": True, "data": {
            "count": len(logs),
            "csv": output.getvalue()[:5000],
            "truncated": len(logs) > 100,
        }}


@require_superuser
async def scan_sensitive_words(arguments: dict) -> dict:
    """扫描内容中的敏感词"""
    text = arguments.get("text", "")
    if not text:
        return {"success": False, "error": "请提供要扫描的文本"}

    async with get_async_session_context() as db:
        from shared.models.security import SensitiveWord
        words = (await db.execute(select(SensitiveWord).where(SensitiveWord.is_active == True))).scalars().all()

    found = []
    for word in words:
        if word.word in text:
            found.append({"word": word.word, "level": word.level})

    return {"success": True, "data": {
        "has_sensitive": len(found) > 0, "found": found, "count": len(found),
    }}


@require_superuser
async def manage_sensitive_word(arguments: dict) -> dict:
    """添加/删除敏感词"""
    action = arguments.get("action", "add")
    word = arguments.get("word", "").strip()
    if not word:
        return {"success": False, "error": "请提供敏感词"}

    async with get_async_session_context() as db:
        from shared.models.security import SensitiveWord
        if action == "add":
            existing = await db.scalar(select(SensitiveWord).where(SensitiveWord.word == word))
            if not existing:
                db.add(SensitiveWord(
                    word=word, level=arguments.get("level", "medium"),
                    is_active=True, created_by=0,
                ))
                await db.commit()
                return {"success": True, "message": f"敏感词「{word}」已添加"}
            return {"success": False, "message": "敏感词已存在"}
        elif action == "remove":
            existing = await db.scalar(select(SensitiveWord).where(SensitiveWord.word == word))
            if existing:
                await db.delete(existing)
                await db.commit()
                return {"success": True, "message": f"敏感词「{word}」已删除"}
            return {"success": False, "message": "敏感词不存在"}

    return {"success": False, "error": f"未知操作: {action}"}


@require_role("admin")
async def list_rate_limits(arguments: dict) -> dict:
    """查看当前速率限制状态"""
    try:
        from src.api.v2.security.rate_limit import get_rate_limit_status
        status = get_rate_limit_status()
        return {"success": True, "data": status}
    except Exception as e:
        return {"success": False, "error": f"获取失败: {e}"}


@require_superuser
async def get_security_report(arguments: dict) -> dict:
    """生成安全报告摘要"""
    async with get_async_session_context() as db:
        from shared.models.security import SensitiveWord
        from shared.models.system import AuditLog
        from sqlalchemy import func

        total_critical = await db.scalar(
            select(func.count(AuditLog.id)).where(AuditLog.level == "CRITICAL")
        ) or 0
        total_error = await db.scalar(
            select(func.count(AuditLog.id)).where(AuditLog.level == "ERROR")
        ) or 0
        sensitive_count = await db.scalar(select(func.count(SensitiveWord.id))) or 0

        return {"success": True, "data": {
            "audit_logs": {"critical": total_critical, "error": total_error},
            "sensitive_words": sensitive_count,
        }}
