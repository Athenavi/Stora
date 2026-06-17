"""
Stora 文件预览 API — 图片缩略图、视频流、文本预览、PDF
"""
import os
import mimetypes
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import FileResponse, Response, StreamingResponse
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import FileItem, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/preview", tags=["preview"])

THUMB_DIR = os.path.join(os.getcwd(), "storage", "thumbs")
os.makedirs(THUMB_DIR, exist_ok=True)

TEXT_EXTENSIONS = {
    ".txt", ".md", ".json", ".xml", ".html", ".css", ".js", ".ts", ".py",
    ".java", ".cpp", ".h", ".c", ".rb", ".go", ".rs", ".sh", ".yaml", ".yml",
    ".toml", ".ini", ".cfg", ".log", ".csv", ".sql", ".r", ".swift", ".kt",
}


@router.get("/{file_id}")
async def preview_file(
    file_id: int,
    t: str = Query("raw", description="预览类型: raw/thumb/stream"),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """文件预览 — 自动根据文件类型返回最好的预览方式"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")

    storage_path = getattr(file, "storage_path", None)
    if not storage_path or not os.path.exists(storage_path):
        return fail("文件存储路径不存在")

    file_type = file.file_type or "other"
    mime = file.mime_type or "application/octet-stream"
    ext = os.path.splitext(file.filename or "").lower()

    # ─── Image: return the file directly ───
    if file_type == "image":
        return FileResponse(storage_path, media_type=mime)

    # ─── Video/Audio: streaming ───
    if file_type in ("video", "audio"):
        return _stream_media(storage_path, mime, file.filename)

    # ─── Text/Code: return as text ───
    if ext in TEXT_EXTENSIONS or file_type == "document" and ext in {".txt", ".md"}:
        try:
            with open(storage_path, "r", encoding="utf-8", errors="replace") as f:
                content = f.read()
            return Response(content=content, media_type="text/plain; charset=utf-8")
        except Exception:
            return fail("无法读取文件内容")

    # ─── PDF ───
    if ext == ".pdf":
        return FileResponse(storage_path, media_type="application/pdf")

    # ─── Fallback: force download ───
    return FileResponse(storage_path, media_type=mime, filename=file.filename)


@router.get("/{file_id}/thumbnail")
async def file_thumbnail(
    file_id: int,
    size: int = Query(256, description="缩略图尺寸"),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取文件缩略图（仅图片类型）"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")

    # Use existing thumbnail if available
    if file.thumbnail_url and os.path.exists(file.thumbnail_url):
        return FileResponse(file.thumbnail_url)

    # Generate thumbnail for images
    storage_path = getattr(file, "storage_path", None)
    if not storage_path or not os.path.exists(storage_path):
        return fail("文件不存在")

    if file.file_type != "image":
        return fail("不支持的文件类型")

    thumb_path = os.path.join(THUMB_DIR, f"{file_id}_{size}.jpg")
    if not os.path.exists(thumb_path):
        try:
            _generate_thumbnail(storage_path, thumb_path, size)
        except Exception as e:
            return fail(f"生成缩略图失败: {e}")

    return FileResponse(thumb_path, media_type="image/jpeg")


# ─── Stream helpers ───

def _stream_media(path: str, mime: str, filename: str):
    """视频/音频流式传输"""
    file_size = os.path.getsize(path)

    async def iter_file(start: int, end: int):
        with open(path, "rb") as f:
            f.seek(start)
            remaining = end - start + 1
            while remaining > 0:
                chunk = f.read(min(65536, remaining))
                if not chunk:
                    break
                remaining -= len(chunk)
                yield chunk

    return StreamingResponse(
        iter_file(0, file_size - 1),
        media_type=mime,
        headers={
            "Content-Disposition": f'inline; filename="{filename}"',
            "Accept-Ranges": "bytes",
            "Content-Length": str(file_size),
        },
    )


def _generate_thumbnail(src: str, dst: str, size: int):
    """Generate thumbnail using Pillow"""
    from PIL import Image
    img = Image.open(src)
    img.thumbnail((size, size), Image.LANCZOS)
    if img.mode in ("RGBA", "P"):
        img = img.convert("RGB")
    img.save(dst, "JPEG", quality=85)
