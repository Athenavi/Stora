"""
MCP 用户上下文 — 独立模块避免循环导入
"""
from contextvars import ContextVar
from dataclasses import dataclass
from typing import Optional


@dataclass
class UserCtx:
    """MCP 请求用户上下文"""
    id: int
    username: str
    is_superuser: bool
    role: str = "user"


_user_ctx: ContextVar[Optional[UserCtx]] = ContextVar("_mcp_user_ctx", default=None)


def set_user_ctx(user: Optional[UserCtx]) -> None:
    """设置在当前请求上下文中执行的用户"""
    _user_ctx.set(user)


def get_user_ctx() -> Optional[UserCtx]:
    """获取当前请求的用户上下文"""
    return _user_ctx.get()
