"""
V2 共享路由辅助工具

向后兼容包装 — 实际实现在 src/utils/errors.py
"""
from functools import wraps
from typing import Any, Optional, Callable, Awaitable, TypeVar

from fastapi import HTTPException
from fastapi.responses import JSONResponse

from src.api.v2._base import ApiResponse
from src.utils.errors import ok, fail, StoraException, ErrorCode

T = TypeVar('T')

# 导出旧接口保持兼容
__all__ = ['ok', 'fail', 'StoraException', 'ErrorCode', 'ApiResponse']


def _catch(func: Callable = None):
    """
    路由异常捕获装饰器：自动将 HTTPException 与未预期异常转为 JSON 响应

    支持 @_catch 和 @_catch() 两种用法
    """
    if func is None:
        return _catch

    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException as e:
            return JSONResponse(
                status_code=e.status_code,
                content={"success": False, "error": e.detail},
            )
        except Exception as e:
            return JSONResponse(
                status_code=500,
                content={"success": False, "error": f"服务器内部错误: {str(e)}"},
            )

    return wrapper
