"""
Stora Home API — returns minimal netdisk homepage data.
"""
from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import FileItem, User
from shared.services.files import get_user_quota
from src.api.v2._helpers import ok
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["home"])


@router.get("/api/v2/home")
async def get_home_data(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """Get homepage data for Stora netdisk."""
    quota = await get_user_quota(db, current_user.id)

    recent_files = []
    from sqlalchemy import select
    q = select(FileItem).where(
        FileItem.user_id == current_user.id,
        FileItem.deleted_at.is_(None),
        FileItem.is_folder == False,
    ).order_by(FileItem.created_at.desc()).limit(10)
    result = await db.execute(q)
    recent_files = result.scalars().all()

    return ok({
        "quota": quota,
        "recentFiles": [
            {
                "id": f.id,
                "filename": f.filename,
                "file_size": f.file_size,
                "mime_type": f.mime_type,
                "created_at": str(f.created_at) if f.created_at else None,
            }
            for f in recent_files
        ],
        "quickActions": ["upload", "create_folder", "view_shares"],
    })
