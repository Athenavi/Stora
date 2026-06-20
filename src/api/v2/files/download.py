"""
Stora Files API - 文件下载路由
"""
import io
import os
import zipfile
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import FileResponse, StreamingResponse
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import FileItem, DownloadToken, User, Folder
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["download"])


class BatchDownloadRequest(BaseModel):
    file_ids: list[int]
    folder_ids: list[int] = []


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


@router.post("/batch")
async def batch_download(
    req: BatchDownloadRequest,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """批量下载：将选中的文件和文件夹打包为 ZIP 流式返回"""
    # 收集文件 ID：直接传入 + 文件夹递归
    all_file_ids = list(req.file_ids)

    for fid in req.folder_ids:
        children = await _collect_folder_files(db, fid, current_user.id)
        all_file_ids.extend(children)

    if not all_file_ids:
        return fail("没有可下载的文件")

    # 去重
    all_file_ids = list(set(all_file_ids))

    # 批量查询文件
    result = await db.execute(
        select(FileItem).where(
            FileItem.id.in_(all_file_ids),
            FileItem.user_id == current_user.id,
            FileItem.deleted_at.is_(None),
        )
    )
    files = result.scalars().all()

    if not files:
        return fail("没有可下载的文件")

    CHUNK_SIZE = 65536
    MAX_INLINE_SIZE = 2 * 1024 * 1024 * 1024  # 2GB 以上文件不压缩，放链接引用

    async def iter_zip():
        buffer = io.BytesIO()
        with zipfile.ZipFile(buffer, "w", zipfile.ZIP_DEFLATED) as zf:
            for f in files:
                storage_path = getattr(f, "file_path", None) or getattr(f, "storage_path", None)
                if not storage_path or not os.path.exists(storage_path):
                    # 文件不存在，写入一个提示
                    info = zipfile.ZipInfo(f"_缺失/{f.filename or 'unknown'}.txt")
                    zf.writestr(info, f"文件不存在或路径丢失: {storage_path}\n")
                    continue

                arcname = f.filename or f"file_{f.id}"
                file_size = f.file_size or 0

                if file_size > MAX_INLINE_SIZE:
                    # 超大文件：写入链接引用文件
                    info = zipfile.ZipInfo(f"{arcname}.url")
                    zf.writestr(info, f"[InternetShortcut]\nURL=/api/v2/files/download/{f.id}\n")
                else:
                    # 正常文件：直接压缩
                    with open(storage_path, "rb") as fh:
                        zf.writestr(arcname, fh.read())

                # 每处理一个文件 yield 一次 buffer 内容
                buffer.seek(0)
                data = buffer.read()
                buffer.seek(0)
                buffer.truncate()
                if data:
                    yield data

        # 最终剩余数据
        buffer.seek(0)
        remaining = buffer.read()
        if remaining:
            yield remaining
        buffer.close()

    filename_encoded = f"stora-batch-{len(files)}files.zip"
    return StreamingResponse(
        iter_zip(),
        media_type="application/zip",
        headers={
            "Content-Disposition": f'attachment; filename="{filename_encoded}"',
        },
    )


async def _collect_folder_files(
    db: AsyncSession, folder_id: int, user_id: int
) -> list[int]:
    """递归收集文件夹下所有文件 ID"""
    file_ids: list[int] = []

    # 查询文件夹下的文件
    result = await db.execute(
        select(FileItem.id).where(
            FileItem.folder_id == folder_id,
            FileItem.user_id == user_id,
            FileItem.deleted_at.is_(None),
        )
    )
    for row in result.all():
        file_ids.append(row[0])

    # 查询子文件夹
    result = await db.execute(
        select(Folder).where(
            Folder.parent_id == folder_id,
            Folder.user_id == user_id,
        )
    )
    sub_folders = result.scalars().all()

    for sf in sub_folders:
        child_ids = await _collect_folder_files(db, sf.id, user_id)
        file_ids.extend(child_ids)

    return file_ids


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
