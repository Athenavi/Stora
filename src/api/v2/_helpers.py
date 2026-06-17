"""
V2 共享路由辅助工具

向后兼容包装 — 实际实现在 src/utils/errors.py
"""
from typing import TypeVar

from src.api.v2._base import ApiResponse
from src.utils.errors import ok, fail, StoraException, ErrorCode

T = TypeVar('T')

# 导出旧接口保持兼容
__all__ = ['ok', 'fail', 'StoraException', 'ErrorCode', 'ApiResponse']


def _catch():
    return None
