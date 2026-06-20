"""
用户模块 — 精简版（仅保留网盘必需的资料管理功能）
已移除：关注/粉丝/屏蔽/推荐/活动/兴趣 等博客社交功能
"""
import os
from datetime import datetime

from fastapi import APIRouter, Depends, Request, UploadFile, File
from fastapi.responses import JSONResponse
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User
from src.api.v2._helpers import ok, fail
from src.api.v2.auth_v1pack import get_current_user
from src.api.v2.user_utils.password_utils import validate_password_async, update_password
from src.api.v2.user_utils.user_entities import check_user_conflict_async, change_username, db_save_bio, \
    save_uploaded_avatar
from src.setting import app_config
from src.utils.database.main import get_async_session as get_async_db
from src.utils.security.forms import ChangePasswordForm
from src.utils.security.safe import is_valid_iso_language_code

router = APIRouter(tags=["users"])


def _format_user_detail(user: User) -> dict:
    """用户详细信息"""
    return {
        "id": user.id, "username": user.username, "email": user.email,
        "bio": user.bio, "profile_picture": user.profile_picture,
        "is_active": user.is_active, "is_superuser": user.is_superuser,
        "created_at": user.date_joined.isoformat() if getattr(user, 'date_joined', None) else None,
        "last_login": user.last_login.isoformat() if getattr(user, 'last_login', None) else None,
    }


# ==================== 当前用户操作 ====================

@router.get("/me")
async def get_current_user_api(request: Request, current_user: User = Depends(get_current_user)):
    """当前用户信息"""
    user_data = _format_user_detail(current_user)
    return JSONResponse(content={"success": True, "data": user_data})


@router.put("/me")
async def update_current_user_profile_api(
    request: Request, db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(get_current_user)):
    """更新当前用户资料"""
    data = await request.json()
    username = data.get('username')
    if username and username != current_user.username:
        conflict = await check_user_conflict_async('username', username, db)
        if conflict:
            return fail("用户名已被使用")
        await change_username(current_user.id, username, db)
    bio = data.get('bio')
    if bio is not None:
        await db_save_bio(current_user.id, bio, db)
    locale = data.get('locale')
    if locale and is_valid_iso_language_code(locale):
        current_user.locale = locale
        await db.commit()
    return ok({"user_id": current_user.id}, "资料更新成功")


@router.post("/me/change-password")
async def change_password_api(request: Request, db: AsyncSession = Depends(get_async_db),
                              current_user: User = Depends(get_current_user)):
    """修改当前用户密码"""
    form_data = await request.form()
    form = ChangePasswordForm(form_data)
    if not form.validate():
        return fail("表单验证失败")
    if not await validate_password_async(current_user.id, form.current_password.data, db):
        return fail("原密码错误")
    await update_password(current_user.id, form.new_password.data, form.confirm_password.data,
                          request.client.host if request.client else '', db)
    return ok(msg="密码修改成功")


@router.post("/me/avatar")
async def update_avatar_api(file: UploadFile = File(...),
                            current_user=Depends(get_current_user)):
    """更新头像（JPG/PNG/WEBP, max 5MB）"""
    if file.content_type not in ('image/jpeg', 'image/png', 'image/webp', 'image/jpg'):
        return fail("不支持的文件类型")
    content = await file.read()
    if len(content) > 5 * 1024 * 1024:
        return fail("文件大小不能超过 5MB")
    await file.seek(0)
    from src.utils.database.main import get_async_session_context
    async with get_async_session_context() as db:
        result = await save_uploaded_avatar(file, current_user.id, db)
    return ok({"avatar_url": f"/api/v2/static/avatar/{result}.webp"}, "头像更新成功")


@router.get("/me/settings")
async def get_user_settings_api(current_user: User = Depends(get_current_user)):
    """获取用户设置"""
    return ok({"locale": current_user.locale, "profile_private": current_user.profile_private})


@router.put("/me/settings")
async def update_user_settings_api(request: Request, db: AsyncSession = Depends(get_async_db),
                                   current_user: User = Depends(get_current_user)):
    """更新用户设置"""
    data = await request.json()
    locale = data.get('locale')
    if locale and is_valid_iso_language_code(locale):
        current_user.locale = locale
    if 'profile_private' in data:
        current_user.profile_private = bool(data['profile_private'])
    await db.commit()
    return ok(msg="设置更新成功")


# ==================== 用户公开信息 ====================

@router.get("/{user_id}")
async def get_user_profile_api(user_id: int, db: AsyncSession = Depends(get_async_db),
                               current_user: User = Depends(get_current_user)):
    """获取用户公开资料"""
    user = await db.scalar(select(User).where(User.id == user_id))
    if not user:
        return fail("用户不存在")
    data = _format_user_detail(user)
    data.pop('email', None)
    return ok(data)
