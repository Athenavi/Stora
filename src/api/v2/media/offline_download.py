"""
VIP 离线下载 API 路由
提供 VIP 会员的离线下载管理接口
"""
from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import User
from shared.services.media.offline_download_service import OfflineDownloadService
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/offline-download", tags=["offline-download"])


def _catch(func):
    """统一错误捕获装饰器"""
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            return fail(str(e))
    return wrapper


@router.get("/limits", summary="获取离线下载限制信息")
@_catch
async def get_offline_download_limits(
    current_user: User = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """获取当前用户的 VIP 离线下载配额与限制"""
    service = OfflineDownloadService(db, current_user)
    limits = await service.get_user_limits()
    return ok(data=limits)


@router.post("/tasks", summary="创建离线下载任务")
@_catch
async def create_offline_download_task(
    body: dict,
    current_user: User = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """
    创建 VIP 离线下载任务（需要 VIP 会员权限）
    
    - 基础 VIP：最多 2 个并发，单文件 ≤50MB，最多 5 个待处理
    - 高级 VIP：最多 5 个并发，单文件 ≤200MB，最多 20 个待处理
    - Pro VIP：最多 10 个并发，单文件 ≤500MB，最多 50 个待处理
    """
    url = body.get("url")
    resource_type = body.get("resource_type", "image")
    filename = body.get("filename")

    if not url:
        return fail("url 是必填项")

    service = OfflineDownloadService(db, current_user)
    task, error = await service.create_download_task(
        source_url=url,
        resource_type=resource_type,
        filename=filename,
    )
    if error:
        return fail(error)

    return ok(data={
        "task_id": task.id,
        "status": task.status,
        "message": "离线下载任务已创建",
    })


@router.get("/tasks", summary="获取离线下载任务列表")
@_catch
async def list_offline_download_tasks(
    status: Optional[str] = Query(None, description="状态筛选: pending/downloading/completed/failed/cancelled"),
    page: int = Query(1, ge=1, description="页码"),
    per_page: int = Query(20, ge=1, le=100, description="每页数量"),
    current_user: User = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """获取当前用户的离线下载任务列表"""
    service = OfflineDownloadService(db, current_user)
    result = await service.get_user_tasks(status=status, page=page, per_page=per_page)
    return ok(data=result)


@router.get("/tasks/{task_id}", summary="获取任务详情")
@_catch
async def get_offline_download_task_detail(
    task_id: int,
    current_user: User = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """获取单个离线下载任务的详细信息"""
    service = OfflineDownloadService(db, current_user)
    task_data, error = await service.get_task_detail(task_id)
    if error:
        raise HTTPException(status_code=404, detail=error)
    return ok(data=task_data)


@router.post("/tasks/{task_id}/cancel", summary="取消任务")
@_catch
async def cancel_offline_download_task(
    task_id: int,
    current_user: User = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """取消正在进行的离线下载任务"""
    service = OfflineDownloadService(db, current_user)
    success, error = await service.cancel_task(task_id)
    if not success:
        raise HTTPException(status_code=400, detail=error or "取消失败")
    return ok(msg="任务已取消")


@router.post("/tasks/{task_id}/retry", summary="重试任务")
@_catch
async def retry_offline_download_task(
    task_id: int,
    current_user: User = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """重试失败的离线下载任务"""
    service = OfflineDownloadService(db, current_user)
    success, error = await service.retry_task(task_id)
    if not success:
        raise HTTPException(status_code=400, detail=error or "重试失败")
    return ok(msg="任务已重新加入队列")
