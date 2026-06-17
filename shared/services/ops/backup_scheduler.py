"""
P8-3: 定时备份任务调度器（已迁移到 src/scheduler.py）
使用 APScheduler 实现自动化备份调度

此模块仅保留任务函数供中心调度器调用
"""
import asyncio
from datetime import datetime

from shared.services.ops.backup_manager import backup_manager
from shared.services.ops.health_checker import health_checker
from src.unified_logger import default_logger as logger


# 任务函数已迁移到 src/scheduler.py，此文件仅作兼容引用

class _DummyScheduler:
    """最小调度器桩，完整功能已迁移到 src/scheduler.py"""
    def add_job(self, func, trigger=None, **kwargs):
        pass
    def start(self):
        pass
    def shutdown(self):
        pass
    def get_jobs(self):
        return []


class BackupScheduler:
    """备份调度器 — 兼容旧引用，实际已迁移到 src/scheduler.py"""

    def __init__(self):
        self.scheduler = _DummyScheduler()

    async def daily_backup_job(self):
        """每日备份任务"""
        logger.info("Starting daily database backup...")
        result = await backup_manager.create_database_backup('daily')

        if result['success']:
            logger.info(f"Daily backup completed: {result['filename']}")
        else:
            logger.error(f"Daily backup failed: {result.get('error')}")

    async def weekly_backup_job(self):
        """每周备份任务"""
        logger.info("Starting weekly full backup...")

        # 数据库备份
        db_result = await backup_manager.create_database_backup('weekly')

        # 文件备份
        files_result = await backup_manager.create_files_backup()

        if db_result['success'] and files_result['success']:
            logger.info("Weekly full backup completed")
        else:
            logger.error("Weekly backup partially failed")

    async def monthly_backup_job(self):
        """每月备份任务"""
        logger.info("Starting monthly archive backup...")
        result = await backup_manager.create_database_backup('monthly')

        if result['success']:
            logger.info(f"Monthly backup completed: {result['filename']}")
        else:
            logger.error(f"Monthly backup failed: {result.get('error')}")

    async def hourly_health_check(self):
        """每小时健康检查"""
        logger.debug("Running hourly health check...")
        health = await health_checker.check_application_health()

        if health['status'] != 'healthy':
            logger.warning(f"Health check failed: {health}")

    def start(self):
        """启动调度器"""
        self.scheduler.start()
        logger.info("Backup scheduler started (dummy)")

    def stop(self):
        """停止调度器"""
        self.scheduler.shutdown()
        logger.info("Backup scheduler stopped")


# 全局实例
backup_scheduler = BackupScheduler()
