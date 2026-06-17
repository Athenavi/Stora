"""
MCP 媒体管理工具处理器
"""
from sqlalchemy import select
from pathlib import Path

from shared.models.media import Media
from src.utils.database.main import get_async_session_context


async def list_media(arguments: dict) -> dict:
    """获取媒体文件列表"""
    limit = min(arguments.get("limit", 20), 50)
    media_type = arguments.get("media_type", "").strip().lower()

    async with get_async_session_context() as db:
        q = select(Media).order_by(Media.created_at.desc()).limit(limit)
        if media_type:
            prefix = {"image": "image", "video": "video", "audio": "audio", "document": "application"}.get(media_type, media_type)
            q = q.where(Media.mime_type.startswith(prefix))

        media_list = (await db.execute(q)).scalars().all()
        return {"success": True, "total": len(media_list), "media": [{
            "id": m.id, "filename": m.original_filename or m.filename or "unknown",
            "mime_type": m.mime_type or "", "file_size": m.file_size or 0,
            "url": m.file_url or f"/media/{m.filename or ''}" if m.filename else "",
            "alt_text": m.alt_text or "", "category": m.category or "",
            "created_at": m.created_at.isoformat() if m.created_at else "",
        } for m in media_list]}


async def delete_media(arguments: dict) -> dict:
    """删除媒体文件"""
    media_id = arguments.get("media_id")
    if not media_id:
        raise ValueError("媒体ID不能为空")

    async with get_async_session_context() as db:
        media = await db.scalar(select(Media).where(Media.id == int(media_id)))
        if not media:
            raise ValueError(f"媒体 #{media_id} 不存在")
        await db.delete(media)
        await db.commit()
        return {"success": True, "message": f"媒体 #{media_id} 已删除", "media_id": media_id}
