"""
Stora 错误处理 — 错误码枚举 + 统一异常类
"""
from __future__ import annotations

from typing import Any, Optional

from fastapi import HTTPException
from fastapi.responses import JSONResponse


class ErrorCode:
    """错误码常量"""
    # 通用 (1xxx)
    SUCCESS = 0
    UNKNOWN_ERROR = 1000
    VALIDATION_ERROR = 1001
    INTERNAL_ERROR = 1002

    # 认证 (2xxx)
    NOT_AUTHENTICATED = 2000
    INVALID_TOKEN = 2001
    TOKEN_EXPIRED = 2002
    PERMISSION_DENIED = 2003
    INVALID_CREDENTIALS = 2004
    USER_DISABLED = 2005

    # 资源 (3xxx)
    NOT_FOUND = 3000
    FILE_NOT_FOUND = 3001
    FOLDER_NOT_FOUND = 3002
    USER_NOT_FOUND = 3003
    SHARE_NOT_FOUND = 3004
    SHARE_EXPIRED = 3005
    SHARE_LIMIT_EXCEEDED = 3006

    # 文件操作 (4xxx)
    UPLOAD_FAILED = 4000
    DOWNLOAD_FAILED = 4001
    QUOTA_EXCEEDED = 4002
    FILENAME_INVALID = 4003
    FILE_TYPE_BLOCKED = 4004
    FILE_TOO_LARGE = 4005
    CHUNK_MISSING = 4006
    HASH_MISMATCH = 4007
    PATH_TRAVERSAL = 4008

    # 业务 (5xxx)
    DUPLICATE_ENTRY = 5000
    RATE_LIMITED = 5001
    OPERATION_FAILED = 5002

    _messages: dict[int, str] = {
        SUCCESS: "成功",
        UNKNOWN_ERROR: "未知错误",
        VALIDATION_ERROR: "参数校验失败",
        INTERNAL_ERROR: "服务器内部错误",
        NOT_AUTHENTICATED: "未登录",
        INVALID_TOKEN: "无效的令牌",
        TOKEN_EXPIRED: "令牌已过期",
        PERMISSION_DENIED: "权限不足",
        INVALID_CREDENTIALS: "用户名或密码错误",
        USER_DISABLED: "账号已被禁用",
        NOT_FOUND: "资源不存在",
        FILE_NOT_FOUND: "文件不存在",
        FOLDER_NOT_FOUND: "文件夹不存在",
        USER_NOT_FOUND: "用户不存在",
        SHARE_NOT_FOUND: "分享链接不存在",
        SHARE_EXPIRED: "分享链接已过期",
        SHARE_LIMIT_EXCEEDED: "分享下载次数已达上限",
        UPLOAD_FAILED: "上传失败",
        DOWNLOAD_FAILED: "下载失败",
        QUOTA_EXCEEDED: "存储空间不足",
        FILENAME_INVALID: "文件名不合法",
        FILE_TYPE_BLOCKED: "不支持的文件类型",
        FILE_TOO_LARGE: "文件过大",
        CHUNK_MISSING: "分片缺失",
        HASH_MISMATCH: "文件哈希不匹配",
        PATH_TRAVERSAL: "路径穿越检测",
        DUPLICATE_ENTRY: "记录已存在",
        RATE_LIMITED: "请求过于频繁",
        OPERATION_FAILED: "操作失败",
    }

    @classmethod
    def message(cls, code: int) -> str:
        return cls._messages.get(code, "未知错误")


class StoraException(HTTPException):
    """统一业务异常"""

    def __init__(
        self,
        code: int = ErrorCode.UNKNOWN_ERROR,
        message: Optional[str] = None,
        status_code: int = 400,
        data: Any = None,
    ):
        self.code = code
        self.message = message or ErrorCode.message(code)
        self.data = data
        super().__init__(status_code=status_code, detail=self.message)

    def to_dict(self) -> dict:
        return {
            "success": False,
            "code": self.code,
            "message": self.message,
            "data": self.data,
        }


def ok(data: Any = None, message: str = "") -> dict:
    """统一成功响应"""
    return {
        "success": True,
        "code": ErrorCode.SUCCESS,
        "message": message or None,
        "data": data,
    }


def fail(code: int = ErrorCode.UNKNOWN_ERROR, message: Optional[str] = None) -> dict:
    """统一失败响应 (用于非异常场景)"""
    return {
        "success": False,
        "code": code,
        "message": message or ErrorCode.message(code),
        "data": None,
    }
