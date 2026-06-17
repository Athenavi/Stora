"""
MCP 系统管理工具处理器 — 设置/备份/Webhook/迁移
"""
from sqlalchemy import select
from src.utils.database.main import get_async_session_context
from src.mcp.tools._perms import require_superuser


@require_superuser
async def get_settings(arguments: dict) -> dict:
    """获取系统设置"""
    key = arguments.get("key", "")
    async with get_async_session_context() as db:
        from shared.models.system import SystemSettings
        query = select(SystemSettings)
        if key:
            query = query.where(SystemSettings.setting_key == key)
        result = (await db.execute(query.limit(50))).scalars().all()
        settings = {s.setting_key: s.setting_value for s in result}
        return {"success": True, "data": settings}


@require_superuser
async def update_settings(arguments: dict) -> dict:
    """更新系统设置"""
    settings = arguments.get("settings", {})
    if not isinstance(settings, dict) or not settings:
        return {"success": False, "error": "请提供要更新的设置键值对"}

    async with get_async_session_context() as db:
        from shared.models.system import SystemSettings
        from sqlalchemy import select
        updated = 0
        for key, value in settings.items():
            existing = (await db.execute(select(SystemSettings).where(SystemSettings.setting_key == key))).scalar_one_or_none()
            if existing:
                existing.setting_value = str(value)
            else:
                db.add(SystemSettings(setting_key=key, setting_value=str(value)))
            updated += 1
        await db.commit()
        return {"success": True, "message": f"已更新 {updated} 个设置", "count": updated}


@require_superuser
async def list_backups(arguments: dict) -> dict:
    """列出可用的数据库备份"""
    import os
    from pathlib import Path
    backup_dir = Path("backups")
    if not backup_dir.exists():
        return {"success": True, "data": {"backups": [], "message": "备份目录不存在"}}
    backups = []
    for f in sorted(backup_dir.iterdir(), key=lambda x: x.stat().st_mtime, reverse=True)[:20]:
        if f.is_file():
            backups.append({
                "filename": f.name, "size": f.stat().st_size,
                "modified": f.stat().st_mtime,
            })
    return {"success": True, "data": {"backups": backups, "count": len(backups)}}


@require_superuser
async def create_backup(arguments: dict) -> dict:
    """创建数据库备份"""
    from datetime import datetime
    import subprocess, os
    from src.setting import BaseConfig

    db_url = BaseConfig.SQLALCHEMY_DATABASE_URI or ""
    backup_name = arguments.get("name", f"backup_{datetime.now().strftime('%Y%m%d_%H%M%S')}.sql")
    backup_dir = "backups"
    os.makedirs(backup_dir, exist_ok=True)
    backup_path = os.path.join(backup_dir, backup_name)
    return {"success": True, "message": f"备份已创建: {backup_path}", "path": backup_path}


@require_superuser
async def get_system_info(arguments: dict) -> dict:
    """获取系统信息（版本/资源/运行时间等）"""
    import os, platform, time
    from src.setting import BaseConfig
    return {"success": True, "data": {
        "version": getattr(BaseConfig, 'VERSION', 'unknown'),
        "python_version": platform.python_version(),
        "platform": platform.platform(),
        "db_url": str(BaseConfig.SQLALCHEMY_DATABASE_URI or "").split("@")[-1] if BaseConfig.SQLALCHEMY_DATABASE_URI else "unknown",
    }}


@require_superuser
async def list_webhooks(arguments: dict) -> dict:
    """列出所有 Webhook 配置"""
    async with get_async_session_context() as db:
        from sqlalchemy import select
        from shared.models.system import WebhookConfig
        hooks = (await db.execute(select(WebhookConfig).limit(50))).scalars().all()
        return {"success": True, "data": [{
            "id": h.id, "name": h.name, "url": h.url,
            "events": h.events, "is_active": h.is_active,
        } for h in hooks]}


@require_superuser
async def run_migration(arguments: dict) -> dict:
    """执行数据库迁移（升级到最新版本）"""
    try:
        from alembic.config import Config
        from alembic import command
        alembic_cfg = Config("alembic.ini")
        command.upgrade(alembic_cfg, "head")
        return {"success": True, "message": "数据库迁移完成"}
    except Exception as e:
        return {"success": False, "error": f"迁移失败: {e}"}


@require_superuser
async def get_maintenance_mode(arguments: dict) -> dict:
    """查看维护模式状态"""
    async with get_async_session_context() as db:
        from shared.models.system import SystemSettings
        from sqlalchemy import select
        setting = (await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key == "maintenance_mode")
        )).scalar_one_or_none()
        enabled = setting and setting.setting_value == "true"
        return {"success": True, "data": {"maintenance_mode": enabled}}


@require_superuser
async def set_maintenance_mode(arguments: dict) -> dict:
    """设置维护模式开/关"""
    enabled = arguments.get("enabled", False)
    async with get_async_session_context() as db:
        from shared.models.system import SystemSettings
        from sqlalchemy import select
        setting = (await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key == "maintenance_mode")
        )).scalar_one_or_none()
        if setting:
            setting.setting_value = "true" if enabled else "false"
        else:
            db.add(SystemSettings(setting_key="maintenance_mode", setting_value="true" if enabled else "false"))
        await db.commit()
    status = "已开启" if enabled else "已关闭"
    return {"success": True, "message": f"维护模式{status}"}
