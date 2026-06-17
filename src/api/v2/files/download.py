"""
Stora Files API - 文件下载路由
"""
import os
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import FileResponse
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import FileItem, DownloadToken, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/download", tags=["download"])


@router.get("/{file_id}")
async def download_file(
    file_id: int,
    token: Optional[str] = Query(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """下载文件（验证权限或分享令牌）"""
    file = await db.get(FileItem, file_id)
    if not file:
        return fail("文件不存在")
    
    # Check ownership or share token
    if file.user_id != current_user.id and not token:
        return fail("无权限下载")
    
    if token:
        token_record = (await db.execute(
            select(DownloadToken).where(
                DownloadToken.token == token,
                DownloadToken.file_id == file_id,
                DownloadToken.is_used == False,
            )
        )).scalar_one_or_none()
        if not token_record:
            return fail("下载令牌无效或已使用")
        token_record.is_used = True
    
    storage_path = getattr(file, "storage_path", None)
    if not storage_path or not os.path.exists(storage_path):
        return fail("文件存储路径不存在")
    
    # Update download count
    file.download_count = (file.download_count or 0) + 1
    await db.commit()
    
    return FileResponse(
        path=storage_path,
        filename=file.filename or "download",
        media_type=file.mime_type or "application/octet-stream",
    )


@router.get("/token/{file_id}")
async def get_download_token(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取一次性下载令牌"""
    file = await db.get(FileItem, file_id)
    if not file:
        return fail("文件不存在")
    
    import uuid
    from datetime import datetime, timedelta
    
    token_str = uuid.uuid4().hex
    token = DownloadToken(
        token=token_str,
        file_id=file_id,
        user_id=current_user.id,
        expires_at=datetime.utcnow() + timedelta(hours=1),
    )
    db.add(token)
    await db.commit()
    
    return ok({"token": token_str, "expires_in": 3600})
