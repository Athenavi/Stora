"""Stora 用户配额 API"""
from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import FileItem, StorageQuota, User
from src.api.v2._helpers import ok
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["quota"])


@router.get("/me/quota")
async def get_my_quota(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取当前用户存储配额"""
    from sqlalchemy import select
    quota = (await db.execute(
        select(StorageQuota).where(StorageQuota.user_id == current_user.id)
    )).scalar_one_or_none()

    if not quota:
        quota = StorageQuota(user_id=current_user.id)
        db.add(quota)
        await db.flush()

    return ok({
        "max_storage": quota.max_storage,
        "used_storage": quota.used_storage,
        "max_file_size": quota.max_file_size,
        "max_files_count": quota.max_files_count,
        "files_count": quota.files_count,
        "usage_percent": round(
            (quota.used_storage or 0) / max(quota.max_storage or 1, 1) * 100, 1
        ),
    })
