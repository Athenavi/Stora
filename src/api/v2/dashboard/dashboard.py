"""
Stora Dashboard API — netdisk dashboard overview.
"""
from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func

from shared.models import FileItem, Folder, User
from shared.services.files import get_user_quota
from src.api.v2._helpers import ok
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["dashboard"])


@router.get("")
async def get_dashboard(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """Get dashboard overview data."""
    quota = await get_user_quota(db, current_user.id)

    # Count files by type
    type_counts = {}
    for t in ("image", "video", "audio", "document", "archive", "other"):
        cnt = (await db.execute(
            select(func.count()).select_from(FileItem).where(
                FileItem.user_id == current_user.id,
                FileItem.deleted_at.is_(None),
                FileItem.file_type == t,
            )
        )).scalar() or 0
        type_counts[t] = cnt

    # Recent activity
    recent = (await db.execute(
        select(FileItem).where(
            FileItem.user_id == current_user.id,
            FileItem.deleted_at.is_(None),
        ).order_by(FileItem.updated_at.desc()).limit(10)
    )).scalars().all()

    return ok({
        "quota": quota,
        "fileTypeDistribution": type_counts,
        "totalFiles": sum(type_counts.values()),
        "totalFolders": (await db.execute(
            select(func.count()).select_from(Folder).where(Folder.user_id == current_user.id)
        )).scalar() or 0,
        "recentActivity": [
            {
                "id": f.id,
                "filename": f.filename,
                "file_type": f.file_type,
                "file_size": f.file_size,
                "updated_at": str(f.updated_at) if f.updated_at else None,
            }
            for f in recent
        ],
    })
