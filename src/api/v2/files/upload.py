"""
Stora Files API - 文件上传/分片上传/秒传路由
"""
import os
import hashlib
import uuid
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, UploadFile, File, Form, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import FileItem, FileFingerprint, Folder, UploadTask, UploadChunk, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["upload"])

UPLOAD_DIR = os.path.join(os.getcwd(), "storage", "uploads")
CHUNK_DIR = os.path.join(os.getcwd(), "storage", "chunks")
os.makedirs(UPLOAD_DIR, exist_ok=True)
os.makedirs(CHUNK_DIR, exist_ok=True)


@router.post("")
async def upload_file(
    file: UploadFile = File(...),
    folder_id: Optional[int] = Form(None),
    file_hash: Optional[str] = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """单文件上传（支持秒传）"""
    content = await file.read()
    actual_hash = hashlib.sha256(content).hexdigest()
    
    # Check instant upload (hash match)
    if file_hash and file_hash == actual_hash:
        fingerprint = (await db.execute(
            select(FileFingerprint).where(FileFingerprint.hash == actual_hash)
        )).scalar_one_or_none()
        if fingerprint:
            # Instant upload: create reference to existing file
            fingerprint.reference_count += 1
            new_file = FileItem(
                user_id=current_user.id,
                folder_id=folder_id,
                filename=file.filename,
                original_filename=file.filename,
                file_size=fingerprint.file_size,
                mime_type=file.content_type or fingerprint.mime_type,
                file_hash=actual_hash,
                storage_path=fingerprint.storage_path,
            )
            db.add(new_file)
            await db.commit()
            await db.refresh(new_file)
            new_file.file_url = f"/api/v2/files/download/{new_file.id}"
            await db.commit()
            return ok({"file": _file_to_dict(new_file), "instant": True})
    
    # Save file
    ext = os.path.splitext(file.filename)[1] or ""
    storage_name = f"{uuid.uuid4().hex}{ext}"
    storage_path = os.path.join(UPLOAD_DIR, storage_name)
    with open(storage_path, "wb") as f:
        f.write(content)
    
    # Detect file type
    file_type = _detect_file_type(file.content_type or "", file.filename)
    
    # Check for existing fingerprint (deduplication)
    existing_fp = (await db.execute(
        select(FileFingerprint).where(FileFingerprint.hash == actual_hash)
    )).scalar_one_or_none()
    if existing_fp:
        fingerprint = existing_fp
        fingerprint.reference_count += 1
        os.remove(storage_path)  # remove duplicate file, reuse existing one
    else:
        fingerprint = FileFingerprint(
            hash=actual_hash,
            file_size=len(content),
            mime_type=file.content_type or "application/octet-stream",
            storage_path=storage_path,
            reference_count=1,
        )
        db.add(fingerprint)

    # Resolve storage path (use existing fingerprint's path if deduplicated)
    resolved_storage_path = fingerprint.storage_path

    # Create file record
    new_file = FileItem(
        user_id=current_user.id,
        folder_id=folder_id,
        filename=file.filename,
        original_filename=file.filename,
        file_size=len(content),
        mime_type=file.content_type,
        file_type=file_type,
        file_hash=actual_hash,
        file_path=resolved_storage_path,
        storage_driver="local",
    )
    db.add(new_file)
    await db.commit()
    await db.refresh(new_file)
    
    # Set correct download URL with file ID
    new_file.file_url = f"/api/v2/files/download/{new_file.id}"
    
    # Update quota via StorageQuota model
    from shared.models import StorageQuota
    quota = (await db.execute(
        select(StorageQuota).where(StorageQuota.user_id == current_user.id)
    )).scalar_one_or_none()
    if quota:
        quota.used_storage = (quota.used_storage or 0) + len(content)
    
    await db.commit()
    
    return ok({"file": _file_to_dict(new_file), "instant": False})


@router.post("/init")
async def init_chunked_upload(
    filename: str = Form(...),
    total_size: int = Form(...),
    total_chunks: int = Form(...),
    file_hash: Optional[str] = Form(None),
    folder_id: Optional[int] = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """初始化分片上传"""
    upload_id = uuid.uuid4().hex
    task = UploadTask(
        id=upload_id,
        user_id=current_user.id,
        filename=filename,
        total_size=total_size,
        total_chunks=total_chunks,
        file_hash=file_hash or "",
        status="initialized",
    )
    db.add(task)
    await db.commit()
    return ok({
        "upload_id": upload_id,
        "chunk_size": 5 * 1024 * 1024,  # 5MB default chunk
    })


@router.post("/chunk")
async def upload_chunk(
    upload_id: str = Form(...),
    chunk_index: int = Form(...),
    chunk: UploadFile = File(...),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """上传分片"""
    task = await db.get(UploadTask, upload_id)
    if not task or task.user_id != current_user.id:
        return fail("上传任务不存在")
    
    content = await chunk.read()
    chunk_hash = hashlib.sha256(content).hexdigest()
    
    # Save chunk
    chunk_dir = os.path.join(CHUNK_DIR, upload_id)
    os.makedirs(chunk_dir, exist_ok=True)
    chunk_path = os.path.join(chunk_dir, f"{chunk_index}")
    with open(chunk_path, "wb") as f:
        f.write(content)
    
    # Record chunk
    record = UploadChunk(
        upload_id=upload_id,
        chunk_index=chunk_index,
        chunk_hash=chunk_hash,
        chunk_size=len(content),
        chunk_path=chunk_path,
    )
    db.add(record)
    
    task.uploaded_chunks = (task.uploaded_chunks or 0) + 1
    await db.commit()
    return ok({"chunk_index": chunk_index})


@router.post("/complete")
async def complete_upload(
    upload_id: str = Form(...),
    folder_id: Optional[int] = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """完成分片上传，合并文件"""
    task = await db.get(UploadTask, upload_id)
    if not task or task.user_id != current_user.id:
        return fail("上传任务不存在")
    
    if task.uploaded_chunks < task.total_chunks:
        return fail(f"分片不完整: {task.uploaded_chunks}/{task.total_chunks}")
    
    # Merge chunks
    chunk_dir = os.path.join(CHUNK_DIR, upload_id)
    ext = os.path.splitext(task.filename)[1] or ""
    storage_name = f"{uuid.uuid4().hex}{ext}"
    storage_path = os.path.join(UPLOAD_DIR, storage_name)
    
    with open(storage_path, "wb") as outfile:
        for i in range(task.total_chunks):
            chunk_path = os.path.join(chunk_dir, str(i))
            if os.path.exists(chunk_path):
                with open(chunk_path, "rb") as infile:
                    outfile.write(infile.read())
    
    # Calculate hash
    sha256 = hashlib.sha256()
    with open(storage_path, "rb") as f:
        for block in iter(lambda: f.read(65536), b""):
            sha256.update(block)
    file_hash = sha256.hexdigest()
    
    # Detect file type
    file_type = _detect_file_type("", task.filename)
    import mimetypes
    mime_type = mimetypes.guess_type(task.filename)[0] or "application/octet-stream"
    
    # Create file record
    new_file = FileItem(
        user_id=current_user.id,
        folder_id=folder_id,
        filename=task.filename,
        original_filename=task.filename,
        file_size=task.total_size,
        mime_type=mime_type,
        file_type=file_type,
        file_hash=file_hash,
        file_path=storage_path,
        storage_driver="local",
    )
    db.add(new_file)
    
    task.status = "completed"
    await db.commit()
    await db.refresh(new_file)
    
    # Set correct download URL
    new_file.file_url = f"/api/v2/files/download/{new_file.id}"
    await db.commit()
    
    # Cleanup chunks
    import shutil
    shutil.rmtree(chunk_dir, ignore_errors=True)
    
    return ok({"file": _file_to_dict(new_file)})


@router.post("/check")
async def check_upload(
    file_hash: str = Query(...),
    filename: str = Query(...),
    file_size: int = Query(...),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """检查文件哈希，支持秒传"""
    fingerprint = (await db.execute(
        select(FileFingerprint).where(FileFingerprint.hash == file_hash)
    )).scalar_one_or_none()
    
    if fingerprint and fingerprint.file_size == file_size:
        # Check user quota
        return ok({
            "exists": True,
            "size": fingerprint.file_size,
            "mime_type": fingerprint.mime_type,
        })
    return ok({"exists": False})


# ─── Helpers ───

def _file_to_dict(f: FileItem) -> dict:
    return {
        "id": f.id,
        "filename": f.filename,
        "original_filename": f.original_filename,
        "file_size": f.file_size,
        "mime_type": f.mime_type,
        "file_type": f.file_type,
        "folder_id": f.folder_id,
        "is_favorite": f.is_favorite,
        "is_folder": f.is_folder,
        "thumbnail_url": f.thumbnail_url,
        "file_url": f.file_url,
        "description": f.description,
        "download_count": f.download_count,
        "width": f.width,
        "height": f.height,
        "duration": f.duration,
        "file_hash": f.file_hash,
        "storage_driver": f.storage_driver,
        "created_at": str(f.created_at) if f.created_at else None,
        "updated_at": str(f.updated_at) if f.updated_at else None,
    }


def _detect_file_type(mime_type: str, filename: str) -> str:
    ext = os.path.splitext(filename)[1].lower()
    image_exts = {".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".ico"}
    video_exts = {".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".webm"}
    audio_exts = {".mp3", ".wav", ".ogg", ".flac", ".aac", ".wma"}
    doc_exts = {".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt"}
    archive_exts = {".zip", ".rar", ".7z", ".tar", ".gz", ".bz2"}
    
    if ext in image_exts or mime_type.startswith("image/"):
        return "image"
    if ext in video_exts or mime_type.startswith("video/"):
        return "video"
    if ext in audio_exts or mime_type.startswith("audio/"):
        return "audio"
    if ext in doc_exts:
        return "document"
    if ext in archive_exts:
        return "archive"
    return "other"
