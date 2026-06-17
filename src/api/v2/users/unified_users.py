"""
用户模块 - V2 优化版
整合所有用户相关功能：资料管理、关注、屏蔽等

优化: 统一错误处理装饰器消除 22 处重复 try/except, 提取分页逻辑
"""
import os
from datetime import datetime
from functools import wraps
from typing import Optional, Callable, Any

from fastapi import APIRouter, Depends, Query, Request, HTTPException, UploadFile, File
from fastapi.responses import JSONResponse
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User, UserBlock
from shared.services.users.user_manager import user_csv_service
from src.api.v2._base import ApiResponse
from src.api.v2._helpers import ok, fail
from src.api.v2.user_utils.password_utils import validate_password_async, update_password
from src.api.v2.user_utils.user_entities import check_user_conflict_async, change_username, db_save_bio, \
    save_uploaded_avatar
from src.auth.auth_deps import admin_required as admin_required_api, jwt_required_dependency as jwt_required, \
    get_current_active_user
from src.extensions import cache
from src.utils.database.main import get_async_session as get_async_db
from src.setting import app_config
from src.utils.security.forms import ChangePasswordForm
from src.utils.security.ip_utils import get_client_ip
from src.utils.security.safe import is_valid_iso_language_code
from src.utils.send_email import request_email_change

# ---------------------------------------------------------------------------
# 内存关注/屏蔽数据库（保持与旧代码行为一致，后续应迁移到数据库表）
# ---------------------------------------------------------------------------
followers_db: dict = {}
follows_db: dict = {}
blocks_db: dict = {}

router = APIRouter(tags=["users"])


def _with_db(func: Callable) -> Callable:
    """统一错误处理装饰器：消除重复的 try/except/print(traceback) 模板"""
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except Exception as e:
            import traceback
            print(f"[{func.__name__}] {e}")
            traceback.print_exc()
            return fail(str(e))
    return wrapper


def _format_user_brief(user: User) -> dict:
    """统一用户简略信息格式"""
    return {
        "id": user.id, "username": user.username, "email": user.email,
        "is_active": user.is_active, "is_superuser": getattr(user, 'is_superuser', False),
        "created_at": user.date_joined.isoformat() if getattr(user, 'date_joined', None) else None,
        "last_login": user.last_login.isoformat() if getattr(user, 'last_login', None) else None,
    }


def _format_user_detail(user: User) -> dict:
    """统一用户详细信息格式"""
    avatar = None
    if user.profile_picture:
        safe = user.profile_picture.replace('\\', '/').split('/')[-1]
        avatar = f"/api/v2/static/avatar/{safe}.webp"
    return {
        "id": user.id, "username": user.username, "email": user.email,
        "avatar": avatar, "bio": user.bio, "profile_picture": user.profile_picture,
        "is_active": user.is_active, "is_superuser": user.is_superuser,
        "created_at": user.date_joined.isoformat() if getattr(user, 'date_joined', None) else None,
        "last_login": user.last_login.isoformat() if getattr(user, 'last_login', None) else None,
    }


# ==================== 管理员功能 ====================

@router.get("/")
@_with_db
async def get_users_list_api(
        page: int = Query(1, ge=1),
        per_page: int = Query(10, ge=1, le=100),
        search: str = Query(""),
        db: AsyncSession = Depends(get_async_db),
        _=Depends(admin_required_api)
):
    """用户列表（分页+搜索，仅管理员）"""
    filters = []
    if search:
        filters.append(User.username.contains(search) | User.email.contains(search))

    total = await db.scalar(select(func.count(User.id)).where(*filters)) or 0

    q = select(User).where(*filters).offset((page - 1) * per_page).limit(per_page)
    users = (await db.execute(q)).scalars().all()

    total_pages = max(1, (total + per_page - 1) // per_page)
    return ApiResponse(success=True, data=[_format_user_brief(u) for u in users], pagination={
        "current_page": page, "per_page": per_page, "total": total,
        "total_pages": total_pages, "has_next": page < total_pages, "has_prev": page > 1,
    })


@router.post("/import-csv")
@_with_db
async def import_users_csv_api(request: Request, db: AsyncSession = Depends(get_async_db),
                                _=Depends(admin_required_api)):
    """从 CSV 导入用户（仅管理员）"""
    form = await request.form()
    file = form.get('file')
    if not file:
        return fail("请上传 CSV 文件")
    content = await file.read()
    result = user_csv_service.import_users(content.decode('utf-8'), db)
    return ok(result, "CSV 导入完成")


@router.get("/export-csv")
@_with_db
async def export_users_csv_api(_=Depends(admin_required_api)):
    """导出用户 CSV（仅管理员）"""
    from fastapi.responses import StreamingResponse
    import io
    output = io.StringIO()
    user_csv_service.export_users(output)
    output.seek(0)
    return StreamingResponse(iter([output.getvalue()]), media_type="text/csv",
                             headers={"Content-Disposition": "attachment; filename=users.csv"})


@router.get("/download-sample-csv")
@_with_db
async def download_sample_csv_api():
    """下载示例 CSV 模板"""
    import io
    output = io.StringIO()
    user_csv_service.generate_sample(output)
    output.seek(0)
    from fastapi.responses import StreamingResponse
    return StreamingResponse(iter([output.getvalue()]), media_type="text/csv",
                             headers={"Content-Disposition": "attachment; filename=sample_users.csv"})


# ==================== 当前用户操作 ====================

@router.get("/me")
@_with_db
async def get_current_user_api(request: Request, current_user: User = Depends(get_current_active_user)):
    """当前用户信息（含令牌自动刷新）"""
    new_token = getattr(request.state, 'new_access_token', None)
    user_data = _format_user_detail(current_user)
    content = {"success": True, "data": user_data}
    if new_token:
        content["new_access_token"] = new_token

    resp = JSONResponse(content=content)
    if new_token:
        is_https = str(app_config.domain).startswith('https://') or os.environ.get('DEBUG', 'False').lower() == 'false'
        resp.set_cookie("access_token", new_token, httponly=True, secure=is_https,
                        samesite="strict", max_age=3600)
    return resp


@router.put("/me")
@_with_db
async def update_current_user_profile_api(
        request: Request, db: AsyncSession = Depends(get_async_db),
        current_user: User = Depends(get_current_active_user)):
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

    profile_private = data.get('profile_private')
    if profile_private is not None:
        current_user.profile_private = bool(profile_private)

    if any(k in data for k in ('locale', 'profile_private')):
        await db.commit()

    return ok({"user_id": current_user.id}, "资料更新成功")


@router.post("/me/change-password")
@_with_db
async def change_password_api(request: Request, db: AsyncSession = Depends(get_async_db),
                               current_user: User = Depends(get_current_active_user)):
    """修改当前用户密码"""
    # FastAPI 的 request 需要先解析 form 数据再传给 WTForms
    form_data = await request.form()
    form = ChangePasswordForm(form_data)
    if not form.validate():
        return fail("表单验证失败")

    if not await validate_password_async(current_user.id, form.current_password.data, db):
        return fail("原密码错误")

    await update_password(current_user.id, form.new_password.data, form.confirm_password.data, request.client.host if request.client else '', db)
    return ok(msg="密码修改成功")


@router.post("/me/avatar")
@_with_db
async def update_avatar_api(file: UploadFile = File(...),
                            current_user=Depends(jwt_required)):
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
@_with_db
async def get_user_settings_api(current_user: User = Depends(get_current_active_user)):
    """获取用户设置"""
    return ok({"locale": current_user.locale, "profile_private": current_user.profile_private})


@router.put("/me/settings")
@_with_db
async def update_user_settings_api(request: Request, db: AsyncSession = Depends(get_async_db),
                                    current_user: User = Depends(get_current_active_user)):
    """更新用户设置"""
    data = await request.json()
    locale = data.get('locale')
    if locale and is_valid_iso_language_code(locale):
        current_user.locale = locale
    if 'profile_private' in data:
        current_user.profile_private = bool(data['profile_private'])
    await db.commit()
    return ok(msg="设置更新成功")


# ==================== 关注/屏蔽（内存存储）====================

@router.get("/me/followers")
@_with_db
async def get_followers(current_user: User = Depends(get_current_active_user)):
    """我的粉丝列表"""
    fans = followers_db.get(current_user.id, {})
    return ok({"fans_list": [{'follower_id': int(k), 'created_at': datetime.fromtimestamp(v).isoformat()}
                              for k, v in sorted(fans.items(), key=lambda x: x[1], reverse=True)],
               "fans_count": len(fans)})


@router.get("/me/following")
@_with_db
async def get_following(current_user: User = Depends(get_current_active_user)):
    """我关注的用户列表"""
    following = follows_db.get(current_user.id, {})
    return ok({"following_list": [{'user_id': int(k), 'created_at': datetime.fromtimestamp(v).isoformat()}
                                   for k, v in sorted(following.items(), key=lambda x: x[1], reverse=True)],
               "following_count": len(following)})


@router.get("/me/blocked")
@_with_db
async def get_blocked_users(db: AsyncSession = Depends(get_async_db),
                            current_user: User = Depends(get_current_active_user)):
    """我屏蔽的用户列表"""
    rows = (await db.execute(
        select(UserBlock).where(UserBlock.user_id == current_user.id)
    )).scalars().all()
    return ok({"blocked_users": [{"user_id": r.blocked_user_id, "created_at": r.created_at.isoformat()
                                   if r.created_at else None} for r in rows],
               "blocked_count": len(rows)})


@router.post("/me/block/{user_id}")
@_with_db
async def block_user(user_id: int, db: AsyncSession = Depends(get_async_db),
                      current_user: User = Depends(get_current_active_user)):
    """屏蔽用户"""
    if user_id == current_user.id:
        return fail("不能屏蔽自己")
    existing = await db.scalar(
        select(UserBlock).where(UserBlock.user_id == current_user.id, UserBlock.blocked_user_id == user_id))
    if existing:
        return fail("用户已被屏蔽")
    db.add(UserBlock(user_id=current_user.id, blocked_user_id=user_id, created_at=datetime.now()))
    await db.commit()
    return ok(msg="屏蔽成功")


@router.delete("/me/block/{user_id}")
@_with_db
async def unblock_user(user_id: int, db: AsyncSession = Depends(get_async_db),
                        current_user: User = Depends(get_current_active_user)):
    """取消屏蔽"""
    row = await db.scalar(
        select(UserBlock).where(UserBlock.user_id == current_user.id, UserBlock.blocked_user_id == user_id))
    if not row:
        return fail("未屏蔽该用户")
    await db.delete(row)
    await db.commit()
    return ok(msg="已取消屏蔽")


# ==================== 用户公开信息 ====================

@router.get("/{user_id}")
@_with_db
async def get_user_profile_api(user_id: int, db: AsyncSession = Depends(get_async_db),
                                current_user: User = Depends(get_current_active_user)):
    """获取用户公开资料（需登录）"""
    user = await db.scalar(select(User).where(User.id == user_id))
    if not user:
        return fail("用户不存在")
    data = _format_user_detail(user)
    # 公开资料不暴露 email
    data.pop('email', None)
    return ok(data)


@router.post("/{user_id}/follow")
@_with_db
async def follow_user(user_id: int, current_user: User = Depends(get_current_active_user)):
    """关注用户"""
    if user_id == current_user.id:
        return fail("不能关注自己")
    now = datetime.now().timestamp()
    follows_db.setdefault(current_user.id, {})[user_id] = now
    followers_db.setdefault(user_id, {})[current_user.id] = now
    count = len(follows_db.get(current_user.id, {}))
    return ok({"following_count": count}, "关注成功")


@router.delete("/{user_id}/follow")
@_with_db
async def unfollow_user(user_id: int, current_user: User = Depends(get_current_active_user)):
    """取消关注"""
    follows_db.get(current_user.id, {}).pop(user_id, None)
    followers_db.get(user_id, {}).pop(current_user.id, None)
    count = len(follows_db.get(current_user.id, {}))
    return ok({"following_count": count}, "已取消关注")


@router.get("/{user_id}/followers")
@_with_db
async def get_user_followers(user_id: int, db: AsyncSession = Depends(get_async_db)):
    """指定用户的粉丝"""
    fans = followers_db.get(user_id, {})
    users = {}
    for fid in fans:
        u = await db.scalar(select(User).where(User.id == int(fid)))
        if u:
            users[fid] = _format_user_brief(u)
    return ok({"fans_list": [{"user": users.get(fid), "created_at": datetime.fromtimestamp(ts).isoformat()}
                              for fid, ts in sorted(fans.items(), key=lambda x: x[1], reverse=True)],
               "fans_count": len(fans)})


@router.get("/{user_id}/following")
@_with_db
async def get_user_following(user_id: int, db: AsyncSession = Depends(get_async_db)):
    """指定用户关注的人"""
    following = follows_db.get(user_id, {})
    users = {}
    for uid in following:
        u = await db.scalar(select(User).where(User.id == int(uid)))
        if u:
            users[uid] = _format_user_brief(u)
    return ok({"following_list": [{"user": users.get(uid), "created_at": datetime.fromtimestamp(ts).isoformat()}
                                   for uid, ts in sorted(following.items(), key=lambda x: x[1], reverse=True)],
               "following_count": len(following)})


@router.get("/{user_id}/activity")
@_with_db
async def get_user_activity(user_id: int):
    """用户动态（暂无实现）"""
    return ok({"activities": []})


@router.get("/{user_id}/interests")
@_with_db
async def get_user_interests(user_id: int):
    """用户兴趣（暂无实现）"""
    return ok({"interests": []})


@router.get("/recommendations")
@_with_db
async def recommend_users(_=Depends(get_current_active_user)):
    """推荐用户（暂无实现）"""
    return ok({"users": []})
