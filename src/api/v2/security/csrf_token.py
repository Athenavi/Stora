"""
CSRF Token API
提供 CSRF token 的获取接口，用于前端安全防护
"""
import logging
from functools import wraps

from fastapi import APIRouter, Depends, HTTPException, Request

from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import jwt_required_dependency as jwt_required

router = APIRouter(tags=["csrf"])
logger = logging.getLogger(__name__)


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


@router.get("/csrf-token", summary="获取 CSRF token")
@_catch
async def get_csrf_token(request: Request, current_user=Depends(jwt_required)):
    """获取 CSRF token"""
    import secrets
    from src.extensions import cache

    token = secrets.token_urlsafe(32)
    # Store in cache for 2 hours
    cache.set(f"csrf_token:{token}", str(current_user.id), ex=7200)

    return ok(data={
        "csrf_token": token,
        "expires_in": 7200,
    })
