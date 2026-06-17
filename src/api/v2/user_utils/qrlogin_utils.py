import base64
import hashlib
import io
import secrets
import time

import qrcode
from fastapi import Request

# 内存缓存用于 QR 登录令牌（避免同步 Redis 操作阻塞事件循环）
_qr_cache: dict[str, dict] = {}
_qr_cache_expiry: dict[str, float] = {}


def _cache_set(key: str, value, ttl: int = 180):
    """内存缓存写入 QR 令牌"""
    _qr_cache[key] = value if isinstance(value, dict) else {"data": value}
    _qr_cache_expiry[key] = time.time() + ttl


def _cache_get(key: str):
    """内存缓存读取 QR 令牌，自动清理过期项"""
    expiry = _qr_cache_expiry.get(key)
    if expiry and time.time() > expiry:
        _qr_cache.pop(key, None)
        _qr_cache_expiry.pop(key, None)
        return None
    return _qr_cache.get(key)


def gen_qr_token(user_agent: str, timestamp: str, sys_version: str, encoding: str = "utf-8") -> str:
    """生成随机的二维码 token"""
    data = f"{user_agent}{timestamp}{sys_version}{secrets.token_hex(16)}"
    return hashlib.sha256(data.encode(encoding)).hexdigest()


def qr_login(request: Request, sys_version: str, global_encoding: str, domain: str, cache=None):
    """生成登录二维码，返回 base64 图片与 token（同步函数，使用内存缓存）"""
    ct = str(int(time.time()))
    user_agent = request.headers.get("User-Agent", "Unknown")

    if not domain.startswith(("http://", "https://")):
        # 优先使用 Origin 请求头（前端页面发出的请求会自动携带）
        origin = request.headers.get("Origin") or request.headers.get("Referer") or ""
        if origin and origin.startswith(("http://", "https://")):
            full_domain = origin.rstrip("/") + "/"
        else:
            # 回退到 Host 头（可能返回后端地址）
            host = request.headers.get("host", "localhost:4321")
            scheme = request.url.scheme if hasattr(request.url, "scheme") else "http"
            full_domain = f"{scheme}://{host}/"
    else:
        full_domain = domain

    token = gen_qr_token(user_agent, ct, sys_version, global_encoding)
    qr_data = f"{full_domain}mobile-login?login_token={token}"

    # 生成二维码图片
    img = qrcode.make(qr_data)
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    qr_b64 = base64.b64encode(buf.getvalue()).decode(global_encoding)

    # 内存缓存待扫码状态
    _cache_set(f"QR-token_{token}", {"status": "pending", "created_at": ct}, ttl=180)

    return {
        "success": True,
        "qr_code": f"data:image/png;base64,{qr_b64}",
        "token": token,
        "expires_at": str(int(time.time()) + 180),
    }


async def phone_scan_back(request: Request, current_user, cache=None, login_token: str = None):
    """手机扫码确认，将当前用户的 refresh_token 写入缓存"""
    token = login_token or request.query_params.get("login_token") or request.query_params.get("token")
    if not token:
        return {"success": False, "message": "Missing login token"}

    cached = _cache_get(f"QR-token_{token}")
    if not cached:
        return {"success": False, "message": "Invalid or expired token"}

    refresh_token = request.cookies.get("refresh_token")
    if not refresh_token:
        from src.api.v2.auth_v1pack import create_jwt_token
        refresh_token = create_jwt_token(subject=str(current_user.id), token_type="refresh")

    allow_data = {
        "status": "success",
        "refresh_token": refresh_token,
        "user_id": current_user.id,
    }
    _cache_set(f"QR-allow_{token}", allow_data, ttl=180)
    _cache_set(f"QR-token_{token}", {"status": "success"}, ttl=180)

    return {"success": True, "message": "授权成功，请在两分钟内完成登录"}


async def check_qr_login_back(request: Request, cache=None):
    """PC 端轮询检查扫码状态"""
    token = request.query_params.get("token")
    next_url = request.query_params.get("next", "/profile")

    if not token:
        return {"success": False, "status": "error", "message": "Missing token parameter"}

    qr_status = _cache_get(f"QR-token_{token}")
    if not qr_status:
        return {"success": True, "status": "expired", "message": "QR code expired"}

    if isinstance(qr_status, dict):
        status = qr_status.get("status", "pending")
    else:
        status = "success" if "success" in str(qr_status) else "pending"

    if status == "success":
        import asyncio
        allow_info = _cache_get(f"QR-allow_{token}")
        if not allow_info:
            await asyncio.sleep(0.5)
            allow_info = _cache_get(f"QR-allow_{token}")
        if allow_info and isinstance(allow_info, dict):
            return {
                "success": True,
                "status": "success",
                "next_url": next_url,
                "refresh_token": allow_info.get("refresh_token", ""),
            }

    return {"success": True, "status": status}
