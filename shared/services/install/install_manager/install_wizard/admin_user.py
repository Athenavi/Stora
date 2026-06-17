"""
安装向导 — 管理员创建模块
"""
from pathlib import Path
from typing import Dict, Any, Optional
from datetime import datetime

from src.unified_logger import default_logger as logger


async def create_admin_user(project_root: Path, username: str, email: str, password: str) -> Dict[str, Any]:
    """创建管理员用户"""
    if not username or not email or not password:
        return {"success": False, "message": "用户名、邮箱和密码不能为空"}
    if len(password) < 6:
        return {"success": False, "message": "密码长度不能少于6位"}

    try:
        from src.utils.database.main import get_async_session_context
        from shared.models.user import User as UserModel
        from shared.services.users.user_manager import create_user_account

        async with get_async_session_context() as db:
            existing = await db.execute(
                __import__("sqlalchemy").select(UserModel).where(UserModel.username == username)
            )
            if existing.scalar_one_or_none():
                return {"success": False, "message": "用户名已存在"}

            now = datetime.utcnow()
            admin = UserModel(
                username=username,
                email=email,
                is_superuser=True,
                is_active=True,
                date_joined=now,
            )
            admin.set_password(password)
            db.add(admin)
            await db.commit()
            return {"success": True, "message": f"管理员 {username} 创建成功", "user_id": admin.id}
    except Exception as e:
        logger.exception(f"创建管理员失败")
        return {"success": False, "message": f"创建失败: {str(e)}"}
