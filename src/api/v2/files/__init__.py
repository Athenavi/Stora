"""
Stora 文件管理 API 聚合器
"""
from fastapi import APIRouter

from src.api.v2.files.items import router as items_router
from src.api.v2.files.upload import router as upload_router
from src.api.v2.files.download import router as download_router
from src.api.v2.files.search import router as search_router
from src.api.v2.files.preview import router as preview_router
from src.api.v2.files.versions import router as versions_router
from src.api.v2.files.offline import router as offline_router
from src.api.v2.share.share import router as share_router


def _build_router():
    router = APIRouter(tags=["files"])
    router.include_router(items_router)
    router.include_router(upload_router)
    router.include_router(download_router)
    router.include_router(search_router)
    router.include_router(preview_router)
    router.include_router(versions_router)
    router.include_router(offline_router)
    router.include_router(share_router)
    return router


router = _build_router()
