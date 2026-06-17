"""
用户设置 API - V2 优化版

优化: 去除 debug 日志泛滥, 统一响应格式, 移除重复 import
"""
from typing import Optional

from fastapi import Request, Depends, APIRouter, UploadFile, File
from fastapi.responses import JSONResponse
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User
from src.api.v2._base import ApiResponse
from src.api.v2._helpers import ok, fail
from src.api.v2.user_utils.user_entities import check_user_conflict, change_username, bind_email, db_save_bio, \
    save_uploaded_avatar
from src.auth import jwt_required_dependency as jwt_required
from src.utils.database.main import get_async_session as get_async_db
from src.utils.security.safe import valid_language_codes
from src.utils.send_email import request_email_change


class ProfileUpdateRequest(BaseModel):
    username: Optional[str] = None
    email: Optional[str] = None
    bio: Optional[str] = None
    profile_private: Optional[bool] = None
    locale: Optional[str] = None


class ChangeEmailRequest(BaseModel):
    email: str


class ChangePrivacyRequest(BaseModel):
    profile_private: bool = False


class ChangeLocaleRequest(BaseModel):
    locale: str = "zh_CN"


async def _get_user_or_error(db: AsyncSession, user_id: int) -> tuple[User | None, JSONResponse | None]:
    """获取用户对象, 不存在返回错误响应"""
    user = await db.scalar(select(User).where(User.id == user_id))
    if not user:
        return None, JSONResponse(content={"error": "用户不存在"}, status_code=404)
    return user, None


async def change_profiles_back(request: Request, user_id: int, cache_instance, domain: str, db: AsyncSession):
    data = await request.json()
    change_type = data.get('change_type')
    if not change_type:
        return JSONResponse(content={'error': 'Change type is required'}, status_code=400)
    if change_type not in ['avatar', 'username', 'email', 'password', 'bio', 'privacy', 'locale']:
        return JSONResponse(content={'error': 'Invalid change type'}, status_code=400)

    user, err = await _get_user_or_error(db, user_id)
    if err:
        return err

    if change_type == 'username':
        username = data.get('form_data', {}).get('username')
        if cache_instance.get(f'limit_username_lock_{user_id}'):
            return JSONResponse(content={'error': 'Cannot change username more than once a week'}, status_code=400)
        if check_user_conflict('username', username, db):
            return JSONResponse(content={'error': 'Username already exists'}, status_code=400)
        await change_username(user_id, username, db)
        cache_instance.set(f'limit_username_lock_{user_id}', True, ex=604800)
        return JSONResponse(content={'message': 'Username updated successfully'})

    elif change_type == 'email':
        email = data.get('form_data', {}).get('email')
        if check_user_conflict('email', email, db):
            return JSONResponse(content={'error': 'Email already exists'}, status_code=400)
        await request_email_change(user_id, cache_instance, domain, email)
        return JSONResponse(content={'message': 'Email updated successfully'})

    elif change_type == 'bio':
        await db_save_bio(user_id, data.get('form_data', {}).get('bio'), db)
        return JSONResponse(content={'message': 'Bio updated successfully'})

    elif change_type == 'privacy':
        user.profile_private = bool(data.get('profile_private', False))
        await db.commit()
        return JSONResponse(content={'message': 'Privacy settings updated successfully'})

    elif change_type == 'locale':
        locale = data.get('locale', 'zh_CN')
        if not valid_language_codes(locale):
            return JSONResponse(content={'error': 'Invalid locale'}, status_code=400)
        user.locale = locale
        await db.commit()
        return JSONResponse(content={'message': 'Locale updated successfully'})

    return await update_setting_profiles(request, change_type, user_id, db)


async def confirm_email_back(user_id: int, cache_instance, token: str, db: AsyncSession):
    temp_email_data = cache_instance.get(f"temp_email_{user_id}")
    if not temp_email_data or token != temp_email_data.get('token'):
        return JSONResponse(content={"error": "Invalid verification data"}, status_code=400)
    await bind_email(user_id, temp_email_data['new_email'], db)
    return {"status": "success", "message": "Email updated successfully"}


router = APIRouter(tags=["user-settings"])


@router.post("/profile/avatar")
async def update_avatar_api(file: UploadFile = File(...), current_user=Depends(jwt_required)):
    """更新用户头像（支持 JPG/PNG/WEBP, 最大 5MB）"""
    allowed_types = ['image/jpeg', 'image/png', 'image/webp', 'image/jpg']
    if file.content_type not in allowed_types:
        return JSONResponse(content={"success": False, "error": "不支持的文件类型"}, status_code=400)

    content = await file.read()
    if len(content) > 5 * 1024 * 1024:
        return JSONResponse(content={"success": False, "error": "文件大小不能超过5MB"}, status_code=400)

    await file.seek(0)
    from src.utils.database.main import get_async_session_context
    async with get_async_session_context() as db:
        result = await save_uploaded_avatar(file, current_user.id, db)

    avatar_url = f"/api/v2/static/avatar/{result}.webp"
    return JSONResponse(content={"success": True, "avatar_url": avatar_url})


@router.put("/profiles")
async def update_setting_profiles(
        request: Request,
        current_user=Depends(jwt_required),
        db: AsyncSession = Depends(get_async_db)
):
    """更新设置资料 - 用户名/简介/隐私/语言等"""
    from src.extensions import cache
    from src.setting import app_config
    return await change_profiles_back(
        request=request, user_id=current_user.id,
        cache_instance=cache, domain=app_config.domain, db=db)
