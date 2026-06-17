"""
系统管理API聚合路由器 - V2统一入口
整合V1的system相关模块

使用懒加载模式：仅在首次访问 router 时才导入 V1 子模块。
"""
import os
import platform
import sys
from pathlib import Path

from fastapi import APIRouter, Depends, Query, Request

from src.api.v2._base import ApiResponse


_router = None


def _build_router():
    global _router
    if _router is not None:
        return _router

    router = APIRouter(tags=["system"])

    from src.auth.auth_deps import admin_required as admin_required_api

    # ── 原生 V2 路由（已从 V1 迁移）────



    @router.get("/health", summary="站点健康检查",
                description="运行完整的站点健康检查,返回各项指标和评分(仅管理员)")
    async def site_health_check_api(
            request: Request,
            format: str = Query('json', description="报告格式: json或text"),
            current_user=Depends(admin_required_api)
    ):
        try:
            from shared.services.system.site_health import site_health_service
            if format == 'text':
                report = site_health_service.generate_report('text')
                from fastapi.responses import PlainTextResponse
                return PlainTextResponse(content=report)
            health_data = site_health_service.run_full_check()
            return ApiResponse(success=True, data=health_data)
        except Exception as e:
            import traceback
            print(f"Error in site_health_check_api: {str(e)}")
            print(traceback.format_exc())
            return ApiResponse(success=False, error=str(e))

    @router.get("/health/ping", summary="简易存活检查",
                description="公开端点，仅返回服务是否存活")
    async def ping():
        return {"status": "ok", "timestamp": __import__('datetime').datetime.now().isoformat()}

    @router.get("/health/readiness", summary="就绪检查",
                description="公开端点，检查数据库等核心依赖是否就绪")
    async def readiness():
        issues = []
        try:
            from src.extensions import get_async_db_session
            from sqlalchemy import text
            async for db in get_async_db_session():
                await db.execute(text("SELECT 1"))
                break
        except Exception:
            issues.append("database_unreachable")
        return {"status": "ready" if not issues else "degraded", "issues": issues}

    @router.get("/info", summary="系统信息",
                description="获取系统基本信息(仅管理员)")
    async def system_info_api(
            request: Request,
            current_user=Depends(admin_required_api)
    ):
        try:
            base_dir = Path(__file__).resolve().parent.parent.parent.parent
            info = {
                'python_version': f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}",
                'platform': f"{platform.system()} {platform.release()}",
                'base_dir': str(base_dir),
                'environment': os.getenv('ENVIRONMENT', 'development'),
                'debug_mode': os.getenv('DEBUG', 'False').lower() in ('true', '1', 'yes'),
            }
            return ApiResponse(success=True, data=info)
        except Exception as e:
            import traceback
            print(f"Error in system_info_api: {str(e)}")
            print(traceback.format_exc())
            return ApiResponse(success=False, error=str(e))

    # ── V1 聚合子模块 ────
    from src.api.v2.system.admin_settings import router as admin_settings_router
    from src.api.v2.system.backup_management import router as backup_management_router
    from src.api.v2.system.batch_operations import router as batch_operations_router
    from src.api.v2.system.data_export import router as data_export_router
    from src.api.v2.system.database_migration import router as database_migration_router
    from src.api.v2.system.incremental_backup import router as incremental_backup_router
    from src.api.v2.system.installation import router as installation_router
    from src.api.v2.system.maintenance import router as maintenance_router
    from src.api.v2.system.migrations import router as migrations_router
    from src.api.v2.system.multisite import router as multisite_router
    from src.api.v2.system.report_management import router as report_management_router
    from src.api.v2.system.resource_transfer import router as resource_transfer_router
    from src.api.v2.system.screen_options import router as screen_options_router
    from src.api.v2.system.webhook_management import router as webhook_management_router
    from src.api.v2.system.workflow import router as workflow_router
    from src.api.v2.system.migration_management import router as migration_management_router
    from src.api.v2.system.version import router as version_router

    router.include_router(admin_settings_router, prefix="/settings")
    router.include_router(database_migration_router, prefix="/db/database-migration")
    router.include_router(report_management_router, prefix="/report")
    router.include_router(webhook_management_router, prefix="/webhook")
    router.include_router(incremental_backup_router, prefix="/backup-plus")
    router.include_router(backup_management_router, prefix="/backup")
    router.include_router(batch_operations_router, prefix="/batch")
    router.include_router(data_export_router, prefix="/export")
    router.include_router(installation_router, prefix="/install")
    router.include_router(maintenance_router, prefix="/maintenance")
    router.include_router(migrations_router, prefix="/migrations")
    router.include_router(multisite_router, prefix="/multisite")
    router.include_router(resource_transfer_router, prefix="/transfer")
    router.include_router(screen_options_router, prefix="/screen-options")
    router.include_router(workflow_router, prefix="/workflow")
    router.include_router(migration_management_router, prefix="/migration-management")
    router.include_router(version_router, prefix="")

    _router = router
    return _router


def __getattr__(name):
    if name == "router":
        return _build_router()
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
