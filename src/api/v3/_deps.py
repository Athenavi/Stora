"""
V3 共享依赖

为所有 V3 路由模块提供统一的依赖注入入口，
避免每个模块重复 import 路径。
"""
from sqlalchemy.ext.asyncio import AsyncSession
from shared.models.user import User as UserModel
from src.auth.auth_deps import get_current_user, jwt_optional_dependency as get_optional_user
from src.utils.database.main import get_async_session as get_db
