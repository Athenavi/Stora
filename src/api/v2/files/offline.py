"""
Stora Offline Download API — download files from URLs
"""
from datetime import datetime

from fastapi import APIRouter, Depends, Form
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import DownloadTask, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="", tags=["offline-download"])


@router.post("/api/v2/offline-download")
async def create_offline_task(
    source_url: str = Form(...),
    filename: str = Form(""),
    resource_type: str = Form("other"),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """创建离线下载任务"""
    task = DownloadTask(
        user_id=int(current_user.get("sub", 0)),
        source_url=source_url,
        filename=filename or source_url.split("/")[-1][:200] or "download",
        resource_type=resource_type,
        status="pending",
    )
    db.add(task)
    await db.commit()
    await db.refresh(task)
    return ok({"id": task.id, "filename": task.filename, "status": task.status})


@router.get("/api/v2/offline-download/tasks")
async def list_offline_tasks(
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """列出离线下载任务"""
    from sqlalchemy import select
    user_id = int(current_user.get("sub", 0))
    tasks = (await db.execute(
        select(DownloadTask).where(DownloadTask.user_id == user_id).order_by(DownloadTask.created_at.desc()).limit(20)
    )).scalars().all()
    return ok([{
        "id": t.id, "filename": t.filename, "source_url": t.source_url,
        "status": t.status, "progress": t.progress,
        "created_at": str(t.created_at) if t.created_at else None,
    } for t in tasks])
