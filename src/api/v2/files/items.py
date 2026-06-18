"""
Stora Files API - 文件与文件夹 CRUD 路由
"""
from typing import Optional, List

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, or_, and_, delete
from sqlalchemy.orm import selectinload

from shared.models import FileItem, FileFingerprint, Folder, StorageQuota, User
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["files"])


# ─── File CRUD ───

@router.get("")
async def list_files(
    folder_id: Optional[int] = Query(None, description="文件夹ID，null=根目录"),
    search: Optional[str] = Query(None),
    file_type: Optional[str] = Query(None),
    sort_by: str = Query("created_at"),
    sort_order: str = Query("desc"),
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出文件（分页、筛选、搜索）"""
    conditions = [FileItem.user_id == current_user.id, FileItem.deleted_at.is_(None)]
    
    if folder_id is None:
        conditions.append(FileItem.folder_id.is_(None))
    else:
        # Verify folder belongs to user
        folder = await db.get(Folder, folder_id)
        if not folder or folder.user_id != current_user.id:
            return fail("文件夹不存在")
        conditions.append(FileItem.folder_id == folder_id)
    
    if search:
        conditions.append(FileItem.filename.ilike(f"%{search}%"))
    if file_type:
        conditions.append(FileItem.file_type == file_type)
    
    # Sort
    sort_col = getattr(FileItem, sort_by, FileItem.created_at)
    order = sort_col.desc() if sort_order == "desc" else sort_col.asc()
    
    # Count + Query
    count_q = select(func.count()).select_from(FileItem).where(and_(*conditions))
    total = (await db.execute(count_q)).scalar() or 0
    
    q = select(FileItem).where(and_(*conditions)).order_by(order)
    q = q.offset((page - 1) * page_size).limit(page_size)
    items = (await db.execute(q)).scalars().all()
    
    return ok({
        "items": [_file_to_dict(f) for f in items],
        "total": total,
        "page": page,
        "page_size": page_size,
    })


@router.get("/{file_id}")
async def get_file(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取文件详情"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    return ok(_file_to_dict(file))


@router.patch("/{file_id}")
async def update_file(
    file_id: int,
    filename: Optional[str] = None,
    description: Optional[str] = None,
    is_favorite: Optional[bool] = None,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """更新文件信息（重命名、收藏等）"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    if filename:
        file.filename = filename
    if description is not None:
        file.description = description
    if is_favorite is not None:
        file.is_favorite = is_favorite
    await db.commit()
    await db.refresh(file)
    return ok(_file_to_dict(file))


@router.delete("/{file_id}")
async def delete_file(
    file_id: int,
    permanent: bool = Query(False),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """删除文件（移入回收站或永久删除）"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    
    if permanent:
        await db.delete(file)
    else:
        file.deleted_at = func.now()
    await db.commit()
    return ok(msg="删除成功")


# ─── Folder CRUD ───

@router.post("/folders")
async def create_folder(
    name: str,
    parent_id: Optional[int] = None,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """创建文件夹"""
    if parent_id:
        parent = await db.get(Folder, parent_id)
        if not parent or parent.user_id != current_user.id:
            return fail("父文件夹不存在")
    
    folder = Folder(
        user_id=current_user.id,
        parent_id=parent_id,
        name=name,
    )
    db.add(folder)
    await db.commit()
    await db.refresh(folder)
    return ok(_folder_to_dict(folder))


@router.patch("/folders/{folder_id}")
async def update_folder(
    folder_id: int,
    name: Optional[str] = None,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """更新文件夹"""
    folder = await db.get(Folder, folder_id)
    if not folder or folder.user_id != current_user.id:
        return fail("文件夹不存在")
    if name:
        folder.name = name
    await db.commit()
    await db.refresh(folder)
    return ok(_folder_to_dict(folder))


@router.delete("/folders/{folder_id}")
async def delete_folder(
    folder_id: int,
    recursive: bool = Query(False, description="是否递归删除子内容"),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """删除文件夹"""
    folder = await db.get(Folder, folder_id)
    if not folder or folder.user_id != current_user.id:
        return fail("文件夹不存在")
    
    # Check for children
    if not recursive:
        child_count = (await db.execute(
            select(func.count()).select_from(FileItem).where(
                FileItem.folder_id == folder_id, FileItem.deleted_at.is_(None)
            )
        )).scalar() or 0
        subfolder_count = (await db.execute(
            select(func.count()).select_from(Folder).where(Folder.parent_id == folder_id)
        )).scalar() or 0
        if child_count > 0 or subfolder_count > 0:
            return fail("文件夹非空，请使用 recursive=true")
    
    await db.delete(folder)
    await db.commit()
    return ok(msg="删除成功")


@router.get("/folders/{folder_id}/children")
async def list_folder_children(
    folder_id: int,
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出文件夹内子文件夹和文件"""
    folder = await db.get(Folder, folder_id)
    if not folder or folder.user_id != current_user.id:
        return fail("文件夹不存在")
    
    # Subfolders
    sub_q = select(Folder).where(
        Folder.parent_id == folder_id, Folder.user_id == current_user.id
    ).order_by(Folder.sort_order, Folder.name)
    folders = (await db.execute(sub_q)).scalars().all()
    
    # Files
    file_q = select(FileItem).where(
        FileItem.folder_id == folder_id,
        FileItem.user_id == current_user.id,
        FileItem.deleted_at.is_(None),
    ).order_by(FileItem.created_at.desc())
    files = (await db.execute(file_q)).scalars().all()
    
    return ok({
        "folders": [_folder_to_dict(f) for f in folders],
        "files": [_file_to_dict(f) for f in files],
        "path": await _get_folder_path(folder_id, db),
    })


@router.get("/folders/tree")
async def list_folder_tree(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出用户文件夹树（用于导航）"""
    folders = (await db.execute(
        select(Folder).where(Folder.user_id == current_user.id).order_by(Folder.name)
    )).scalars().all()
    
    def build_tree(parent_id=None):
        children = [f for f in folders if f.parent_id == parent_id]
        return [_folder_to_dict(f) | {"children": build_tree(f.id)} for f in children]
    
    return ok(build_tree())


# ─── Move Operations ───

@router.post("/move")
async def move_files(
    file_ids: List[int],
    target_folder_id: Optional[int] = None,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """批量移动文件到目标文件夹"""
    if target_folder_id:
        target = await db.get(Folder, target_folder_id)
        if not target or target.user_id != current_user.id:
            return fail("目标文件夹不存在")
    
    result = await db.execute(
        select(FileItem).where(
            FileItem.id.in_(file_ids),
            FileItem.user_id == current_user.id,
        )
    )
    files = result.scalars().all()
    for f in files:
        f.folder_id = target_folder_id
    await db.commit()
    return ok(msg=f"已移动 {len(files)} 个文件")


# ─── Trash / Recycle Bin ───

@router.get("/trash")
async def list_trash(
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出回收站中的文件"""
    conditions = [
        FileItem.user_id == current_user.id,
        FileItem.deleted_at.isnot(None),
    ]
    count_q = select(func.count()).select_from(FileItem).where(and_(*conditions))
    total = (await db.execute(count_q)).scalar() or 0

    q = select(FileItem).where(and_(*conditions)).order_by(FileItem.deleted_at.desc())
    q = q.offset((page - 1) * page_size).limit(page_size)
    items = (await db.execute(q)).scalars().all()

    return ok({
        "items": [_file_to_dict(f) for f in items],
        "total": total,
        "page": page,
        "page_size": page_size,
    })


@router.post("/trash/{file_id}/restore")
async def restore_file(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """从回收站恢复文件"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    file.deleted_at = None
    await db.commit()
    return ok(msg="已恢复")


@router.post("/trash/batch-restore")
async def batch_restore(
    file_ids: List[int],
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """批量恢复文件"""
    result = await db.execute(
        select(FileItem).where(
            FileItem.id.in_(file_ids),
            FileItem.user_id == current_user.id,
            FileItem.deleted_at.isnot(None),
        )
    )
    files = result.scalars().all()
    for f in files:
        f.deleted_at = None
    await db.commit()
    return ok(msg=f"已恢复 {len(files)} 个文件")


@router.delete("/trash/{file_id}/destroy")
async def destroy_file(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """永久删除文件（物理删除）"""
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")

    # Decrement fingerprint ref count
    if file.file_hash:
        fp = (await db.execute(
            select(FileFingerprint).where(FileFingerprint.hash == file.file_hash)
        )).scalar_one_or_none()
        if fp:
            fp.reference_count -= 1
            if fp.reference_count <= 0:
                import os
                if fp.storage_path and os.path.exists(fp.storage_path):
                    os.remove(fp.storage_path)
                await db.delete(fp)

    # Update quota
    from shared.models import StorageQuota
    quota = (await db.execute(
        select(StorageQuota).where(StorageQuota.user_id == current_user.id)
    )).scalar_one_or_none()
    if quota:
        quota.used_storage = max(0, (quota.used_storage or 0) - (file.file_size or 0))
        quota.files_count = max(0, (quota.files_count or 0) - 1)

    await db.delete(file)
    await db.commit()
    return ok(msg="已永久删除")


@router.post("/trash/clear")
async def clear_trash(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """清空回收站"""
    result = await db.execute(
        select(FileItem).where(
            FileItem.user_id == current_user.id,
            FileItem.deleted_at.isnot(None),
        )
    )
    files = result.scalars().all()
    for f in files:
        await db.delete(f)
    await db.commit()
    return ok(msg=f"已清空 {len(files)} 个文件")


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
        "created_at": str(f.created_at) if f.created_at else None,
        "updated_at": str(f.updated_at) if f.updated_at else None,
    }


def _folder_to_dict(f: Folder) -> dict:
    return {
        "id": f.id,
        "name": f.name,
        "parent_id": f.parent_id,
        "color": f.color,
        "icon": f.icon,
        "is_shared": f.is_shared,
        "sort_order": f.sort_order,
        "created_at": str(f.created_at) if f.created_at else None,
    }


async def _get_folder_path(folder_id: int, db: AsyncSession) -> list:
    """Build folder breadcrumb path"""
    path = []
    current_id = folder_id
    while current_id:
        folder = await db.get(Folder, current_id)
        if not folder:
            break
        path.append({"id": folder.id, "name": folder.name})
        current_id = folder.parent_id
    path.reverse()
    return path
