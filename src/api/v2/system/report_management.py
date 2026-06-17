"""
报表管理 API
提供自定义报表生成和查询功能
"""

from functools import wraps
from typing import List, Optional

from fastapi import APIRouter, Depends, Query, Body, HTTPException

from shared.services.analytics.scheduled_report_service import create_scheduled_report_service
from shared.services.system.report_generator import report_generator, ReportGenerator
from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import admin_required as admin_required_api
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["reports"])


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


@router.get("/content", summary="获取内容表现报表")
@_catch
async def get_content_report(
        days: int = Query(30, ge=7, le=90, description="统计天数"),
        group_by: str = Query('day', enum=['day', 'week', 'month'], description="分组方式"),
        current_user=Depends(admin_required_api),
        db=Depends(get_async_db)
):
    """
    获取内容表现报表
    
    仅管理员可访问。
    """
    generator = ReportGenerator(db)
    report = await generator.generate_content_report(days, group_by)

    return ok(data=report)


@router.get("/user-activity", summary="获取用户活动报表")
@_catch
async def get_user_activity_report(
        days: int = Query(30, ge=7, le=90, description="统计天数"),
        current_user=Depends(admin_required_api)
):
    """
    获取用户活动报表
    
    仅管理员可访问。
    """
    report = report_generator.generate_user_activity_report(days)

    return ok(data=report)


@router.get("/traffic", summary="获取流量分析报表")
@_catch
async def get_traffic_report(
        days: int = Query(30, ge=7, le=90, description="统计天数"),
        current_user=Depends(admin_required_api)
):
    """
    获取流量分析报表
    
    仅管理员可访问。
    """
    report = report_generator.generate_traffic_report(days)

    return ok(data=report)


@router.post("/custom", summary="生成自定义报表")
@_catch
async def generate_custom_report(
        metrics: List[str] = Body(..., description="要包含的指标列表"),
        days: int = Body(30, ge=7, le=90, description="统计天数"),
        filters: Optional[dict] = Body(None, description="过滤条件"),
        current_user=Depends(admin_required_api)
):
    """
    生成自定义报表
    
    支持选择多个指标组合生成报表。
    """
    # 验证指标
    valid_metrics = ['content', 'users', 'traffic', 'engagement']
    invalid = [m for m in metrics if m not in valid_metrics]
    if invalid:
        return fail(f"无效的指标: {invalid}. 有效指标: {valid_metrics}")

    report = report_generator.generate_custom_report(metrics, days, filters)

    return ok(data=report)


@router.post("/export", summary="导出报表")
@_catch
async def export_report(
        report_type: str = Body(..., description="报表类型"),
        format: str = Body('json', enum=['json', 'csv'], description="导出格式"),
        days: int = Body(30, ge=7, le=90, description="统计天数"),
        current_user=Depends(admin_required_api)
):
    """
    导出报表为指定格式
    """
    # 生成报表
    if report_type == 'content':
        report = report_generator.generate_content_report(days)
    elif report_type == 'user-activity':
        report = report_generator.generate_user_activity_report(days)
    elif report_type == 'traffic':
        report = report_generator.generate_traffic_report(days)
    else:
        return fail(f"不支持的报表类型: {report_type}")

    # 导出
    exported = report_generator.export_report(report, format)

    return ok(data={
        'content': exported,
        'format': format,
        'type': report_type,
    })


@router.get("/templates", summary="获取报表面板")
@_catch
async def get_report_templates(current_user=Depends(admin_required_api)):
    """
    获取可用的报表面板和预设
    
    仅管理员可访问。
    """
    templates = [
        {
            'id': 'content-overview',
            'name': '内容概览',
            'description': '文章发布、浏览量、互动数据总览',
            'metrics': ['content'],
            'default_days': 30,
        },
        {
            'id': 'user-growth',
            'name': '用户增长',
            'description': '新用户、活跃用户、留存率分析',
            'metrics': ['users'],
            'default_days': 30,
        },
        {
            'id': 'traffic-analysis',
            'name': '流量分析',
            'description': '访问量、来源渠道、页面表现',
            'metrics': ['traffic'],
            'default_days': 30,
        },
        {
            'id': 'engagement-metrics',
            'name': '参与度分析',
            'description': '用户互动、停留时间、跳出率',
            'metrics': ['engagement'],
            'default_days': 30,
        },
        {
            'id': 'comprehensive',
            'name': '综合报表',
            'description': '包含所有指标的完整报表',
            'metrics': ['content', 'users', 'traffic', 'engagement'],
            'default_days': 30,
        },
    ]

    return ok(data={
        'templates': templates,
        'count': len(templates),
    })


@router.post("/scheduled", summary="创建定时报表任务")
@_catch
async def create_scheduled_report(
        name: str = Body(..., description="报表名称"),
        report_type: str = Body(..., description="报表类型"),
        frequency: str = Body('daily', enum=['daily', 'weekly', 'monthly'], description="执行频率"),
        metrics: Optional[List[str]] = Body(None, description="指标列表（custom类型需要）"),
        days: int = Body(30, ge=7, le=90, description="统计天数"),
        export_format: str = Body('json', enum=['json', 'csv'], description="导出格式"),
        current_user=Depends(admin_required_api),
        db = Depends(get_async_db)
):
    """
    创建定时报表任务
    
    支持每日/每周/每月自动生成报表。
    """
    service = create_scheduled_report_service(db)

    config = {
        'name': name,
        'type': report_type,
        'frequency': frequency,
        'metrics': metrics or [],
        'days': days,
        'export_format': export_format,
    }

    result = await service.create_scheduled_report(config)

    return ok(data=result)


@router.get("/scheduled", summary="获取定时报表任务列表")
@_catch
async def get_scheduled_reports(
        current_user=Depends(admin_required_api),
        db = Depends(get_async_db)
):
    """
    获取所有定时报表任务
    
    仅管理员可访问。
    """
    service = create_scheduled_report_service(db)
    reports = await service.get_scheduled_reports()

    return ok(data={
        'reports': reports,
        'count': len(reports),
    })


@router.post("/scheduled/{report_id}/toggle", summary="启用/禁用定时报表")
@_catch
async def toggle_scheduled_report(
        report_id: int,
        current_user=Depends(admin_required_api),
        db = Depends(get_async_db)
):
    """
    启用或禁用定时报表任务
    
    仅管理员可访问。
    """
    service = create_scheduled_report_service(db)
    result = await service.toggle_report(report_id)

    return ok(data=result)
