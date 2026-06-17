"""
Stora 文件管理 API 聚合路由器
提供文件/文件夹的 CRUD、上传、下载、分享、搜索、预览接口
"""
from fastapi import APIRouter


def _build_router():
    router = APIRouter(tags=["files"])

    # 文件与文件夹 CRUD
    from src.api.v2.media.offline_download import router as offline_router
    router.include_router(offline_router)

    # 图片编辑
    from src.api.v2.media.image_edit import router as image_edit_router
    router.include_router(image_edit_router, prefix="/edit")

    return router


router = _build_router()
