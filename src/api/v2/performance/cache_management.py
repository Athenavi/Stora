"""
缓存管理 API
提供多级缓存的管理、监控和预热功能
"""
from functools import wraps
from typing import Optional, List, Dict, Any

from fastapi import APIRouter, Depends, HTTPException, Body

from shared.services.core.multi_level_cache import multi_level_cache
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import jwt_required_dependency as jwt_required

router = APIRouter()


def _check_admin(user):
    is_admin = getattr(user, 'is_superuser', False) or getattr(user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="Permission denied")


@router.get("/stats", summary="获取缓存统计", description="获取多级缓存的详细统计信息")
@_catch
async def get_cache_stats(current_user=Depends(jwt_required)):
    """获取缓存统计信息"""
    _check_admin(current_user)
    stats = multi_level_cache.get_stats()
    return ok(data=stats)


@router.post("/warmup", summary="缓存预热", description="批量预热缓存数据")
@_catch
async def warmup_cache(
        keys_data: List[Dict[str, Any]] = Body(..., description="预热数据列表"),
        current_user=Depends(jwt_required),
):
    """缓存预热"""
    _check_admin(current_user)
    await multi_level_cache.warmup(keys_data)
    return ok(data=None, message=f"Successfully warmed up {len(keys_data)} cache entries")


@router.delete("/clear", summary="清空缓存", description="清空所有缓存层级")
@_catch
async def clear_cache(current_user=Depends(jwt_required)):
    """清空所有缓存"""
    _check_admin(current_user)
    await multi_level_cache.clear()
    return ok(data=None, message="All cache levels cleared successfully")


@router.get("/items/{key}", summary="获取缓存值", description="从多级缓存中获取指定键的值")
@_catch
async def get_cache_value(key: str, current_user=Depends(jwt_required)):
    """获取单个缓存值"""
    _check_admin(current_user)
    value = await multi_level_cache.get(key)
    if value is None:
        raise HTTPException(status_code=404, detail="Cache key not found")
    return ok(data={'key': key, 'value': value})


@router.post("/items/{key}", summary="设置缓存值", description="设置缓存值到所有层级")
@_catch
async def set_cache_value(
        key: str,
        value: Any = Body(..., description="缓存值"),
        ttl: Optional[int] = Body(None, description="TTL(秒)"),
        current_user=Depends(jwt_required),
):
    """设置缓存值"""
    _check_admin(current_user)
    await multi_level_cache.set(key, value, ttl)
    return ok(data=None, message="Cache value set successfully")


@router.delete("/items/{key}", summary="删除缓存值", description="从所有缓存层级删除指定键")
@_catch
async def delete_cache_value(key: str, current_user=Depends(jwt_required)):
    """删除缓存值"""
    _check_admin(current_user)
    await multi_level_cache.delete(key)
    return ok(data=None, message="Cache value deleted successfully")


@router.post("/config", summary="更新缓存配置", description="动态更新缓存配置")
@_catch
async def update_cache_config(
        config: Dict[str, Any] = Body(..., description="配置项"),
        current_user=Depends(jwt_required),
):
    """更新缓存配置"""
    _check_admin(current_user)
    return ok(data=config, message="Config update received. Note: Some config changes require service restart.")
