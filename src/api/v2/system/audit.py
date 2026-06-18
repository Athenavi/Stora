"""
Stora 审计日志 API — 查看和搜索操作日志
"""
from typing import Optional

from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_

from shared.models import AuditLog, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["audit-logs"])


@router.get("")
async def list_audit_logs(
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=200),
    action: Optional[str] = Query(None),
    user_id: Optional[int] = Query(None),
    resource_type: Optional[str] = Query(None),
    level: Optional[str] = Query(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """列出审计日志 (仅管理员)"""
    # TODO: Add admin permission check
    conditions = []
    if action:
        conditions.append(AuditLog.action.ilike(f"%{action}%"))
    if user_id:
        conditions.append(AuditLog.user_id == user_id)
    if resource_type:
        conditions.append(AuditLog.resource_type == resource_type)
    if level:
        conditions.append(AuditLog.level == level)

    count_q = select(func.count()).select_from(AuditLog).where(and_(*conditions))
    total = (await db.execute(count_q)).scalar() or 0

    q = select(AuditLog).where(and_(*conditions)).order_by(AuditLog.created_at.desc())
    q = q.offset((page - 1) * page_size).limit(page_size)
    logs = (await db.execute(q)).scalars().all()

    return ok({
        "items": [{
            "id": log.id,
            "user_id": log.user_id,
            "user_name": log.user_name,
            "action": log.action,
            "resource_type": log.resource_type,
            "resource_id": log.resource_id,
            "level": log.level,
            "description": log.description,
            "ip_address": log.ip_address,
            "status": log.status,
            "created_at": str(log.created_at) if log.created_at else None,
        } for log in logs],
        "total": total,
        "page": page,
        "page_size": page_size,
    })
