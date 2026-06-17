"""
数据迁移钩子系统

三层升级流水线的 Layer 2 — 幂等数据迁移。
每个钩子自查预条件，只做需要做的事，重复执行安全。
"""
import importlib
import pkgutil
from typing import Callable, List, Tuple

_registry: List[Tuple[int, str, Callable]] = []


def data_migration(name: str = None, priority: int = 500):
    """注册一个幂等数据迁移钩子

    Args:
        name: 钩子名称（默认取函数名）
        priority: 执行优先级（越小越先执行）

    用法:
        @data_migration(priority=100)
        def normalize_slugs(db):
            ...
    """
    def decorator(fn):
        hook_name = name or fn.__name__
        _registry.append((priority, hook_name, fn))
        _registry.sort(key=lambda x: x[0])
        return fn
    return decorator


def discover():
    """自动发现 hooks/ 目录下的所有钩子"""
    pkg = importlib.import_module("shared.upgrade.hooks")
    for _, mod_name, _ in pkgutil.iter_modules(pkg.__path__):
        importlib.import_module(f"shared.upgrade.hooks.{mod_name}")
    return list(_registry)
