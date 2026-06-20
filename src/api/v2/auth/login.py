"""
Stora Auth — Login/Register/Profile routes + email code login
"""
import random
import string
from datetime import datetime, timedelta

from fastapi import APIRouter, Depends, HTTPException, Form
from fastapi.responses import JSONResponse
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import User
from src.auth import hash_password, verify_password, create_access_token, create_refresh_token, jwt_required
from src.api.v2._helpers import ok, fail
from src.extensions import get_async_db_session as get_async_db
from src.setting import app_config

router = APIRouter(tags=["auth"])

# In-memory verification code store (production should use Redis)
_verify_codes: dict[str, dict] = {}


def _generate_code(length=6) -> str:
    return "".join(random.choices(string.digits, k=length))


# ─── Email verification code login ───


@router.post("/send-code")
async def send_verify_code(
    email: str = Form(...),
):
    """发送邮箱验证码"""
    if not email or "@" not in email:
        return fail("请输入有效邮箱地址")

    # Rate limit: don't send if a valid code already exists
    existing = _verify_codes.get(email)
    if existing and existing["expires_at"] > datetime.utcnow():
        # If code was sent less than 60s ago, reject
        elapsed = (datetime.utcnow() - existing["sent_at"]).total_seconds()
        if elapsed < 60:
            return fail(f"请 {int(60 - elapsed)} 秒后再试")

    code = _generate_code()
    _verify_codes[email] = {
        "code": code,
        "expires_at": datetime.utcnow() + timedelta(minutes=5),
        "sent_at": datetime.utcnow(),
    }

    # Try to send email
    try:
        from src.utils.send_email import send_email
        subject = "Stora 登录验证码"
        body = f"您的验证码是: {code}\n\n验证码有效期为 5 分钟。如非本人操作，请忽略此邮件。"
        await send_email(subject, body, [email])
    except Exception:
        # Fallback: return code directly for testing
        pass

    # In dev mode, return the code for convenience
    is_dev = getattr(app_config, "DEBUG", False) or getattr(app_config, "ENVIRONMENT", "") == "development"
    result = {"message": "验证码已发送"}
    if is_dev:
        result["code"] = code
    return ok(result)


@router.post("/login-with-code")
async def login_with_code(
    email: str = Form(...),
    code: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
):
    """使用邮箱验证码登录/注册"""
    record = _verify_codes.get(email)
    if not record:
        return fail("请先获取验证码")

    if record["code"] != code:
        return fail("验证码错误")

    if record["expires_at"] < datetime.utcnow():
        _verify_codes.pop(email, None)
        return fail("验证码已过期，请重新获取")

    # Consume the code
    _verify_codes.pop(email, None)

    # Find or create user
    user = (await db.execute(
        select(User).where(User.email == email)
    )).scalar_one_or_none()

    if not user:
        # Auto-register with email as username
        import secrets
        username = email.split("@")[0][:30]
        # Ensure unique username
        existing_username = (await db.execute(
            select(User).where(User.username == username)
        )).scalar_one_or_none()
        if existing_username:
            username = f"{username}_{secrets.token_hex(2)}"

        user = User(
            username=username,
            email=email,
            password=hash_password(secrets.token_urlsafe(16)),
            date_joined=datetime.utcnow(),
        )
        db.add(user)
        await db.flush()

    user.last_login_at = datetime.utcnow()
    await db.commit()

    token_data = {"sub": str(user.id), "username": user.username}
    access_token = create_access_token(token_data)
    refresh_token = create_refresh_token(token_data)

    return JSONResponse(
        content=ok({
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
        }),
        headers={
            "Set-Cookie": f"access_token={access_token}; Path=/; Max-Age=7200; SameSite=Lax;"
        }
    )


@router.post("/register")
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


@router.post("/login")
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

    return JSONResponse(
        content=ok({
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
        }),
        headers={
            "Set-Cookie": f"access_token={access_token}; Path=/; Max-Age=7200; SameSite=Lax;"
        }
    )


@router.get("/me")
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


@router.post("/logout")
async def logout():
    """登出（客户端清除 token 即可）"""
    return ok(msg="已登出")
