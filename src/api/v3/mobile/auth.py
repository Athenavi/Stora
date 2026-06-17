"""
Stora Mobile Auth API — lightweight auth for mobile
"""
from datetime import datetime

from fastapi import APIRouter, Depends, Form
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import User
from src.auth import hash_password, verify_password, create_access_token, jwt_required
from src.api.v2._helpers import ok, fail
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["mobile-auth"])


@router.post("/login")
async def mobile_login(
    username: str = Form(...),
    password: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
):
    """移动端登录"""
    user = (await db.execute(select(User).where(User.username == username))).scalar_one_or_none()
    if not user or not verify_password(password, user.password):
        return fail("用户名或密码错误")
    user.last_login_at = datetime.utcnow()
    await db.commit()
    token = create_access_token({"sub": str(user.id), "username": user.username})
    return ok({"access_token": token, "token_type": "bearer", "user": {"id": user.id, "username": user.username}})


@router.post("/api/v3/auth/register")
async def mobile_register(
    username: str = Form(...),
    email: str = Form(...),
    password: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
):
    """移动端注册"""
    existing = (await db.execute(select(User).where((User.username == username) | (User.email == email)))).scalar_one_or_none()
    if existing:
        return fail("用户名或邮箱已注册")
    user = User(username=username, email=email, password=hash_password(password), date_joined=datetime.utcnow())
    db.add(user)
    await db.commit()
    return ok({"id": user.id, "username": user.username})


@router.get("/api/v3/auth/me")
async def mobile_me(
    current_user: dict = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """当前用户信息"""
    user = await db.get(User, int(current_user.get("sub", 0)))
    if not user:
        return fail("用户不存在")
    return ok({"id": user.id, "username": user.username, "email": user.email, "is_superuser": user.is_superuser})
