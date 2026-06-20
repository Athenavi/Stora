"""
Stora Share API - 文件分享/链接管理路由
"""
import uuid
import hashlib
from datetime import datetime, timedelta
from typing import Optional, List

from fastapi import APIRouter, Depends, HTTPException, Query, Form
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_

from shared.models import FileItem, Folder, FileShare, ShareLink, User, AccessLog
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required, jwt_optional
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/shares", tags=["shares"])


@router.post("")
async def create_share(
    file_id: Optional[int] = Form(None),
    folder_id: Optional[int] = Form(None),
    permission: str = Form("read"),
    password: Optional[str] = Form(None),
    expires_in_hours: Optional[int] = Form(None),
    max_downloads: int = Form(0),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """创建分享链接"""
    if not file_id and not folder_id:
        return fail("请指定文件或文件夹")
    
    # Create share record
    share = FileShare(
        file_id=file_id,
        folder_id=folder_id,
        owner_id=current_user.id,
        permission=permission,
        is_link_share=True,
        expires_at=datetime.utcnow() + timedelta(hours=expires_in_hours) if expires_in_hours else None,
    )
    db.add(share)
    await db.flush()
    
    # Create share link
    short_code = uuid.uuid4().hex[:12]
    link = ShareLink(
        share_id=share.id,
        file_id=file_id,
        folder_id=folder_id,
        user_id=current_user.id,
        short_code=short_code,
        password_hash=_hash_password(password) if password else None,
        permission=permission,
        max_downloads=max_downloads,
        expires_at=share.expires_at,
    )
    db.add(link)
    await db.commit()
    await db.refresh(link)
    
    return ok({
        "id": link.id,
        "short_code": link.short_code,
        "url": f"/s/{short_code}",
        "permission": link.permission,
        "password_protected": bool(password),
        "expires_at": str(link.expires_at) if link.expires_at else None,
        "max_downloads": link.max_downloads,
    })


@router.get("")
async def list_shares(
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出我的分享"""
    q = select(ShareLink).where(
        ShareLink.user_id == current_user.id
    ).order_by(ShareLink.created_at.desc())
    q = q.offset((page - 1) * page_size).limit(page_size)
    items = (await db.execute(q)).scalars().all()
    
    total = (await db.execute(
        select(func.count()).select_from(ShareLink).where(ShareLink.user_id == current_user.id)
    )).scalar() or 0
    
    return ok({
        "items": [_link_to_dict(l) for l in items],
        "total": total,
        "page": page,
        "page_size": page_size,
    })


@router.delete("/{link_id}")
async def revoke_share(
    link_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """撤销分享链接"""
    link = await db.get(ShareLink, link_id)
    if not link or link.user_id != current_user.id:
        return fail("分享链接不存在")
    link.is_active = False
    await db.commit()
    return ok(msg="已撤销分享")


@router.get("/access/{short_code}")
async def access_share(
    short_code: str,
    password: Optional[str] = Query(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: Optional[dict] = Depends(jwt_optional),
):
    """访问分享链接（公开访问）"""
    link = (await db.execute(
        select(ShareLink).where(
            ShareLink.short_code == short_code,
            ShareLink.is_active == True,
        )
    )).scalar_one_or_none()
    
    if not link:
        return fail("分享链接不存在或已失效")
    
    # Check expiry
    if link.expires_at and datetime.utcnow() > link.expires_at:
        link.is_active = False
        await db.commit()
        return fail("分享链接已过期")
    
    # Check password
    if link.password_hash:
        if not password or _hash_password(password) != link.password_hash:
            return ok({"need_password": True, "protected": True})
    
    # Check download limit
    if link.max_downloads > 0 and link.download_count >= link.max_downloads:
        return fail("分享下载次数已达上限")
    
    link.view_count = (link.view_count or 0) + 1
    
    # Get shared item info
    info = {}
    if link.file_id:
        file = await db.get(FileItem, link.file_id)
        if file:
            info = _file_to_dict(file)
    elif link.folder_id:
        folder = await db.get(Folder, link.folder_id)
        if folder:
            info = {"id": folder.id, "name": folder.name, "type": "folder"}
    
    await db.commit()
    return ok({
        "share_info": _link_to_dict(link),
        "item": info,
        "password_protected": bool(link.password_hash),
    })


# ─── Helpers ───

def _hash_password(password: str) -> str:
    return hashlib.sha256(password.encode()).hexdigest()


def _link_to_dict(l: ShareLink) -> dict:
    return {
        "id": l.id,
        "short_code": l.short_code,
        "permission": l.permission,
        "is_active": l.is_active,
        "view_count": l.view_count,
        "download_count": l.download_count,
        "max_downloads": l.max_downloads,
        "password_protected": bool(l.password_hash),
        "expires_at": str(l.expires_at) if l.expires_at else None,
        "created_at": str(l.created_at) if l.created_at else None,
    }


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
