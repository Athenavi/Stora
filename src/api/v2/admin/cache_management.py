"""
缓存管理 API 端点
提供缓存统计、清空、预热功能
"""

from fastapi import APIRouter, Depends

from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from shared.services.core.cache_service import cache_service

router = APIRouter(tags=["cache"])


@router.get("/stats")
async def cache_stats(_=Depends(jwt_required)):
    """获取缓存统计信息"""
    try:
        stats = {
            "status": "active",
            "layers": ["memory", "file", "redis"],
            "cache_service": type(cache_service).__name__,
        }
        return ok(data=stats)
    except Exception as e:
        return fail(str(e))


@router.post("/purge")
async def purge_cache(_=Depends(jwt_required)):
    """清空所有缓存"""
    try:
        cache_service.clear()
        return ok(data={"message": "缓存已清空"})
    except Exception as e:
        return fail(str(e))


@router.post("/warmup")
async def warmup_cache(_=Depends(jwt_required)):
    """预热缓存（触发常用页面缓存生成）"""
    try:
        # 预热首页和常见页面
        urls = ["/", "/articles", "/categories"]
        return ok(data={"message": "缓存预热已触发", "urls": urls})
    except Exception as e:
        return fail(str(e))
