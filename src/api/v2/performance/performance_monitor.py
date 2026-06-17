"""
性能监控 API
提供系统性能指标的实时监控和报告
"""
from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Body

from shared.services.performance.performance_monitor import performance_monitor
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import jwt_required_dependency as jwt_required

router = APIRouter()


def _check_admin(user):
    is_admin = getattr(user, 'is_superuser', False) or getattr(user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="Permission denied")


@router.get("/dashboard", summary="性能仪表板", description="获取完整的性能监控仪表板数据")
@_catch
async def get_performance_dashboard(current_user=Depends(jwt_required)):
    """获取性能仪表板数据"""
    _check_admin(current_user)
    report = performance_monitor.get_comprehensive_report()
    return ok(data=report)


@router.get("/server-metrics", summary="服务器指标", description="获取服务器资源使用情况")
@_catch
async def get_server_metrics(current_user=Depends(jwt_required)):
    """获取服务器指标"""
    _check_admin(current_user)
    metrics = performance_monitor.get_server_metrics()
    return ok(data=metrics)


@router.get("/page-load-stats", summary="页面加载统计", description="获取页面加载性能统计")
@_catch
async def get_page_load_stats(
        hours: int = Query(24, ge=1, le=168, description="统计最近多少小时"),
        current_user=Depends(jwt_required),
):
    """获取页面加载统计"""
    _check_admin(current_user)
    stats = performance_monitor.get_page_load_stats(hours=hours)
    return ok(data=stats)


@router.get("/db-query-stats", summary="数据库查询统计", description="获取数据库查询性能统计")
@_catch
async def get_db_query_stats(
        hours: int = Query(24, ge=1, le=168, description="统计最近多少小时"),
        current_user=Depends(jwt_required),
):
    """获取数据库查询统计"""
    _check_admin(current_user)
    stats = performance_monitor.get_db_query_stats(hours=hours)
    return ok(data=stats)


@router.get("/api-stats", summary="API统计", description="获取API性能和错误统计")
@_catch
async def get_api_stats(
        hours: int = Query(24, ge=1, le=168, description="统计最近多少小时"),
        current_user=Depends(jwt_required),
):
    """获取API统计"""
    _check_admin(current_user)
    stats = performance_monitor.get_api_stats(hours=hours)
    return ok(data=stats)


@router.post("/record-page-load", summary="记录页面加载", description="记录页面加载时间")
@_catch
async def record_page_load(
        url: str = Body(..., description="页面URL"),
        load_time: float = Body(..., ge=0, description="加载时间(秒)"),
        user_agent: Optional[str] = Body(None, description="用户代理"),
):
    """记录页面加载时间"""
    performance_monitor.record_page_load(url, load_time, user_agent)
    return ok(data=None, message="Page load time recorded")


@router.post("/record-db-query", summary="记录数据库查询", description="记录数据库查询时间")
@_catch
async def record_db_query(
        query_type: str = Body(..., description="查询类型"),
        duration: float = Body(..., ge=0, description="查询耗时(秒)"),
        table: Optional[str] = Body(None, description="表名"),
        current_user=Depends(jwt_required),
):
    """记录数据库查询时间"""
    _check_admin(current_user)
    performance_monitor.record_db_query(query_type, duration, table)
    return ok(data=None, message="Database query recorded")


@router.post("/record-api-response", summary="记录API响应", description="记录API响应时间和状态")
@_catch
async def record_api_response(
        endpoint: str = Body(..., description="API端点"),
        method: str = Body(..., description="HTTP方法"),
        response_time: float = Body(..., ge=0, description="响应时间(秒)"),
        status_code: int = Body(..., description="状态码"),
        current_user=Depends(jwt_required),
):
    """记录API响应"""
    _check_admin(current_user)
    performance_monitor.record_api_response(endpoint, method, response_time, status_code)
    return ok(data=None, message="API response recorded")


@router.get("/recommendations", summary="优化建议", description="获取性能优化建议")
@_catch
async def get_recommendations(current_user=Depends(jwt_required)):
    """获取性能优化建议"""
    _check_admin(current_user)
    report = performance_monitor.get_comprehensive_report()
    return ok(data={'recommendations': report['recommendations'], 'generated_at': report['generated_at']})


@router.get("/health-check", summary="健康检查", description="快速健康检查")
@_catch
async def health_check():
    """健康检查"""
    metrics = performance_monitor.get_server_metrics()
    status = "healthy"
    issues = []
    if metrics['cpu']['percent'] > 90:
        status = "degraded"
        issues.append("High CPU usage")
    if metrics['memory']['percent'] > 90:
        status = "degraded"
        issues.append("High memory usage")
    if metrics['disk']['percent'] > 95:
        status = "critical"
        issues.append("Disk space critically low")
    return ok(data={
        'status': status, 'issues': issues,
        'uptime_seconds': metrics['uptime'], 'timestamp': metrics.get('timestamp'),
    })
