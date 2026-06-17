"""
Stora 全局异常处理器 — 捕获 StoraException、ValidationError、未预期异常
"""
import traceback
from datetime import datetime

from fastapi import Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from src.utils.errors import StoraException, ErrorCode


async def stora_exception_handler(request: Request, exc: StoraException) -> JSONResponse:
    """处理 StoraException"""
    return JSONResponse(
        status_code=exc.status_code,
        content=exc.to_dict(),
    )


async def validation_exception_handler(request: Request, exc: RequestValidationError) -> JSONResponse:
    """处理请求参数校验错误"""
    errors = []
    for err in exc.errors():
        errors.append({
            "field": ".".join(str(l) for l in err.get("loc", [])),
            "message": err.get("msg", ""),
            "type": err.get("type", ""),
        })
    return JSONResponse(
        status_code=422,
        content={
            "success": False,
            "code": ErrorCode.VALIDATION_ERROR,
            "message": "参数校验失败",
            "data": {"errors": errors},
        },
    )


async def http_exception_handler(request: Request, exc: StarletteHTTPException) -> JSONResponse:
    """处理 HTTPException"""
    return JSONResponse(
        status_code=exc.status_code,
        content={
            "success": False,
            "code": ErrorCode.UNKNOWN_ERROR,
            "message": exc.detail or ErrorCode.message(ErrorCode.UNKNOWN_ERROR),
            "data": None,
        },
    )


async def general_exception_handler(request: Request, exc: Exception) -> JSONResponse:
    """处理未预期的异常 (500)"""
    # Log the error
    print(f"[ERROR] Unhandled exception: {type(exc).__name__}: {exc}")
    traceback.print_exc()

    return JSONResponse(
        status_code=500,
        content={
            "success": False,
            "code": ErrorCode.INTERNAL_ERROR,
            "message": ErrorCode.message(ErrorCode.INTERNAL_ERROR),
            "data": None,
        },
    )


def register_error_handlers(app):
    """在 FastAPI 应用中注册所有异常处理器"""
    app.add_exception_handler(StoraException, stora_exception_handler)
    app.add_exception_handler(RequestValidationError, validation_exception_handler)
    app.add_exception_handler(StarletteHTTPException, http_exception_handler)
    app.add_exception_handler(Exception, general_exception_handler)
    print("[Stora] 全局异常处理器已注册")
