"""
向后兼容层 — 原 src.auth.auth_deps 模块

提供 jwt_required_dependency / admin_required / get_current_user / get_current_active_user
等依赖注入函数，从 DB 返回完整的 User 模型。
"""
from fastapi import Depends, HTTPException, Request
from fastapi.security import HTTPBearer

from shared.models.user import User
from src.auth.auth_handler import decode_token
from src.extensions import get_async_db_session

_bearer = HTTPBearer(auto_error=False)


async def jwt_required_dependency(
    credentials=Depends(_bearer),
    request: Request = None,
) -> User:
    """验证 JWT 并查询返回 User 模型（含 .id / .is_superuser 等属性）"""
    token = None
    if credentials:
        token = credentials.credentials
    elif request:
        auth = request.headers.get("Authorization")
        if auth and auth.startswith("Bearer "):
            token = auth[7:]
        else:
            token = request.cookies.get("access_token")
    if not token:
        raise HTTPException(status_code=401, detail="Not authenticated")

    payload = decode_token(token)
    user_id = int(payload.get("sub", 0))
    if not user_id:
        raise HTTPException(status_code=401, detail="Invalid token payload")

    async for db in get_async_db_session():
        user = await db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=401, detail="User not found")

    return user


async def admin_required(current_user: User = Depends(jwt_required_dependency)) -> User:
    """验证 JWT 并检查管理员权限"""
    if not (getattr(current_user, "is_superuser", False) or getattr(current_user, "is_staff", False)):
        raise HTTPException(status_code=403, detail="Admin access required")
    return current_user


async def get_current_active_user(current_user: User = Depends(jwt_required_dependency)) -> User:
    """验证 JWT 并返回活跃用户"""
    if not getattr(current_user, "is_active", True):
        raise HTTPException(status_code=403, detail="Inactive user")
    return current_user


get_current_user = jwt_required_dependency
