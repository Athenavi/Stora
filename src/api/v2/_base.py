"""
V2 共享基础模型
解耦 V1 的 ApiResponse/分页模型，供 V2 所有模块统一使用
"""
from typing import Any, Optional

from pydantic import BaseModel


class ApiResponse(BaseModel):
    """通用 API 响应模型"""
    success: bool
    data: Optional[Any] = None
    message: Optional[str] = None
    error: Optional[str] = None
    pagination: Optional[dict] = None
