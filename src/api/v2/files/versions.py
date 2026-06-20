"""
Stora File Versions API — version history management
"""
from datetime import datetime
from typing import List, Optional

from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_

from shared.models import FileItem, FileVersion, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="", tags=["versions"])


async def _save_version(file: FileItem, db: AsyncSession) -> Optional[FileVersion]:
    """保存当前文件快照为新版本（可被其他模块调用）"""
    # Get next version number
    last = (await db.execute(
        select(func.max(FileVersion.version_number)).where(FileVersion.file_id == file.id)
    )).scalar() or 0

    v = FileVersion(
        file_id=file.id,
        user_id=file.user_id,
        version_number=last + 1,
        file_size=file.file_size,
        file_hash=file.file_hash,
        storage_path=getattr(file, "file_path", None) or getattr(file, "storage_path", ""),
        change_note="自动保存（在线编辑）",
    )
    db.add(v)
    return v


@router.get("/{file_id}/versions")
async def list_versions(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """列出文件的所有版本"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != int(current_user.get("sub", 0)):
        return fail("文件不存在")

    versions = (await db.execute(
        select(FileVersion).where(FileVersion.file_id == file_id).order_by(FileVersion.version_number.desc())
    )).scalars().all()

    return ok([{
        "id": v.id,
        "version_number": v.version_number,
        "file_size": v.file_size,
        "change_note": v.change_note,
        "created_at": str(v.created_at) if v.created_at else None,
    } for v in versions])


@router.post("/{file_id}/versions")
async def create_version(
    file_id: int,
    change_note: Optional[str] = "",
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """创建新版本（保留当前文件快照）"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != int(current_user.get("sub", 0)):
        return fail("文件不存在")

    # Get next version number
    last = (await db.execute(
        select(func.max(FileVersion.version_number)).where(FileVersion.file_id == file_id)
    )).scalar() or 0

    v = FileVersion(
        file_id=file_id,
        user_id=file.user_id,
        version_number=last + 1,
        file_size=file.file_size,
        file_hash=file.file_hash,
        storage_path=getattr(file, "storage_path", ""),
        change_note=change_note,
    )
    db.add(v)
    await db.commit()
    return ok({"version_number": v.version_number}, msg="版本已创建")


@router.post("/versions/{version_id}/restore")
async def restore_version(
    version_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """回滚到指定版本"""
    v = await db.get(FileVersion, version_id)
    if not v:
        return fail("版本不存在")

    file = await db.get(FileItem, v.file_id)
    if not file or file.user_id != int(current_user.get("sub", 0)):
        return fail("无权限")

    import os
    import shutil

    # Locate version storage path
    version_path = getattr(v, "storage_path", "")
    if not version_path or not os.path.exists(version_path):
        return fail("版本文件已丢失，无法恢复")

    # Save current file as new version before overwriting
    current_path = getattr(file, "file_path", None) or getattr(file, "storage_path", "")
    if current_path and os.path.exists(current_path):
        # Copy current file over the version file (so it becomes the new "latest version")
        shutil.copy2(current_path, version_path)

    # Now point the file to the version storage
    setattr(file, "file_path", version_path)
    file.file_hash = v.file_hash
    file.file_size = v.file_size
    await db.commit()
    return ok(msg=f"已回滚到版本 {v.version_number}")
