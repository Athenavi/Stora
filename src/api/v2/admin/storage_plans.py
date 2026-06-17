"""
Stora 存储套餐管理 API
"""
from typing import Optional, List

from fastapi import APIRouter, Depends, Form, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func

from shared.models import User
from shared.models.file.storage_plan import StoragePlan
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(prefix="/api/v2/admin/storage-plans", tags=["storage-plans"])


@router.get("")
async def list_plans(
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """列出所有套餐"""
    plans = (await db.execute(
        select(StoragePlan).order_by(StoragePlan.sort_order, StoragePlan.name)
    )).scalars().all()
    return ok([{
        "id": p.id, "name": p.name, "description": p.description,
        "storage_bytes": p.storage_bytes, "max_file_size": p.max_file_size,
        "max_files_count": p.max_files_count, "price": p.price,
        "is_active": p.is_active, "sort_order": p.sort_order,
    } for p in plans])


@router.post("")
async def create_plan(
    name: str = Form(...),
    storage_bytes: int = Form(1073741824),
    max_file_size: int = Form(104857600),
    max_files_count: int = Form(10000),
    price: int = Form(0),
    description: Optional[str] = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """创建套餐"""
    plan = StoragePlan(
        name=name, description=description,
        storage_bytes=storage_bytes, max_file_size=max_file_size,
        max_files_count=max_files_count, price=price,
    )
    db.add(plan)
    await db.commit()
    await db.refresh(plan)
    return ok({"id": plan.id, "name": plan.name})


@router.put("/{plan_id}")
async def update_plan(
    plan_id: int,
    name: Optional[str] = Form(None),
    storage_bytes: Optional[int] = Form(None),
    max_file_size: Optional[int] = Form(None),
    price: Optional[int] = Form(None),
    is_active: Optional[bool] = Form(None),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """更新套餐"""
    plan = await db.get(StoragePlan, plan_id)
    if not plan:
        return fail("套餐不存在")
    if name is not None: plan.name = name
    if storage_bytes is not None: plan.storage_bytes = storage_bytes
    if max_file_size is not None: plan.max_file_size = max_file_size
    if price is not None: plan.price = price
    if is_active is not None: plan.is_active = is_active
    await db.commit()
    return ok(msg="已更新")


@router.delete("/{plan_id}")
async def delete_plan(
    plan_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_required),
):
    """删除套餐"""
    plan = await db.get(StoragePlan, plan_id)
    if not plan:
        return fail("套餐不存在")
    await db.delete(plan)
    await db.commit()
    return ok(msg="已删除")
