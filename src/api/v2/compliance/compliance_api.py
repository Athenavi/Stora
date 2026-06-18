"""
Stora Compliance API — GDPR compliance management.
"""
from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func

from shared.models import GDPRConsent, User
from src.api.v2._helpers import ok
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["compliance"])


@router.get("/compliance/consents")
async def list_consents(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """List GDPR consents for current user."""
    consents = (await db.execute(
        select(GDPRConsent).where(GDPRConsent.user_id == current_user.id)
    )).scalars().all()
    return ok([
        {
            "id": c.id,
            "consent_type": c.consent_type,
            "granted": c.granted,
            "created_at": str(c.created_at) if c.created_at else None,
        }
        for c in consents
    ])
