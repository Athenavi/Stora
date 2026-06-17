"""
V2 共享路由辅助工具

提供统一错误处理、响应构造等模式，减少所有 V2 路由文件的重复代码。
"""
from functools import wraps
from typing import Any, Optional, Callable, Awaitable, TypeVar

from fastapi import HTTPException
from fastapi.responses import JSONResponse

from src.api.v2._base import ApiResponse

T = TypeVar('T')


def ok(data: Any = None, msg: str = "") -> ApiResponse:
    """构造成功响应"""
    return ApiResponse(success=True, data=data, message=msg or None)


def fail(msg: str = "操作失败") -> ApiResponse:
    """构造失败响应"""
    return ApiResponse(success=False, error=msg)


def _catch(func):
    """路由异常捕获装饰器：自动将 HTTPException 与未预期异常转为 JSON 响应"""
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


async def call(
    coro: Awaitable[dict],
    error_prefix: str = "操作",
    extract: Optional[Callable[[dict], Any]] = None,
) -> ApiResponse:
    """
    执行 service 调用，统一处理异常和响应包装

    用法:
        return await call(service.do_something(db, id), "用户创建")

    支持从 service 返回的 dict 中提取 data（默认返回整个 result）:
        return await call(service.do_something(), extract=lambda r: r.get('data'))
    """
    try:
        result = await coro
        if isinstance(result, dict) and result.get('success') is False:
            return fail(result.get('error', f'{error_prefix}失败'))
        data = extract(result) if extract else result
        return ok(data=data, msg=f'{error_prefix}成功')
    except Exception as e:
        return fail(f'{error_prefix}失败: {e}')
