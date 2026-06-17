"""
Stora Mobile Users API — profile for mobile
"""
from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import User
from src.auth import jwt_required
from src.api.v2._helpers import ok
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["mobile-users"])


@router.get("/profile")
async def mobile_profile(
    current_user: dict = Depends(jwt_required),
    db: AsyncSession = Depends(get_async_db),
):
    """移动端用户资料"""
    user = await db.get(User, int(current_user.get("sub", 0)))
    if not user:
        return ok(None)
    return ok({"id": user.id, "username": user.username, "email": user.email, "bio": user.bio})
