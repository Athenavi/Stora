"""
Stora 分享服务 — 文件分享/链接管理业务逻辑
"""
import uuid
import hashlib
from datetime import datetime, timedelta
from typing import Optional

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import ShareLink, FileShare, FileItem, Folder, User


def _hash_password(password: str) -> str:
    return hashlib.sha256(password.encode()).hexdigest()


async def create_share_link(
    db: AsyncSession,
    user_id: int,
    file_id: Optional[int] = None,
    folder_id: Optional[int] = None,
    permission: str = "read",
    password: Optional[str] = None,
    expires_in_hours: Optional[int] = None,
    max_downloads: int = 0,
) -> dict:
    """创建分享链接"""
    share = FileShare(
        file_id=file_id,
        folder_id=folder_id,
        owner_id=user_id,
        permission=permission,
        is_link_share=True,
        expires_at=datetime.utcnow() + timedelta(hours=expires_in_hours) if expires_in_hours else None,
    )
    db.add(share)
    await db.flush()

    short_code = uuid.uuid4().hex[:12]
    link = ShareLink(
        share_id=share.id,
        file_id=file_id,
        folder_id=folder_id,
        user_id=user_id,
        short_code=short_code,
        password_hash=_hash_password(password) if password else None,
        permission=permission,
        max_downloads=max_downloads,
        expires_at=share.expires_at,
    )
    db.add(link)
    await db.commit()
    await db.refresh(link)

    return {
        "id": link.id,
        "short_code": link.short_code,
        "url": f"/s/{short_code}",
        "permission": link.permission,
        "password_protected": bool(password),
        "expires_at": str(link.expires_at) if link.expires_at else None,
        "max_downloads": link.max_downloads,
    }


async def validate_share_access(
    db: AsyncSession,
    short_code: str,
    password: Optional[str] = None,
) -> tuple:
    """验证分享链接访问权限，返回 (link, item_info, error)"""
    link = (await db.execute(
        select(ShareLink).where(
            ShareLink.short_code == short_code,
            ShareLink.is_active == True,
        )
    )).scalar_one_or_none()

    if not link:
        return None, None, "分享链接不存在或已失效"

    if link.expires_at and datetime.utcnow() > link.expires_at:
        link.is_active = False
        await db.commit()
        return None, None, "分享链接已过期"

    if link.password_hash:
        if not password or _hash_password(password) != link.password_hash:
            return None, None, "需要密码"

    if link.max_downloads > 0 and link.download_count >= link.max_downloads:
        return None, None, "下载次数已达上限"

    link.view_count = (link.view_count or 0) + 1
    await db.commit()
    return link, None, None
