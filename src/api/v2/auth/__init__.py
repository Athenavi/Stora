"""Stora Auth API Router"""
from fastapi import APIRouter

_router = None


def _build_router():
    global _router
    if _router is not None:
        return _router

    router = APIRouter(tags=["auth"])

    # Login/Register/Profile
    from src.api.v2.auth.login import router as login_router
    router.include_router(login_router)

    # OAuth
    from src.api.v2.auth.oauth import router as oauth_router
    router.include_router(oauth_router)

    # User CRUD (from existing module)
    from src.api.v2.users.unified_users import router as users_router
    router.include_router(users_router)

    _router = router
    return _router


def __getattr__(name):
    if name == "router":
        return _build_router()
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
