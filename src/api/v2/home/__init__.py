"""
首页API聚合路由器 - V2统一入口
包装 V1 core.home 模块，后续逐步将路由迁移为原生 V2 实现
"""
from fastapi import APIRouter

_router = None


def _build_router():
    global _router
    if _router is not None:
        return _router

    router = APIRouter(tags=["home"])

    # ── V1 模块（待逐步内联优化）──
    from src.api.v2.core.home import router as home_router
    router.include_router(home_router)

    _router = router
    return _router


def __getattr__(name):
    if name == "router":
        return _build_router()
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
