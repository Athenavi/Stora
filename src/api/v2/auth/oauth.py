"""
Stora OAuth 登录 — Google + GitHub OAuth 2.0

端点:
  GET  /api/v2/auth/oauth/{provider}     → 重定向到 OAuth 授权页
  GET  /api/v2/auth/oauth/callback        → OAuth 回调处理

配置 (.env):
  GITHUB_CLIENT_ID / GITHUB_CLIENT_SECRET
  GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET
  OAUTH_REDIRECT_URI
"""
import httpx
from datetime import datetime
from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import RedirectResponse
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models import User
from src.auth import hash_password, create_access_token, jwt_optional
from src.api.v2._helpers import ok, fail
from src.extensions import get_async_db_session as get_async_db
from src.setting import settings

router = APIRouter(prefix="/oauth", tags=["oauth"])

PROVIDERS = {
    "github": {
        "authorize_url": "https://github.com/login/oauth/authorize",
        "token_url": "https://github.com/login/oauth/access_token",
        "user_url": "https://api.github.com/user",
        "client_id": settings.GITHUB_CLIENT_ID,
        "client_secret": settings.GITHUB_CLIENT_SECRET,
        "scope": "read:user user:email",
    },
    "google": {
        "authorize_url": "https://accounts.google.com/o/oauth2/v2/auth",
        "token_url": "https://oauth2.googleapis.com/token",
        "user_url": "https://www.googleapis.com/oauth2/v2/userinfo",
        "client_id": settings.GOOGLE_CLIENT_ID,
        "client_secret": settings.GOOGLE_CLIENT_SECRET,
        "scope": "openid email profile",
    },
}


@router.get("/{provider}")
async def oauth_login(provider: str, request: Request):
    """重定向到 OAuth 提供商授权页"""
    p = PROVIDERS.get(provider)
    if not p or not p["client_id"]:
        return fail(f"不支持的 OAuth 提供商: {provider}")

    params = {
        "client_id": p["client_id"],
        "redirect_uri": settings.OAUTH_REDIRECT_URI,
        "scope": p["scope"],
        "response_type": "code",
        "state": provider,
    }
    url = f"{p['authorize_url']}?{'&'.join(f'{k}={v}' for k, v in params.items())}"
    return RedirectResponse(url=url)


@router.get("/callback")
async def oauth_callback(
    code: str = Query(...),
    state: str = Query("github"),
    db: AsyncSession = Depends(get_async_db),
    current_user: dict = Depends(jwt_optional),
):
    """OAuth 回调处理"""
    p = PROVIDERS.get(state)
    if not p:
        return fail("无效的 OAuth 提供商")

    # Exchange code for token
    async with httpx.AsyncClient() as client:
        token_resp = await client.post(
            p["token_url"],
            data={
                "client_id": p["client_id"],
                "client_secret": p["client_secret"],
                "code": code,
                "redirect_uri": settings.OAUTH_REDIRECT_URI,
            },
            headers={"Accept": "application/json"},
        )
        token_data = token_resp.json()
        access_token = token_data.get("access_token")
        if not access_token:
            return fail("OAuth 授权失败")

        # Get user info
        user_resp = await client.get(
            p["user_url"],
            headers={"Authorization": f"Bearer {access_token}"},
        )
        user_info = user_resp.json()

    # Extract user details
    if state == "github":
        oauth_id = str(user_info.get("id"))
        username = user_info.get("login", f"github_{oauth_id}")
        email = user_info.get("email") or f"{username}@github.oauth"
        avatar = user_info.get("avatar_url", "")
    else:
        oauth_id = user_info.get("id")
        username = user_info.get("name", user_info.get("email", f"google_{oauth_id}"))
        email = user_info.get("email", f"{oauth_id}@google.oauth")
        avatar = user_info.get("picture", "")

    # Find or create user
    user = (await db.execute(
        select(User).where(User.email == email)
    )).scalar_one_or_none()

    if not user:
        # Create new user
        import secrets
        user = User(
            username=username,
            email=email,
            password=hash_password(secrets.token_urlsafe(16)),
            profile_picture=avatar or None,
            date_joined=datetime.utcnow(),
        )
        db.add(user)
        await db.flush()

    # Generate JWT
    token = create_access_token({"sub": str(user.id), "username": user.username})

    # Redirect to frontend with token
    return RedirectResponse(
        url=f"/drive?token={token}",
        headers={"Set-Cookie": f"stora_token={token}; Path=/; Max-Age=7200; SameSite=Lax"},
    )
