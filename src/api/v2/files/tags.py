"""
Stora Tags API — file tag CRUD and assignment management
"""
from typing import Optional, List

from fastapi import APIRouter, Depends, Form, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, delete

from shared.models import User
from shared.models.file.file_tag import FileTag
from shared.models.file.file_tag_assignment import FileTagAssignment
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["tags"])


# ─── Tag CRUD ───

@router.get("")
async def list_tags(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出当前用户的所有标签（含文件数）"""
    rows = await db.execute(
        select(FileTag, func.count(FileTagAssignment.id).label("file_count"))
        .outerjoin(FileTagAssignment, FileTagAssignment.tag_id == FileTag.id)
        .where(FileTag.user_id == current_user.id)
        .group_by(FileTag.id)
        .order_by(FileTag.name)
    )
    items = []
    for tag, count in rows:
        d = tag.to_dict()
        d["file_count"] = count
        items.append(d)
    return ok(items)


@router.post("")
async def create_tag(
    name: str = Form(...),
    color: str = Form("#6366f1"),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """创建标签"""
    # Check duplicate
    existing = (await db.execute(
        select(FileTag).where(
            FileTag.user_id == current_user.id,
            FileTag.name == name,
        )
    )).scalar_one_or_none()
    if existing:
        return fail("标签已存在")

    tag = FileTag(user_id=current_user.id, name=name, color=color)
    db.add(tag)
    await db.commit()
    await db.refresh(tag)
    return ok(tag.to_dict())


@router.patch("/{tag_id}")
async def update_tag(
    tag_id: int,
    name: Optional[str] = Form(None),
    color: Optional[str] = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """更新标签"""
    tag = await db.get(FileTag, tag_id)
    if not tag or tag.user_id != current_user.id:
        return fail("标签不存在")
    if name:
        tag.name = name
    if color:
        tag.color = color
    await db.commit()
    return ok(tag.to_dict())


@router.delete("/{tag_id}")
async def delete_tag(
    tag_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """删除标签"""
    tag = await db.get(FileTag, tag_id)
    if not tag or tag.user_id != current_user.id:
        return fail("标签不存在")
    # Remove all assignments
    await db.execute(delete(FileTagAssignment).where(FileTagAssignment.tag_id == tag_id))
    await db.delete(tag)
    await db.commit()
    return ok(msg="标签已删除")


# ─── Tag Assignment ───


@router.get("/file/{file_id}")
async def get_file_tags(
    file_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """获取文件的标签列表"""
    from shared.models.file.file_item import FileItem
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    rows = (await db.execute(
        select(FileTag)
        .join(FileTagAssignment, FileTagAssignment.tag_id == FileTag.id)
        .where(FileTagAssignment.file_id == file_id)
    )).scalars().all()
    return ok([t.to_dict() for t in rows])


@router.post("/file/{file_id}")
async def assign_tag(
    file_id: int,
    tag_id: int = Form(...),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """为文件添加标签"""
    from shared.models.file.file_item import FileItem
    file = await db.get(FileItem, file_id)
    if not file or file.user_id != current_user.id:
        return fail("文件不存在")
    tag = await db.get(FileTag, tag_id)
    if not tag or tag.user_id != current_user.id:
        return fail("标签不存在")
    # Check duplicate
    existing = (await db.execute(
        select(FileTagAssignment).where(
            FileTagAssignment.file_id == file_id,
            FileTagAssignment.tag_id == tag_id,
        )
    )).scalar_one_or_none()
    if existing:
        return fail("已添加该标签")
    assignment = FileTagAssignment(file_id=file_id, tag_id=tag_id)
    db.add(assignment)
    await db.commit()
    return ok(msg="标签已添加")


@router.delete("/file/{file_id}/{tag_id}")
async def remove_tag(
    file_id: int,
    tag_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """移除文件的标签"""
    await db.execute(
        delete(FileTagAssignment).where(
            FileTagAssignment.file_id == file_id,
            FileTagAssignment.tag_id == tag_id,
        )
    )
    await db.commit()
    return ok(msg="标签已移除")
