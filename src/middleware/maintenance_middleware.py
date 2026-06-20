"""
维护模式中间件 — 处于维护模式时非管理员请求返回 503
"""
from fastapi import Request
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

from shared.services.system.maintenance_mode import maintenance_service


class MaintenanceMiddleware(BaseHTTPMiddleware):
    """维护模式中间件：非管理员请求返回 503"""

    # 维护模式下仍然可访问的路径前缀
    PUBLIC_PATHS = [
        "/api/v2/system/maintenance/public-status",
        "/api/v2/system/maintenance/status",
        "/api/v2/health",
        "/api/v2/auth/login",
    ]

    async def dispatch(self, request: Request, call_next):
        # 跳过静态文件和 OPTIONS 预检请求
        if request.method == "OPTIONS":
            return await call_next(request)

        path = request.url.path

        # 检查是否为公开路径
        for prefix in self.PUBLIC_PATHS:
            if path.startswith(prefix):
                return await call_next(request)

        # 检查维护模式
        client_ip = request.client.host if request.client else None
        if maintenance_service.is_maintenance_mode(client_ip):
            # 检查用户是否为管理员（通过 Authorization header 简单判断）
            # 完全验证在 auth 层处理，这里仅做快速检查
            auth = request.headers.get("Authorization", "")
            if auth.startswith("Bearer "):
                # 有 token 的请求交由后续中间件和路由处理
                # 实际权限检查在路由内部
                return await call_next(request)

            config = maintenance_service.load_config()
            return JSONResponse(
                status_code=503,
                content={
                    "success": False,
                    "message": config.get("message", "系统正在维护中，请稍后访问"),
                    "code": "MAINTENANCE_MODE",
                },
                headers={
                    "Retry-After": str(config.get("retry_after", 3600)),
                },
            )

        return await call_next(request)
