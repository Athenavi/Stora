"""
Stora Mobile Media API — file upload & management for mobile
"""
from fastapi import APIRouter, Depends, UploadFile, File, Form
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import FileItem, User
from shared.services.files import create_file
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["mobile-media"])


@router.post("/upload")
async def mobile_upload(
    file: UploadFile = File(...),
    folder_id: int = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """移动端文件上传"""
    user = await db.get(User, int(current_user.get("sub", 0)))
    content = await file.read()
    try:
        f = await create_file(db, user.id, file.filename, content, folder_id, file.content_type or "")
        return ok({"id": f.id, "filename": f.filename, "file_size": f.file_size, "file_type": f.file_type})
    except ValueError as e:
        return fail(str(e))


@router.get("/files")
async def mobile_files(
    folder_id: int = None,
    page: int = 1,
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """移动端文件列表"""
    from sqlalchemy import select, func, and_
    user_id = int(current_user.get("sub", 0))
    conditions = [FileItem.user_id == user_id, FileItem.deleted_at.is_(None)]
    if folder_id:
        conditions.append(FileItem.folder_id == folder_id)
    else:
        conditions.append(FileItem.folder_id.is_(None))
    q = select(FileItem).where(and_(*conditions)).order_by(FileItem.created_at.desc()).offset((page-1)*20).limit(20)
    items = (await db.execute(q)).scalars().all()
    return ok([{"id": f.id, "filename": f.filename, "file_size": f.file_size, "file_type": f.file_type} for f in items])
