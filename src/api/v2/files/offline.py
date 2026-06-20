"""
Stora Offline Download API — download files from URLs
"""
import asyncio
import hashlib
import os
import uuid
from datetime import datetime

from fastapi import APIRouter, BackgroundTasks, Depends, Form
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import DownloadTask, FileItem, FileFingerprint, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required
from src.extensions import get_async_db_session as get_async_db
from src.api.v2.files.upload import _detect_file_type

router = APIRouter(prefix="", tags=["offline-download"])

UPLOAD_DIR = os.path.join(os.getcwd(), "storage", "uploads")
os.makedirs(UPLOAD_DIR, exist_ok=True)


async def _do_download(task_id: int, source_url: str, filename: str, user_id: int):
    """后台执行离线下载——获取 HTTP 内容，保存为 FileItem"""
    from src.extensions import get_async_session_context
    from shared.models.file.download_task import DownloadTask as DT

    async with get_async_session_context() as db:
        task = await db.get(DT, task_id)
        if not task:
            return

        task.status = "downloading"
        await db.commit()

        try:
            import httpx
            async with httpx.AsyncClient(timeout=300) as client:
                resp = await client.get(source_url, follow_redirects=True)
                resp.raise_for_status()
                content = resp.content

            # Determine filename from content-disposition or URL
            if not filename or filename == "download":
                cd = resp.headers.get("content-disposition", "")
                if "filename=" in cd:
                    filename = cd.split("filename=")[-1].strip('" ')
                else:
                    filename = source_url.split("/")[-1].split("?")[0] or "download"

            ext = os.path.splitext(filename)[1] or ""
            storage_name = f"{uuid.uuid4().hex}{ext}"
            storage_path = os.path.join(UPLOAD_DIR, storage_name)

            with open(storage_path, "wb") as f:
                f.write(content)

            # File hash
            actual_hash = hashlib.sha256(content).hexdigest()
            file_type = _detect_file_type(resp.headers.get("content-type", ""), filename)
            mime_type = resp.headers.get("content-type", "application/octet-stream")

            # Save fingerprint
            fp = FileFingerprint(
                hash=actual_hash,
                file_size=len(content),
                mime_type=mime_type,
                storage_path=storage_path,
                reference_count=1,
            )
            db.add(fp)

            # Create file record
            new_file = FileItem(
                user_id=user_id,
                filename=filename,
                original_filename=filename,
                file_size=len(content),
                mime_type=mime_type,
                file_type=file_type,
                file_hash=actual_hash,
                file_path=storage_path,
                storage_driver="local",
            )
            db.add(new_file)
            await db.flush()

            new_file.file_url = f"/api/v2/files/download/{new_file.id}"

            # Update task
            task.status = "completed"
            task.progress = 100
            task.file_id = new_file.id
            await db.commit()

        except Exception as exc:
            task.status = "failed"
            await db.commit()


@router.post("/offline-download")
async def create_offline_task(
    source_url: str = Form(...),
    filename: str = Form(""),
    resource_type: str = Form("other"),
    background_tasks: BackgroundTasks = BackgroundTasks(),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """创建离线下载任务（后台异步下载）"""
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

    # 启动后台下载
    background_tasks.add_task(
        _do_download,
        task.id,
        source_url,
        task.filename,
        int(current_user.get("sub", 0)),
    )

    return ok({"id": task.id, "filename": task.filename, "status": task.status})


@router.get("/offline-download/tasks")
async def list_offline_tasks(
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """列出离线下载任务"""
    user_id = int(current_user.get("sub", 0))
    tasks = (await db.execute(
        select(DownloadTask).where(DownloadTask.user_id == user_id).order_by(DownloadTask.created_at.desc()).limit(20)
    )).scalars().all()
    return ok([{
        "id": t.id, "filename": t.filename, "source_url": t.source_url,
        "status": t.status, "progress": t.progress, "file_id": t.file_id,
        "created_at": str(t.created_at) if t.created_at else None,
    } for t in tasks])
