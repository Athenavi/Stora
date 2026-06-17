"""
VIP 离线下载服务
提供 OfflineDownload 下载任务的 VIP 权限检测、等级限制管理
"""
from datetime import datetime
from typing import Dict, Optional, Tuple

from sqlalchemy import select, func, update
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import User
from shared.models.media import DownloadTask
from shared.services.performance.resource_transfer_service import ResourceTransferService

from src.unified_logger import default_logger as logger


# ─── VIP 等级限制配置 ───────────────────────────────────────
VIP_LIMITS: Dict[int, Dict] = {
    0: {  # 非 VIP — 不可使用离线下载
        "allowed": False,
        "max_concurrent": 0,
        "max_file_size_mb": 0,
        "max_pending": 0,
    },
    1: {  # 基础 VIP
        "allowed": True,
        "max_concurrent": 2,
        "max_file_size_mb": 50,
        "max_pending": 5,
    },
    2: {  # 高级 VIP
        "allowed": True,
        "max_concurrent": 5,
        "max_file_size_mb": 200,
        "max_pending": 20,
    },
    3: {  # 顶级 VIP（Pro）
        "allowed": True,
        "max_concurrent": 10,
        "max_file_size_mb": 500,
        "max_pending": 50,
    },
}


def get_vip_limits(level: int) -> Dict:
    """获取指定 VIP 等级的下载限制"""
    limits = VIP_LIMITS.get(level)
    if limits is None:
        # 降级到最接近的较低等级
        sorted_levels = sorted(VIP_LIMITS.keys())
        for lvl in reversed(sorted_levels):
            if lvl <= level:
                return VIP_LIMITS[lvl]
        return VIP_LIMITS[0]
    return limits


class OfflineDownloadService:
    """VIP 离线下载服务"""

    def __init__(self, db: AsyncSession, user: User):
        self.db = db
        self.user = user
        self._vip_level: Optional[int] = None
        self._limits: Optional[Dict] = None
        self.transfer_service = ResourceTransferService(db)

    async def _resolve_vip_level(self) -> int:
        """
        从 VIPSubscription 表查询用户当前有效 VIP 等级
        （仅当 User.vip_level 为 0 时才查订阅表，避免冗余查询）
        """
        if self._vip_level is not None:
            return self._vip_level

        # 先检查 User 模型上的 vip_level（某些场景下可能已同步）
        if self.user.vip_level and self.user.vip_level > 0 and self.user.vip_expires_at:
            if self.user.vip_expires_at > datetime.now():
                self._vip_level = int(self.user.vip_level)
                self._limits = get_vip_limits(self._vip_level)
                return self._vip_level

        # 查询 VIPSubscription 表（真正的权威数据源）
        # [REMOVED] vip model deleted in Stora refactor
        # [REMOVED] vip model deleted in Stora refactor

        now = datetime.now()
        # 注意：VIPSubscription 的 FK 列名为 user 和 plan（不是 user_id / plan_id）
        stmt = select(VIPSubscription).join(
            VIPPlan, VIPSubscription.plan == VIPPlan.id
        ).where(
            VIPSubscription.user == self.user.id,
            VIPSubscription.status == 1,
            VIPSubscription.expires_at > now
        ).order_by(
            VIPSubscription.expires_at.desc()
        ).limit(1)

        result = await self.db.execute(stmt)
        subscription = result.scalar_one_or_none()

        if subscription and subscription.plan:
            self._vip_level = int(subscription.plan.level)
        else:
            self._vip_level = 0

        self._limits = get_vip_limits(self._vip_level)
        return self._vip_level

    async def _get_limits(self) -> Dict:
        """确保 VIP 等级已解析后返回限制配置"""
        if self._limits is None:
            await self._resolve_vip_level()
        return self._limits  # type: ignore

    async def get_vip_level(self) -> int:
        """获取用户当前有效 VIP 等级"""
        return await self._resolve_vip_level()

    # ── 权限检查 ──────────────────────────────────────────

    async def check_access(self) -> Tuple[bool, Optional[str]]:
        """
        检查用户是否有权使用离线下载
        
        Returns:
            (allowed, error_message)
        """
        limits = await self._get_limits()
        if not limits["allowed"]:
            return False, "该功能仅限 VIP 会员使用，请升级您的账户"
        return True, None

    async def check_download_limits(self) -> Tuple[bool, Optional[str]]:
        """
        检查用户是否达到下载限制
        
        Returns:
            (allowed, error_message)
        """
        allowed, error = await self.check_access()
        if not allowed:
            return False, error

        limits = await self._get_limits()

        # 查当前 pending + downloading 数量
        stmt = select(func.count(DownloadTask.id)).where(
            DownloadTask.user_id == self.user.id,
            DownloadTask.status.in_(["pending", "downloading"])
        )
        result = await self.db.execute(stmt)
        active_count = result.scalar() or 0

        if active_count >= limits["max_concurrent"]:
            return False, f"当前 VIP 等级最多同时下载 {limits['max_concurrent']} 个文件，请等待现有任务完成"

        # 查 pending 数量
        pending_stmt = select(func.count(DownloadTask.id)).where(
            DownloadTask.user_id == self.user.id,
            DownloadTask.status == "pending"
        )
        pending_result = await self.db.execute(pending_stmt)
        pending_count = pending_result.scalar() or 0

        if pending_count >= limits["max_pending"]:
            return False, f"待处理任务已达上限 ({limits['max_pending']})，请先完成或取消部分任务"

        return True, None

    async def check_file_size_limit(self, file_size_mb: int) -> Tuple[bool, Optional[str]]:
        """检查文件大小是否超过 VIP 等级限制"""
        limits = await self._get_limits()
        if file_size_mb > limits["max_file_size_mb"]:
            return False, f"文件大小超过当前 VIP 等级限制 ({limits['max_file_size_mb']}MB)"
        return True, None

    # ── 任务管理 ──────────────────────────────────────────

    async def create_download_task(
        self,
        source_url: str,
        resource_type: str = "image",
        filename: Optional[str] = None,
    ) -> Tuple[Optional[DownloadTask], Optional[str]]:
        """
        创建 VIP 离线下载任务
        
        Returns:
            (task, error_message)
        """
        # 权限检查
        allowed, error = await self.check_download_limits()
        if not allowed:
            return None, error

        # URL 验证
        if not source_url or not source_url.startswith(('http://', 'https://')):
            return None, "URL 必须以 http:// 或 https:// 开头"

        # 检查待处理任务总大小限制（防止队列堆积过大）
        MAX_PENDING_SIZE = 1 * 1024 * 1024 * 1024  # 1 GB
        pending_size_stmt = select(func.coalesce(func.sum(DownloadTask.total_size), 0)).where(
            DownloadTask.user_id == self.user.id,
            DownloadTask.status == "pending"
        )
        pending_size_result = await self.db.execute(pending_size_stmt)
        current_pending_size = pending_size_result.scalar() or 0
        if current_pending_size >= MAX_PENDING_SIZE:
            logger.warning(
                f"[OfflineDownload] 用户 {self.user.id} 待处理任务总大小 "
                f"({current_pending_size / 1024 / 1024:.1f}MB) 超过限制"
            )
            return None, "待处理任务总大小超过 1GB 限制，请等待现有任务完成后再试"

        try:
            task = DownloadTask(
                user_id=self.user.id,
                source_url=source_url,
                resource_type=resource_type,
                filename=filename or source_url.rsplit('/', 1)[-1] or "download",
                status="pending",
                priority=await self.get_vip_level(),  # 高级 VIP 优先处理
                retry_count=0,
                max_retries=3,
            )
            self.db.add(task)
            await self.db.flush()
            await self.db.refresh(task)

            vip_level = await self.get_vip_level()
            logger.info(f"[OfflineDownload] VIP用户 {self.user.id}(Lv.{vip_level}) 创建离线下载任务 {task.id}: {source_url}")
            return task, None

        except Exception as e:
            logger.error(f"[OfflineDownload] 创建任务失败: {e}", exc_info=True)
            return None, f"创建任务失败: {str(e)}"

    async def get_user_tasks(
        self,
        status: Optional[str] = None,
        page: int = 1,
        per_page: int = 20,
    ) -> Dict:
        """
        获取用户的离线下载任务列表
        
        Returns:
            { "tasks": [...], "pagination": {...} }
        """
        query = select(DownloadTask).where(
            DownloadTask.user_id == self.user.id
        )

        if status:
            query = query.where(DownloadTask.status == status)

        # 总数
        count_stmt = select(func.count(DownloadTask.id)).where(
            DownloadTask.user_id == self.user.id
        )
        if status:
            count_stmt = count_stmt.where(DownloadTask.status == status)
        count_result = await self.db.execute(count_stmt)
        total = count_result.scalar() or 0

        # 分页
        offset = (page - 1) * per_page
        query = query.order_by(DownloadTask.created_at.desc()).offset(offset).limit(per_page)
        result = await self.db.execute(query)
        tasks = result.scalars().all()

        tasks_data = []
        for t in tasks:
            tasks_data.append({
                "id": t.id,
                "source_url": t.source_url,
                "resource_type": t.resource_type,
                "filename": t.filename,
                "status": t.status,
                "progress": t.progress,
                "total_size": t.total_size,
                "downloaded_size": t.downloaded_size,
                "error_message": t.error_message,
                "media_id": t.media_id,
                "retry_count": t.retry_count,
                "created_at": t.created_at.isoformat() if t.created_at else None,
                "updated_at": t.updated_at.isoformat() if t.updated_at else None,
                "completed_at": t.completed_at.isoformat() if t.completed_at else None,
            })

        return {
            "tasks": tasks_data,
            "pagination": {
                "page": page,
                "per_page": per_page,
                "total": total,
                "pages": (total + per_page - 1) // per_page if total > 0 else 1,
            }
        }

    async def get_task_detail(self, task_id: int) -> Tuple[Optional[Dict], Optional[str]]:
        """获取单个任务详情"""
        result = await self.db.execute(
            select(DownloadTask).where(
                DownloadTask.id == task_id,
                DownloadTask.user_id == self.user.id
            )
        )
        task = result.scalar_one_or_none()
        if not task:
            return None, "任务不存在"

        return {
            "id": task.id,
            "source_url": task.source_url,
            "resource_type": task.resource_type,
            "filename": task.filename,
            "status": task.status,
            "progress": task.progress,
            "total_size": task.total_size,
            "downloaded_size": task.downloaded_size,
            "error_message": task.error_message,
            "media_id": task.media_id,
            "retry_count": task.retry_count,
            "created_at": task.created_at.isoformat() if task.created_at else None,
            "updated_at": task.updated_at.isoformat() if task.updated_at else None,
            "started_at": task.started_at.isoformat() if task.started_at else None,
            "completed_at": task.completed_at.isoformat() if task.completed_at else None,
        }, None

    async def cancel_task(self, task_id: int) -> Tuple[bool, Optional[str]]:
        """取消任务"""
        success = await self.transfer_service.cancel_task(task_id, self.user.id)
        if not success:
            return False, "无法取消任务（可能已完成或不存在）"
        return True, None

    async def retry_task(self, task_id: int) -> Tuple[bool, Optional[str]]:
        """重试失败的任务"""
        result = await self.db.execute(
            select(DownloadTask).where(
                DownloadTask.id == task_id,
                DownloadTask.user_id == self.user.id
            )
        )
        task = result.scalar_one_or_none()

        if not task:
            return False, "任务不存在"

        if task.status != "failed":
            return False, "只能重试失败的任务"

        await self.db.execute(
            update(DownloadTask)
            .where(DownloadTask.id == task_id)
            .values(
                status="pending",
                error_message=None,
                progress=0,
                downloaded_size=0,
                updated_at=datetime.now()
            )
        )
        await self.db.commit()
        return True, None

    async def get_user_limits(self) -> Dict:
        """获取用户的离线下载限制信息"""
        limits = await self._get_limits()
        vip_level = await self.get_vip_level()
        allowed, _ = await self.check_access()

        # 当前活跃任务数
        active_stmt = select(func.count(DownloadTask.id)).where(
            DownloadTask.user_id == self.user.id,
            DownloadTask.status.in_(["pending", "downloading"])
        )
        active_result = await self.db.execute(active_stmt)
        active_count = active_result.scalar() or 0

        return {
            "vip_level": vip_level,
            "allowed": allowed,
            "max_concurrent": limits["max_concurrent"],
            "max_file_size_mb": limits["max_file_size_mb"],
            "max_pending": limits["max_pending"],
            "active_count": active_count,
            "remaining_slots": max(0, limits["max_concurrent"] - active_count),
        }
