"""
Stora Auth — Login/Register/Profile routes
"""
from datetime import datetime

from fastapi import APIRouter, Depends, HTTPException, Form
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import User
from src.auth import hash_password, verify_password, create_access_token, create_refresh_token, jwt_required
from src.api.v2._helpers import ok, fail
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["auth"])


@router.post("/api/v2/auth/register")
async def register(
    username: str = Form(...),
    email: str = Form(...),
    password: str = Form(...),
    password_confirm: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
):
    """用户注册"""
    if password != password_confirm:
        return fail("两次密码不一致")
    if len(password) < 6:
        return fail("密码至少 6 位")
    if len(username) < 3:
        return fail("用户名至少 3 位")

    # Check existing
    existing = (await db.execute(
        select(User).where((User.username == username) | (User.email == email))
    )).scalar_one_or_none()
    if existing:
        return fail("用户名或邮箱已被注册")

    user = User(
        username=username,
        email=email,
        password=hash_password(password),
        date_joined=datetime.utcnow(),
    )
    db.add(user)
    await db.commit()
    await db.refresh(user)

    return ok({
        "id": user.id,
        "username": user.username,
        "email": user.email,
    }, msg="注册成功")


@router.post("/api/v2/auth/login")
async def login(
    username: str = Form(...),
    password: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
):
    """用户登录"""
    user = (await db.execute(
        select(User).where(User.username == username)
    )).scalar_one_or_none()

    if not user or not verify_password(password, user.password):
        return fail("用户名或密码错误")

    if not user.is_active:
        return fail("账号已被禁用")

    # Update last login
    user.last_login_at = datetime.utcnow()
    await db.commit()

    token_data = {"sub": str(user.id), "username": user.username}
    access_token = create_access_token(token_data)
    refresh_token = create_refresh_token(token_data)

    return ok({
        "access_token": access_token,
        "refresh_token": refresh_token,
        "token_type": "bearer",
        "expires_in": 7200,
        "user": {
            "id": user.id,
            "username": user.username,
            "email": user.email,
            "is_superuser": user.is_superuser,
        },
    })


@router.get("/api/v2/auth/me")
async def get_me(
    current_user: dict = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """获取当前用户信息"""
    user_id = int(current_user.get("sub", 0))
    user = await db.get(User, user_id)
    if not user:
        return fail("用户不存在")
    return ok({
        "id": user.id,
        "username": user.username,
        "email": user.email,
        "profile_picture": user.profile_picture,
        "bio": user.bio,
        "is_active": user.is_active,
        "is_superuser": user.is_superuser,
        "date_joined": str(user.date_joined) if user.date_joined else None,
        "last_login_at": str(user.last_login_at) if user.last_login_at else None,
    })


@router.post("/api/v2/auth/logout")
async def logout():
    """登出（客户端清除 token 即可）"""
    return ok(msg="已登出")
