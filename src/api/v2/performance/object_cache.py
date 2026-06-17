"""
对象缓存管理 API
提供对象缓存的管理、监控和清除功能
"""
from functools import wraps
from typing import List

from fastapi import APIRouter, Depends, HTTPException, Body

from shared.services.core.object_cache import object_cache_service
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import jwt_required_dependency as jwt_required

router = APIRouter()


def _check_admin(user):
    is_admin = getattr(user, 'is_superuser', False) or getattr(user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="Permission denied")


@router.get("/stats", summary="获取对象缓存统计", description="获取对象缓存的统计信息")
@_catch
async def get_object_cache_stats(current_user=Depends(jwt_required)):
    """获取对象缓存统计"""
    _check_admin(current_user)
    stats = await object_cache_service.get_stats()
    return ok(data=stats)


@router.post("/invalidate-tag", summary="按标签清除缓存", description="使指定标签的所有缓存失效")
@_catch
async def invalidate_by_tag(
        tag: str = Body(..., description="缓存标签"),
        current_user=Depends(jwt_required),
):
    """按标签清除缓存"""
    _check_admin(current_user)
    count = await object_cache_service.invalidate_by_tag(tag)
    return ok(data={"count": count}, message=f"Invalidated {count} cached objects with tag: {tag}")


@router.post("/invalidate-tags", summary="批量按标签清除", description="使多个标签的缓存失效")
@_catch
async def invalidate_by_tags(
        tags: List[str] = Body(..., description="缓存标签列表"),
        current_user=Depends(jwt_required),
):
    """批量按标签清除缓存"""
    _check_admin(current_user)
    count = await object_cache_service.invalidate_by_tags(tags)
    return ok(data={"count": count, "tags": tags}, message=f"Invalidated {count} cached objects")


@router.post("/config", summary="更新缓存配置", description="更新对象缓存配置")
@_catch
async def update_cache_config(
        ttl: int = Body(None, ge=1, le=86400, description="默认TTL(秒)"),
        current_user=Depends(jwt_required),
):
    """更新对象缓存配置"""
    _check_admin(current_user)
    if ttl is not None:
        object_cache_service.default_ttl = ttl
    return ok(data={"default_ttl": object_cache_service.default_ttl}, message="Cache configuration updated")
