"""
负载均衡管理 API
提供多实例管理、健康检查、会话管理和故障转移功能
"""
from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Body

from shared.services.performance.load_balancer import load_balancer_service
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import jwt_required_dependency as jwt_required

router = APIRouter(tags=["load-balancer"])


@router.get("/instances", summary="获取活动实例列表")
@_catch
async def get_active_instances(current_user=Depends(jwt_required)):
    """获取所有活动实例"""
    instances = await load_balancer_service.get_active_instances()
    return ok(data={'instances': instances, 'total': len(instances)})


@router.get("/instance/{instance_id}/health", summary="检查实例健康状态")
@_catch
async def check_instance_health(instance_id: str, current_user=Depends(jwt_required)):
    """检查指定实例的健康状态"""
    health = await load_balancer_service.check_instance_health(instance_id)
    return ok(data=health)


@router.post("/instance/{instance_id}/drain", summary="排空实例连接")
@_catch
async def drain_instance(instance_id: str, current_user=Depends(jwt_required)):
    """排空指定实例的连接"""
    success = await load_balancer_service.drain_instance(instance_id)
    if success:
        return ok(data=None, message="Instance drained successfully")
    return fail("Failed to drain instance")


@router.post("/instance/{instance_id}/restore", summary="恢复实例服务")
@_catch
async def restore_instance(instance_id: str, current_user=Depends(jwt_required)):
    """恢复指定实例的服务"""
    success = await load_balancer_service.restore_instance(instance_id)
    if success:
        return ok(data=None, message="Instance restored successfully")
    return fail("Failed to restore instance")


@router.get("/stats", summary="获取负载均衡统计")
@_catch
async def get_load_balancer_stats(current_user=Depends(jwt_required)):
    """获取负载均衡统计"""
    stats = await load_balancer_service.get_statistics()
    return ok(data=stats)


@router.post("/failover/manual", summary="手动触发故障转移")
@_catch
async def manual_failover(
        from_instance: str = Body(..., description="源实例ID"),
        to_instance: str = Body(..., description="目标实例ID"),
        current_user=Depends(jwt_required)
):
    """手动触发故障转移"""
    success = await load_balancer_service.manual_failover(from_instance, to_instance)
    if success:
        return ok(data=None, message="Failover completed successfully")
    return fail("Failover failed")


@router.get("/sessions/{instance_id}", summary="获取实例会话信息")
@_catch
async def get_instance_sessions(instance_id: str, current_user=Depends(jwt_required)):
    """获取指定实例的会话信息"""
    sessions = await load_balancer_service.get_instance_sessions(instance_id)
    return ok(data={'instance_id': instance_id, 'sessions': sessions, 'total': len(sessions)})
