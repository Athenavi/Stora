"""
迁移系统管理 API

提供迁移任务(MigrationTask)和迁移日志(MigrationLog)的 CRUD 管理接口
"""
import json
from datetime import datetime
from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import MigrationTask, MigrationLog
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["migration-management"])


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            return fail(str(e))
    return wrapper


# ==================== 迁移任务管理 ====================


@router.get("/tasks")
@_catch
async def list_tasks(
    page: int = Query(1, ge=1, description="页码"),
    per_page: int = Query(20, ge=1, le=100, description="每页数量"),
    search: Optional[str] = Query(None, description="搜索任务名称"),
    status: Optional[str] = Query(None, description="任务状态"),
    source_platform: Optional[str] = Query(None, description="源平台"),
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """
    获取迁移任务列表

    支持分页、搜索、按状态和源平台筛选
    """
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    query = select(MigrationTask)

    if search:
        query = query.where(MigrationTask.task_name.ilike(f"%{search}%"))

    if status:
        query = query.where(MigrationTask.status == status)

    if source_platform:
        query = query.where(MigrationTask.source_platform == source_platform)

    query = query.order_by(MigrationTask.created_at.desc())

    count_query = select(func.count()).select_from(query.subquery())
    total_result = await db.execute(count_query)
    total = total_result.scalar()

    offset = (page - 1) * per_page
    query = query.offset(offset).limit(per_page)

    result = await db.execute(query)
    tasks = result.scalars().all()

    return ok(data={
        "tasks": [t.to_dict() for t in tasks],
        "pagination": {
            "page": page,
            "per_page": per_page,
            "total": total,
            "total_pages": (total + per_page - 1) // per_page if total > 0 else 0,
        },
    })


@router.get("/tasks/{task_id}")
@_catch
async def get_task(
    task_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """获取迁移任务详情"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    query = select(MigrationTask).where(MigrationTask.id == task_id)
    result = await db.execute(query)
    task = result.scalar_one_or_none()

    if not task:
        return fail("迁移任务不存在")

    return ok(data=task.to_dict())


@router.post("/tasks")
@_catch
async def create_task(
    request: Request,
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """创建迁移任务"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    data = await request.json()

    task_name = data.get("task_name")
    source_platform = data.get("source_platform")
    if not task_name or not source_platform:
        return fail("task_name 和 source_platform 为必填字段")

    config = data.get("config")
    if isinstance(config, dict):
        config = json.dumps(config, ensure_ascii=False)

    now = datetime.utcnow()
    task = MigrationTask(
        task_name=task_name,
        source_platform=source_platform,
        status=data.get("status", "pending"),
        config=config,
        progress=0,
        total_items=data.get("total_items", 0),
        migrated_items=0,
        created_by=current_user.id,
        created_at=now,
        updated_at=now,
    )

    db.add(task)
    await db.commit()
    await db.refresh(task)

    return ok(data=task.to_dict(), msg="迁移任务创建成功")


@router.put("/tasks/{task_id}")
@_catch
async def update_task(
    task_id: int,
    request: Request,
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """更新迁移任务"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    query = select(MigrationTask).where(MigrationTask.id == task_id)
    result = await db.execute(query)
    task = result.scalar_one_or_none()

    if not task:
        return fail("迁移任务不存在")

    data = await request.json()

    updatable_fields = [
        "task_name", "source_platform", "status", "progress",
        "total_items", "migrated_items", "error_message",
    ]
    for field in updatable_fields:
        if field in data:
            setattr(task, field, data[field])

    if "config" in data:
        config = data["config"]
        task.config = json.dumps(config, ensure_ascii=False) if isinstance(config, dict) else config

    if "started_at" in data:
        sa = data["started_at"]
        task.started_at = datetime.fromisoformat(sa.replace("Z", "+00:00")) if isinstance(sa, str) else sa

    if "completed_at" in data:
        ca = data["completed_at"]
        task.completed_at = datetime.fromisoformat(ca.replace("Z", "+00:00")) if isinstance(ca, str) else ca

    # 自动设置开始/完成时间
    if "status" in data:
        if data["status"] == "running" and task.started_at is None:
            task.started_at = datetime.utcnow()
        elif data["status"] in ("completed", "failed", "cancelled"):
            task.completed_at = datetime.utcnow()

    task.updated_at = datetime.utcnow()

    await db.commit()
    await db.refresh(task)

    return ok(data=task.to_dict(), msg="迁移任务更新成功")


@router.delete("/tasks/{task_id}")
@_catch
async def delete_task(
    task_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """删除迁移任务"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    query = select(MigrationTask).where(MigrationTask.id == task_id)
    result = await db.execute(query)
    task = result.scalar_one_or_none()

    if not task:
        return fail("迁移任务不存在")

    # 正在运行的任务不允许删除
    if task.status == "running":
        return fail("运行中的任务不允许删除，请先取消任务")

    # 级联删除关联日志
    await db.execute(
        MigrationLog.__table__.delete().where(MigrationLog.task_id == task_id)
    )

    await db.delete(task)
    await db.commit()

    return ok(msg="迁移任务及其日志删除成功")


# ==================== 迁移日志管理 ====================


@router.get("/logs")
@_catch
async def list_logs(
    page: int = Query(1, ge=1, description="页码"),
    per_page: int = Query(50, ge=1, le=200, description="每页数量"),
    task_id: Optional[int] = Query(None, description="关联的任务ID"),
    log_level: Optional[str] = Query(None, description="日志级别"),
    item_type: Optional[str] = Query(None, description="项目类型"),
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """获取迁移日志列表"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    query = select(MigrationLog)

    if task_id:
        query = query.where(MigrationLog.task_id == task_id)

    if log_level:
        query = query.where(MigrationLog.log_level == log_level)

    if item_type:
        query = query.where(MigrationLog.item_type == item_type)

    query = query.order_by(MigrationLog.created_at.desc())

    count_query = select(func.count()).select_from(query.subquery())
    total_result = await db.execute(count_query)
    total = total_result.scalar()

    offset = (page - 1) * per_page
    query = query.offset(offset).limit(per_page)

    result = await db.execute(query)
    logs = result.scalars().all()

    return ok(data={
        "logs": [l.to_dict() for l in logs],
        "pagination": {
            "page": page,
            "per_page": per_page,
            "total": total,
            "total_pages": (total + per_page - 1) // per_page if total > 0 else 0,
        },
    })


@router.post("/logs")
@_catch
async def create_log(
    request: Request,
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """创建迁移日志"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    data = await request.json()

    task_id = data.get("task_id")
    message = data.get("message")
    if not task_id or not message:
        return fail("task_id 和 message 为必填字段")

    # 验证任务存在性
    task_query = select(MigrationTask).where(MigrationTask.id == task_id)
    task_result = await db.execute(task_query)
    if not task_result.scalar_one_or_none():
        return fail(f"迁移任务 ID={task_id} 不存在")

    now = datetime.utcnow()
    log = MigrationLog(
        task_id=task_id,
        log_level=data.get("log_level", "info"),
        message=message,
        item_type=data.get("item_type"),
        item_id=data.get("item_id"),
        created_at=now,
    )

    db.add(log)
    await db.commit()
    await db.refresh(log)

    return ok(data=log.to_dict(), msg="迁移日志创建成功")


@router.delete("/logs/{log_id}")
@_catch
async def delete_log(
    log_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user=Depends(jwt_required),
):
    """删除迁移日志"""
    is_admin = getattr(current_user, 'is_superuser', False) or getattr(current_user, 'is_staff', False)
    if not is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")

    query = select(MigrationLog).where(MigrationLog.id == log_id)
    result = await db.execute(query)
    log = result.scalar_one_or_none()

    if not log:
        return fail("迁移日志不存在")

    await db.delete(log)
    await db.commit()

    return ok(msg="迁移日志删除成功")
