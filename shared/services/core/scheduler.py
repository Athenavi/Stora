import asyncio

from typing import Optional


class ScheduledPublishScheduler:

    def __init__(self, db_session_factory, check_interval: int = 60):
        self.db_session_factory = db_session_factory
        self.check_interval = check_interval
        self.is_running = False
        self.task: Optional[asyncio.Task] = None

    async def start(self):
        if self.is_running:
            logger.warning("Scheduler is already running")
            return

        self.is_running = True
        self.task = asyncio.create_task(self._run_scheduler())
        logger.info(f"Scheduled publish scheduler started (interval: {self.check_interval}s)")

    async def stop(self):
        if not self.is_running:
            logger.warning("Scheduler is not running")
            return

        self.is_running = False

        if self.task:
            self.task.cancel()
            try:
                await self.task
            except asyncio.CancelledError:
                pass

        logger.info("Scheduled publish scheduler stopped")

    async def _run_scheduler(self):
        while self.is_running:
            try:
                # 检查系统是否已安装
                from shared.services.install.install_manager.installation_wizard import installation_wizard_service
                if not installation_wizard_service.is_installed():
                    logger.debug("System not installed, skipping scheduled publish check")
                    await asyncio.sleep(self.check_interval)
                    continue

                await self._check_and_publish()
            except Exception as e:
                logger.error(f"Error in scheduled publish scheduler: {e}")

            # 等待下一个检查周
            await asyncio.sleep(self.check_interval)



def init_scheduler(db_session_factory, check_interval: int = 60) -> ScheduledPublishScheduler:
    global scheduler
    scheduler = ScheduledPublishScheduler(db_session_factory, check_interval)
    return scheduler


def get_scheduler() -> Optional[ScheduledPublishScheduler]:
    """获取全局调度器实�?""
    return scheduler


async def start_scheduler():
    if scheduler:
        await scheduler.start()


async def stop_scheduler():
    if scheduler:
        await scheduler.stop()
