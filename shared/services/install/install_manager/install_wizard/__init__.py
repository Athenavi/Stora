"""
安装向导 — 服务入口
将各步骤子模块组合为 InstallationWizardService 类
"""
import os
from pathlib import Path
from typing import Dict, Any, List

from src.unified_logger import default_logger as logger

from .prerequisites import check_prerequisites, check_database_connection, test_postgresql_connection
from .database import configure_database, confirm_database_and_migrate
from .admin_user import create_admin_user
from .site_settings import configure_site_settings


class InstallationWizardService:
    """安装向导服务 — 组合各步骤子模块"""

    def __init__(self):
        self.install_lock_file = Path("install.lock")
        self.install_flag_file = Path("storage/.installation_completed")
        _project_root = Path(__file__).resolve().parent.parent.parent.parent.parent
        self.config_dir = _project_root / "config"

    @property
    def project_root(self) -> Path:
        return Path(__file__).resolve().parent.parent.parent.parent.parent

    def ensure_directories(self):
        for d in ["storage", "storage/thumbnails", "config", "static"]:
            (self.project_root / d).mkdir(parents=True, exist_ok=True)

    def is_installed(self) -> bool:
        return self.install_flag_file.exists()

    def get_installation_status(self) -> Dict[str, Any]:
        return {
            "installed": self.is_installed(),
            "steps_completed": self._count_completed_steps(),
            "lock_file_exists": self.install_lock_file.exists(),
        }

    def _count_completed_steps(self) -> int:
        env_path = self.project_root / ".env"
        if not env_path.exists():
            return 0
        count = 1
        try:
            from src.utils.database.main import get_async_session_context
            from shared.models.system import SystemSettings
            import asyncio
            async def _check():
                async with get_async_session_context() as db:
                    q = __import__("sqlalchemy").select(SystemSettings)
                    r = await db.execute(q)
                    return len(r.scalars().all()) > 0
            try:
                loop = asyncio.get_running_loop()
            except RuntimeError:
                loop = None
            if loop and loop.is_running():
                # Already in an async context — skip this check
                pass
            else:
                if asyncio.get_event_loop().run_until_complete(_check()):
                    count += 1
        except Exception:
            pass
        return count

    def check_prerequisites(self) -> Dict[str, Any]:
        return check_prerequisites(self.project_root)

    def check_database_connection(self, config: Dict[str, str]) -> Dict[str, Any]:
        return check_database_connection(config)

    def configure_database(self, config: Dict[str, str]) -> Dict[str, Any]:
        return configure_database(self.project_root, config)

    def create_admin_user(self, username: str, email: str, password: str):
        import asyncio
        loop = asyncio.new_event_loop()
        try:
            return loop.run_until_complete(create_admin_user(self.project_root, username, email, password))
        finally:
            loop.close()

    def configure_site_settings(self, settings: Dict[str, str]) -> Dict[str, Any]:
        return configure_site_settings(self.project_root, settings)

    def confirm_database_and_migrate(self) -> Dict[str, Any]:
        return confirm_database_and_migrate(self.project_root)

    def complete_installation(self, install_info: Dict[str, Any]) -> Dict[str, Any]:
        """完成安装：写入锁定文件"""
        try:
            self.install_flag_file.parent.mkdir(parents=True, exist_ok=True)
            self.install_flag_file.write_text("", encoding="utf-8")
            return {"success": True, "message": "安装完成"}
        except Exception as e:
            return {"success": False, "message": f"写入安装标记失败: {str(e)}"}

    def reset_installation(self) -> Dict[str, Any]:
        """重置安装状态"""
        try:
            if self.install_flag_file.exists():
                self.install_flag_file.unlink()
            return {"success": True, "message": "安装状态已重置"}
        except Exception as e:
            return {"success": False, "message": f"重置失败: {str(e)}"}

    def get_installation_steps(self) -> List[Dict[str, str]]:
        return [
            {"key": "prerequisites", "label": "环境检测", "description": "检查 Python/数据库/目录权限"},
            {"key": "database", "label": "数据库配置", "description": "配置 PostgreSQL 连接"},
            {"key": "migration", "label": "数据库迁移", "description": "执行 Alembic 迁移"},
            {"key": "admin", "label": "创建管理员", "description": "设置管理员账号"},
            {"key": "settings", "label": "站点设置", "description": "配置站点信息"},
            {"key": "complete", "label": "完成安装", "description": "锁定安装状态"},
        ]

    def import_sample_data(self) -> Dict[str, Any]:
        """导入示例数据"""
        from .sample_data import import_sample_data as _import
        return _import(self.project_root)


# 全局实例
installation_wizard_service = InstallationWizardService()
