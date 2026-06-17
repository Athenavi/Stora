"""
HTTP/2 和 HTTP/3 配置 API
提供HTTP/2和HTTP/3的配置管理和优化建议
"""
from functools import wraps

from fastapi import APIRouter, Depends, HTTPException, Body

from shared.services.performance.http2_service import http2_service
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import jwt_required_dependency as jwt_required

router = APIRouter()


def _check_admin(user):
    is_admin = getattr(user, 'is_superuser', False) or getattr(user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="Permission denied")


@router.get("/config", summary="获取HTTP/2配置", description="获取当前HTTP/2和HTTP/3配置")
@_catch
async def get_http2_config(current_user=Depends(jwt_required)):
    """获取HTTP/2配置"""
    _check_admin(current_user)
    config = http2_service.get_configuration()
    return ok(data=config)


@router.post("/config", summary="更新HTTP/2配置", description="更新HTTP/2和HTTP/3配置")
@_catch
async def update_http2_config(
        http2_enabled: bool = Body(None, description="是否启用HTTP/2"),
        http3_enabled: bool = Body(None, description="是否启用HTTP/3"),
        server_push_enabled: bool = Body(None, description="是否启用服务器推送"),
        max_concurrent_streams: int = Body(None, ge=1, le=1000, description="最大并发流数"),
        current_user=Depends(jwt_required),
):
    """更新HTTP/2配置"""
    _check_admin(current_user)
    updates = {}
    if http2_enabled is not None:
        updates['http2_enabled'] = http2_enabled
    if http3_enabled is not None:
        updates['http3_enabled'] = http3_enabled
    if server_push_enabled is not None:
        updates['server_push_enabled'] = server_push_enabled
    if max_concurrent_streams is not None:
        updates['max_concurrent_streams'] = max_concurrent_streams
    result = http2_service.update_configuration(**updates)
    return ok(data=result, message="HTTP/2 configuration updated")


@router.get("/recommendations", summary="获取HTTP/2优化建议", description="获取HTTP/2和HTTP/3优化建议")
@_catch
async def get_http2_recommendations(current_user=Depends(jwt_required)):
    """获取HTTP/2优化建议"""
    _check_admin(current_user)
    recommendations = http2_service.get_recommendations()
    return ok(data=recommendations)
