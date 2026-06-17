"""
图片编辑 API — 裁剪、旋转、滤镜
"""

import os
from datetime import datetime
from fastapi import APIRouter, Depends, HTTPException, UploadFile, File, Form
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models.media import Media
from shared.services.media.image_tool.image_editor import ImageEditor
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["image-edit"])
editor = ImageEditor()


@router.get("/{media_id}/info")
async def get_image_info(
    media_id: int,
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """获取图片元数据信息"""
    try:
        result = await db.execute(select(Media).where(Media.id == media_id))
        media = result.scalar_one_or_none()
        if not media:
            return fail("媒体不存在")

        file_path = media.file_path or media.url or ""
        if not file_path or not os.path.exists(file_path):
            return fail("图片文件不存在")

        info = editor.get_image_info(file_path)
        return ok(data={"media_id": media_id, "filename": media.filename or media.title, "info": info})
    except Exception as e:
        return fail(str(e))


@router.post("/{media_id}/crop")
async def crop_image(
    media_id: int,
    x: int = Form(...), y: int = Form(...),
    width: int = Form(...), height: int = Form(...),
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """裁剪图片"""
    try:
        result = await db.execute(select(Media).where(Media.id == media_id))
        media = result.scalar_one_or_none()
        if not media or not media.file_path:
            return fail("媒体不存在")

        editor.process_image(media.file_path, [{"type": "crop", "x": x, "y": y, "width": width, "height": height}])
        return ok(data={"message": "裁剪成功"})
    except Exception as e:
        return fail(str(e))


@router.post("/{media_id}/rotate")
async def rotate_image(
    media_id: int,
    degrees: float = Form(...),
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """旋转图片"""
    try:
        result = await db.execute(select(Media).where(Media.id == media_id))
        media = result.scalar_one_or_none()
        if not media or not media.file_path:
            return fail("媒体不存在")

        editor.process_image(media.file_path, [{"type": "rotate", "degrees": degrees}])
        return ok(data={"message": f"旋转 {degrees}° 成功"})
    except Exception as e:
        return fail(str(e))


@router.post("/{media_id}/filter")
async def filter_image(
    media_id: int,
    filter_type: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """应用图片滤镜"""
    try:
        result = await db.execute(select(Media).where(Media.id == media_id))
        media = result.scalar_one_or_none()
        if not media or not media.file_path:
            return fail("媒体不存在")

        editor.process_image(media.file_path, [{"type": "filter", "filter": filter_type}])
        return ok(data={"message": f"滤镜 {filter_type} 已应用"})
    except Exception as e:
        return fail(str(e))


@router.post("/{media_id}/grayscale")
async def grayscale_image(
    media_id: int,
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """转为灰度图"""
    try:
        result = await db.execute(select(Media).where(Media.id == media_id))
        media = result.scalar_one_or_none()
        if not media or not media.file_path:
            return fail("媒体不存在")

        editor.process_image(media.file_path, [{"type": "grayscale"}])
        return ok(data={"message": "已转为灰度图"})
    except Exception as e:
        return fail(str(e))
