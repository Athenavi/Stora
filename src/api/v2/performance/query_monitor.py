"""
数据库查询监控API
提供查询分析、性能统计等调试功能
"""
from functools import wraps

from fastapi import APIRouter, Depends, HTTPException, Request
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User
from shared.services.performance.query_monitor import query_monitor_service
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import admin_required as admin_required_api
from src.extensions import get_async_db_session as get_async_db

router = APIRouter()


@router.get("/summary", summary="查询摘要", description="获取数据库查询的摘要统计(仅管理员)")
@_catch
async def query_summary_api(
        request: Request, db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(admin_required_api)
):
    """查询摘要API"""
    summary = query_monitor_service.get_summary()
    return ok(data=summary)


@router.get("/analysis", summary="查询分析", description="获取详细的查询分析报告(仅管理员)")
@_catch
async def query_analysis_api(
        request: Request, db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(admin_required_api)
):
    """查询分析API"""
    analysis = query_monitor_service.get_summary()
    return ok(data=analysis)
