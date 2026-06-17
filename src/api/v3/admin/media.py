"""
V3 媒体管理 API

权限要求:
  GET    /media            → media:view
  POST   /media/upload     → media:upload
  DELETE /media/{id}       → media:delete

路由函数内无权限查询。
"""
import logging
from typing import Optional

from fastapi import APIRouter, Depends, Query, UploadFile, File
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.media import Media
from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db, get_current_user
from src.api.v3._permission import Permission
from shared.models.user import User

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-media"])


# 基于文件幻数的 MIME 类型检测（不依赖外部库）
_MAGIC_MAP: dict[bytes, str] = {
    b'\x89PNG\r\n\x1a\n': 'image/png',
    b'\xff\xd8\xff': 'image/jpeg',
    b'GIF87a': 'image/gif',
    b'GIF89a': 'image/gif',
    b'RIFF': 'image/webp',  # WebP 以 RIFF 开头，需进一步验证
    b'<?xml': 'image/svg+xml',
    b'<svg': 'image/svg+xml',
    b'%PDF': 'application/pdf',
    b'\x00\x00\x00 ftyp': 'video/mp4',
    b'\x00\x00\x00\x18ftyp': 'video/mp4',
    b'ftypmp42': 'video/mp4',
    b'\x1aE\xdf\xa3': 'video/webm',  # Matroska/WebM
    b'ID3': 'audio/mpeg',
    b'\xff\xfb': 'audio/mpeg',
    b'\xff\xf3': 'audio/mpeg',
    b'\xff\xe3': 'audio/mpeg',
    b'OggS': 'audio/ogg',
    b'RIFF': 'audio/wav',  # WAV 也是 RIFF，需区分
}


def _detect_mime_from_bytes(data: bytes) -> Optional[str]:
    """从文件内容前若干字节检测 MIME 类型"""
    if not data:
        return None
    for signature, mime in _MAGIC_MAP.items():
        if data[:len(signature)] == signature:
            if mime == 'image/webp':
                # 进一步验证 WebP: 第 9-12 字节应为 'WEBP'
                if data[8:12] == b'WEBP':
                    return 'image/webp'
                return None
            if mime == 'audio/wav':
                # 区分 WAV 和 WebP: WAV 的 8-11 字节为 'WAVE'
                if data[8:12] == b'WAVE':
                    return 'audio/wav'
                return None
            return mime
    return None


# ============================================================
# 列表
# ============================================================

@router.get("/media", summary="媒体列表")
async def list_media(
    page: int = Query(1, ge=1),
    per_page: int = Query(20, ge=1, le=100),
    media_type: Optional[str] = Query(None, alias="type"),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("media:view")),
):
    query = select(Media)

    if media_type:
        query = query.where(Media.media_type == media_type)

    total = await db.scalar(
        select(func.count()).select_from(query.subquery())
    ) or 0

    offset = (page - 1) * per_page
    result = await db.execute(
        query.order_by(Media.created_at.desc()).offset(offset).limit(per_page)
    )
    items = result.scalars().all()

    return ApiResponse(success=True, data={
        "media": [_media_to_dict(m) for m in items],
        "pagination": {
            "page": page,
            "per_page": per_page,
            "total": total,
            "total_pages": (total + per_page - 1) // per_page,
        },
    })


# ============================================================
# 上传
# ============================================================

@router.post("/media/upload", summary="上传文件")
async def upload_media(
    file: UploadFile = File(...),
    folder_id: Optional[int] = Query(None),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(Permission("media:upload")),
):
    """上传媒体文件"""
    # 文件大小限制（默认 50MB）
    MAX_SIZE = 50 * 1024 * 1024
    ALLOWED_MIMES = {
        # 图片
        'image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/svg+xml',
        'image/bmp', 'image/tiff', 'image/avif',
        # 视频
        'video/mp4', 'video/webm', 'video/quicktime', 'video/x-msvideo',
        'video/x-matroska', 'video/mpeg',
        # 音频
        'audio/mpeg', 'audio/wav', 'audio/ogg', 'audio/aac',
        'audio/flac', 'audio/opus', 'audio/webm',
        # 文档
        'application/pdf', 'text/plain', 'text/markdown', 'text/csv', 'text/html',
        'application/msword',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        'application/vnd.ms-excel',
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        'application/vnd.ms-powerpoint',
        'application/vnd.openxmlformats-officedocument.presentationml.presentation',
        # 压缩包
        'application/zip', 'application/x-rar-compressed', 'application/x-7z-compressed',
        'application/gzip', 'application/x-tar', 'application/x-bzip2',
        # 数据
        'application/json', 'application/xml',
        # 通用
        'application/octet-stream',
    }

    content = await file.read()

    if len(content) > MAX_SIZE:
        return ApiResponse(success=False, error=f"文件大小不能超过 {MAX_SIZE // 1024 // 1024}MB")

    mime = file.content_type or 'application/octet-stream'
    if mime not in ALLOWED_MIMES:
        return ApiResponse(success=False, error=f"不支持的文件类型: {mime}")

    # 二次校验：使用文件幻数验证 MIME（防止 Content-Type 伪造）
    _detected_mime = _detect_mime_from_bytes(content)
    if _detected_mime and _detected_mime not in ALLOWED_MIMES:
        return ApiResponse(success=False, error=f"文件内容与声明类型不符")

    # 保存文件到存储
    from src.api.v2.media_v1pack.upload_service import save_uploaded_file
    result = save_uploaded_file(
        file_content=content,
        filename=file.filename or "untitled",
        content_type=file.content_type or "application/octet-stream",
        user_id=current_user.id,
    )

    if not result.get("success"):
        return ApiResponse(success=False, error=result.get("error", "上传失败"))

    # 写入数据库
    from datetime import datetime, timezone
    now = datetime.now(timezone.utc)
    media = Media(
        filename=result["filename"],
        original_filename=file.filename,
        filepath=result["filepath"],
        media_type=result.get("media_type", "image"),
        file_size=len(content),
        mime_type=file.content_type,
        user_id=current_user.id,
        folder_id=folder_id,
        created_at=now,
        updated_at=now,
    )
    db.add(media)
    await db.commit()
    await db.refresh(media)

    return ApiResponse(success=True, data=_media_to_dict(media), message="上传成功")


# ============================================================
# 删除
# ============================================================

@router.delete("/media/{media_id}", summary="删除文件")
async def delete_media(
    media_id: int,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(Permission("media:delete")),
):
    media = await db.get(Media, media_id)
    if not media:
        return ApiResponse(success=False, error="文件不存在")

    # 所有权校验
    if media.user_id != current_user.id and not current_user.is_superuser:
        return ApiResponse(success=False, error="无权删除此文件")

    # 删除物理文件（校验路径安全性）
    import os
    filepath = media.filepath
    if filepath:
        # 路径穿越防护：确保路径在 storage 目录下
        safe_path = os.path.normpath(filepath)
        if not safe_path.startswith("storage") and not safe_path.startswith("/app/storage"):
            logger.warning(f"拒绝删除非存储目录文件: {filepath}")
            return ApiResponse(success=False, error="无效的文件路径")
        if os.path.exists(safe_path):
            try:
                os.remove(safe_path)
            except OSError as e:
                logger.warning(f"删除物理文件失败: {safe_path} — {e}")

    await db.delete(media)
    await db.commit()

    return ApiResponse(success=True, message="文件已删除")


# ============================================================
# 辅助函数
# ============================================================

def _media_to_dict(m: Media) -> dict:
    return {
        "id": m.id,
        "filename": m.filename,
        "original_filename": getattr(m, 'original_filename', None),
        "media_type": m.media_type,
        "file_size": m.file_size,
        "mime_type": getattr(m, 'mime_type', None),
        "url": m.filepath,
        "created_at": m.created_at.isoformat() if m.created_at else None,
    }
