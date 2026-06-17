"""
Stora Dashboard API — netdisk overview (aggregator)
"""
from fastapi import APIRouter

_router = None


def _build_router():
    global _router
    if _router is not None:
        return _router

    router = APIRouter(tags=["dashboard"])

    from src.api.v2.dashboard.dashboard import router as dashboard_router
    router.include_router(dashboard_router, prefix="")

    _router = router
    return _router


def __getattr__(name):
    if name == "router":
        return _build_router()
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
