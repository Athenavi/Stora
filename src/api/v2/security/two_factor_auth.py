"""
双因素认证(2FA)管理API
"""
import logging
from functools import wraps

from fastapi import APIRouter, Body, Depends, HTTPException, Request
from pydantic import BaseModel
from sqlalchemy.ext.asyncio import AsyncSession

from shared.services.users.login_security_service import login_security_service
from shared.services.users.session_management_service import session_management_service
from shared.services.users.two_factor_auth import two_factor_auth
from src.api.v2._helpers import ok, fail
from src.api.v2.auth_v1pack import create_jwt_token

# 2FA 设置密钥临时存储（cache 的后备，避免同步 cache.set 阻塞事件循环）
# 注意: 此内存 dict 在多进程部署下会丢失，如需生产环境高可用，
#       应使用 Redis 等共享存储作为主要存储，此 dict 仅作为后备。
_setup_secrets: dict = {}
logger = logging.getLogger(__name__)
logger.warning(
    "2FA setup secrets stored in process-local memory dict (_setup_secrets). "
    "Multi-process/load-balanced deployments will lose setup state across workers. "
    "Consider using Redis or database-backed storage for production."
)
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["2fa"])


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"[{func.__name__}] {e}")
            return fail(str(e))
    return wrapper


class Enable2FARequest(BaseModel):
    """启用2FA请求"""
    totp_token: str  # TOTP验证码用于验证


class Verify2FALoginRequest(BaseModel):
    """2FA登录验证请求"""
    user_id: int
    token: str  # TOTP或备用码


@router.get("/setup")
@_catch
async def setup_2fa(
        request: Request,
        db: AsyncSession = Depends(get_async_db),
        current_user=Depends(jwt_required)
):
    """
    设置2FA - 生成密钥和QR码

    返回QR码供用户扫描,以及手动输入的密钥
    """
    from shared.models.user import User
    from sqlalchemy import select

    # 获取当前用户
    query = select(User).where(User.id == current_user.id)
    result = await db.execute(query)
    user = result.scalar_one_or_none()

    if not user:
        return fail("用户不存在")

    if user.is_2fa_enabled:
        return fail("2FA已启用")

    # 生成新的TOTP密钥
    secret = two_factor_auth.generate_totp_secret()

    # 临时存储到session或缓存(不保存到数据库,直到验证成功)
    # 注意: cache.set() 是同步调用，在异步上下文中会阻塞事件循环
    # 使用简单的内存 dict 作为可靠后备
    from src.extensions import cache as maybe_cache
    try:
        maybe_cache.set(f"2fa_setup:{current_user.id}", secret, ex=600)
    except Exception:
        pass
    # 始终保存到内存后备，确保 enable 时可读取
    _setup_secrets[current_user.id] = secret

    # 生成QR码
    qr_data = two_factor_auth.generate_qr_code(
        secret=secret,
        username=user.username,
        email=user.email
    )

    return ok(data={
        "qr_code": qr_data['qr_code'],
        "secret": qr_data['manual_entry_key'],
        "instructions": [
            "1. 下载Google Authenticator或类似应用",
            "2. 扫描二维码或手动输入密钥",
            "3. 输入应用生成的6位验证码以完成设置"
        ]
    }, msg="请扫描二维码或手动输入密钥")


@router.post("/enable")
@_catch
async def enable_2fa(
        request_data: Enable2FARequest,
        db: AsyncSession = Depends(get_async_db),
        current_user=Depends(jwt_required)
):
    """
    启用2FA

    需要用户提供TOTP验证码来验证设置正确
    """
    from shared.models.user import User
    from sqlalchemy import select
    from src.extensions import cache

    # 获取临时存储的密钥（优先缓存，降级内存）
    from src.extensions import cache
    cache_key = f"2fa_setup:{current_user.id}"
    secret = _setup_secrets.get(current_user.id) or cache.get(cache_key)

    if not secret:
        return fail("设置已过期,请重新开始")

    # 验证TOTP令牌
    if not two_factor_auth.verify_totp(secret, request_data.totp_token):
        return fail("验证码错误")

    # 生成备用码
    backup_codes = two_factor_auth.generate_backup_codes()

    # 在数据库中启用2FA
    query = select(User).where(User.id == current_user.id)
    result = await db.execute(query)
    user = result.scalar_one_or_none()

    if not user:
        return fail("用户不存在")

    user.is_2fa_enabled = True
    user.totp_secret = secret
    user.backup_codes = two_factor_auth.hash_backup_codes(backup_codes)

    # 特殊：保留 db.rollback() 处理
    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise

    # 清除临时缓存和内存后备
    cache.delete(cache_key)
    _setup_secrets.pop(current_user.id, None)

    return ok(data={
        "backup_codes": backup_codes,
        "message": "请务必保存这些备用码,它们只能使用一次!"
    }, msg="2FA已成功启用")


@router.post("/disable")
@_catch
async def disable_2fa(
        password: str = Body(..., description="当前密码验证"),
        db: AsyncSession = Depends(get_async_db),
        current_user=Depends(jwt_required)
):
    """
    禁用2FA（需验证当前密码）
    """
    from shared.models.user import User
    from sqlalchemy import select
    from src.utils.security.password_validator import verify_password

    # 获取用户信息
    query = select(User).where(User.id == current_user.id)
    result = await db.execute(query)
    user = result.scalar_one_or_none()

    if not user:
        return fail("用户不存在")

    # 验证密码
    if not user.password or not verify_password(password, user.password):
        return fail("密码验证失败")

    # 清除2FA设置
    user.is_2fa_enabled = False
    user.totp_secret = None
    user.backup_codes = None

    # 特殊：保留 db.rollback() 处理
    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise

    return ok(msg="2FA已禁用")


@router.post("/verify-login")
@_catch
async def verify_2fa_login(
        request: Request,
        request_data: Verify2FALoginRequest,
        db: AsyncSession = Depends(get_async_db)
):
    """
    验证2FA登录

    在用户输入用户名密码后,如果启用了2FA,需要调用此接口
    验证成功后返回正式的access_token和refresh_token
    """
    from shared.models.user import User
    from sqlalchemy import select
    from datetime import datetime, timezone, timedelta

    # 获取设备信息（在try块开头定义，确保后续可用）
    user_agent = request.headers.get("User-Agent", "Unknown")
    ip_address = request.client.host if request.client else None

    # 获取用户信息
    query = select(User).where(User.id == request_data.user_id)
    query_result = await db.execute(query)
    user = query_result.scalar_one_or_none()

    if not user:
        return fail("用户不存在")

    if not user.is_2fa_enabled:
        return fail("2FA未启用")

    # 验证TOTP或备用码
    verification_method = None

    # 首先尝试TOTP验证
    if user.totp_secret and two_factor_auth.verify_totp(user.totp_secret, request_data.token):
        verification_method = 'totp'
    # 然后尝试备用码验证
    elif user.backup_codes:
        import json
        import hashlib
        try:
            hashed_codes = json.loads(user.backup_codes)
            input_hashed = hashlib.sha256(request_data.token.encode('utf-8')).hexdigest()

            if input_hashed in hashed_codes:
                # 移除已使用的备用码并保存
                hashed_codes.remove(input_hashed)
                user.backup_codes = json.dumps(hashed_codes)
                verification_method = 'backup_code'
        except Exception as e:
            print(f"Backup code verification error: {e}")

    if not verification_method:
        return fail("验证码错误")

    if not user.is_active:
        return fail("账户已被禁用")

    # 生成正式的JWT token
    access_token = create_jwt_token(subject=str(user.id), token_type="access")
    refresh_token = create_jwt_token(subject=str(user.id), token_type="refresh")

    # 创建用户会话记录
    try:
        device_info = {
            'user_agent': user_agent,
            'browser': request.headers.get('sec-ch-ua', 'Unknown'),
            'platform': request.headers.get('sec-ch-ua-platform', 'Unknown'),
        }
        session_management_service.create_session(
            user_id=user.id,
            device_info=device_info,
            ip_address=ip_address,
            user_agent=user_agent,
        )
        print(f"[2FA Verify] Session created for user {user.id}")
    except Exception as e:
        print(f"[2FA Verify] Warning: Failed to create session: {e}")

    # 更新最后登录时间
    user.last_login = datetime.now(timezone.utc)
    await db.commit()

    # 记录成功登录
    try:
        await login_security_service.record_login_attempt_async(
            username=user.username,
            ip_address=ip_address or "unknown",
            user_agent=user_agent,
            is_success=True,
            db=db
        )
        await login_security_service.clear_failed_attempts_async(user.username, db)
    except Exception as e:
        print(f"Failed to record login attempt: {e}")

    return ok(data={
        "user": {
            "id": user.id,
            "username": user.username,
            "email": user.email,
            "profile_picture": user.profile_picture or None,
            "is_active": user.is_active,
            "is_superuser": user.is_superuser,
            "is_staff": user.is_staff,
            "vip_level": getattr(user, "vip_level", 0),
        },
        "access_token": access_token,
        "refresh_token": refresh_token,
        "method": verification_method,
        "message": "2FA验证成功，登录完成"
    })


@router.post("/backup-codes/regenerate")
@_catch
async def regenerate_backup_codes(
        db: AsyncSession = Depends(get_async_db),
        current_user=Depends(jwt_required)
):
    """
    重新生成备用码
    """
    from shared.models.user import User
    from sqlalchemy import select

    # 获取用户信息
    query = select(User).where(User.id == current_user.id)
    result = await db.execute(query)
    user = result.scalar_one_or_none()

    if not user:
        return fail("用户不存在")

    if not user.is_2fa_enabled:
        return fail("2FA未启用")

    # 生成新的备用码
    new_codes = two_factor_auth.generate_backup_codes()
    user.backup_codes = two_factor_auth.hash_backup_codes(new_codes)

    # 特殊：保留 db.rollback() 处理
    try:
        await db.commit()
    except Exception:
        await db.rollback()
        raise

    return ok(data={
        "backup_codes": new_codes,
        "message": "旧备用码已失效,请保存新备用码"
    }, msg="备用码已重新生成")


@router.get("/status")
@_catch
async def get_2fa_status(
        db: AsyncSession = Depends(get_async_db),
        current_user=Depends(jwt_required)
):
    """
    获取2FA状态
    """
    from shared.models.user import User
    from sqlalchemy import select

    query = select(User).where(User.id == current_user.id)
    result = await db.execute(query)
    user = result.scalar_one_or_none()

    if not user:
        return fail("用户不存在")

    return ok(data={
        "is_2fa_enabled": user.is_2fa_enabled,
        "has_backup_codes": bool(user.backup_codes),
        "username": user.username,
        "email": user.email
    })
