"""
Stora Files API - 文件搜索路由
"""
from typing import Optional

from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_, or_
from sqlalchemy.orm import selectinload

from shared.models import FileItem, SearchHistory, User
from src.api.v2._helpers import ok
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["search"])


@router.get("")
async def search_files(
    q: str = Query(..., min_length=1, max_length=200),
    file_type: Optional[str] = Query(None),
    sort_by: str = Query("created_at"),
    sort_order: str = Query("desc"),
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """搜索文件（文件名模糊匹配）"""
    conditions = [
        FileItem.user_id == current_user.id,
        FileItem.deleted_at.is_(None),
        FileItem.filename.ilike(f"%{q}%"),
    ]
    if file_type:
        conditions.append(FileItem.file_type == file_type)
    
    sort_col = getattr(FileItem, sort_by, FileItem.created_at)
    order = sort_col.desc() if sort_order == "desc" else sort_col.asc()
    
    count_q = select(func.count()).select_from(FileItem).where(and_(*conditions))
    total = (await db.execute(count_q)).scalar() or 0
    
    qry = select(FileItem).where(and_(*conditions)).order_by(order)
    qry = qry.offset((page - 1) * page_size).limit(page_size)
    items = (await db.execute(qry)).scalars().all()
    
    # Save to search history
    history = SearchHistory(
        user_id=current_user.id,
        keyword=q,
        results_count=total,
    )
    db.add(history)
    await db.commit()
    
    return ok({
        "items": [_file_to_dict(f) for f in items],
        "total": total,
        "page": page,
        "page_size": page_size,
    })


@router.get("/history")
async def search_history(
    limit: int = Query(10, ge=1, le=50),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取搜索历史"""
    q = select(SearchHistory).where(
        SearchHistory.user_id == current_user.id
    ).order_by(SearchHistory.created_at.desc()).limit(limit)
    items = (await db.execute(q)).scalars().all()
    return ok([{
        "keyword": h.keyword,
        "results_count": h.results_count,
        "created_at": str(h.created_at) if h.created_at else None,
    } for h in items])


def _file_to_dict(f: FileItem) -> dict:
    return {
        "id": f.id,
        "filename": f.filename,
        "file_size": f.file_size,
        "mime_type": f.mime_type,
        "file_type": f.file_type,
        "folder_id": f.folder_id,
        "is_favorite": f.is_favorite,
        "thumbnail_url": f.thumbnail_url,
        "created_at": str(f.created_at) if f.created_at else None,
        "updated_at": str(f.updated_at) if f.updated_at else None,
    }
