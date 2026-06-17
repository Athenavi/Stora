"""
V3 权限系统核心

提供 Permission 依赖类，用于 FastAPI 路由的权限检查。
三重缓存（request-level + in-memory + Redis），性能优先。

缓存层次（从快到慢）:
  1. request.state (per-request, 0 内存分配)           → 同请求内 0 查询
  2. _MemoryCache  (进程级, TTL 300s, LRU 淘汰)         → Redis 不可用时兜底
  3. Redis         (跨进程, TTL 300s)                   → 多实例共享
  4. DB            (1× JOIN 查询)                       → 终极回退

用法:
    from src.api.v3._permission import Permission

    @router.get("/articles")
    async def list_articles(
        db=Depends(get_db),
        _=Depends(Permission("article:view")),          # 只检查
        current_user=Depends(Permission("article:view")),  # 检查 + 获取 user
    ):

    # 多权限（AND）
    _=Depends(Permission("article:edit", "article:publish"))

设计原则:
    - 低查询: 三重缓存 + superuser bypass → 常见路径 0 DB 查询
    - 低耦合: 只依赖 auth_deps 和 rbac_service
    - 高稳定: 任何缓存层不可用时自动降级，Fail-closed
"""
import asyncio
import logging
import time
from typing import Optional, Set, Tuple

from fastapi import Depends, HTTPException, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models.user import User as UserModel
from shared.services.security.rbac_service import rbac_service
from src.auth.auth_deps import get_current_user
from src.utils.database.main import get_async_session

logger = logging.getLogger(__name__)

# ============================================================
# 进程级内存缓存 — Redis 不可用时的降级方案
# ============================================================

class _MemoryCache:
    """
    进程级内存权限缓存。

    特性:
      - TTL 自动过期（默认 300s）
      - LRU 淘汰（上限 20000 条目）
      - asyncio.Lock 线程安全
      - 主动失效（invalidate）
      - 命中率统计
    """

    def __init__(self, max_entries: int = 20000, ttl: int = 300):
        self._max_entries = max_entries
        self._ttl = ttl
        # {user_id: (load_time_monotonic, frozenset(codes))}
        self._cache: dict[int, Tuple[float, frozenset]] = {}
        self._lock = asyncio.Lock()
        self._stats = {"hits": 0, "misses": 0, "evictions": 0, "size": 0}

    # ------------------------------------------------------------------
    # 公开接口
    # ------------------------------------------------------------------

    async def get(self, user_id: int) -> Optional[Set[str]]:
        """读取缓存，已过期条目静默淘汰"""
        async with self._lock:
            entry = self._cache.get(user_id)
            if entry is None:
                self._stats["misses"] += 1
                return None

            ts, codes = entry
            if time.monotonic() - ts < self._ttl:
                self._stats["hits"] += 1
                return set(codes)

            # 过期
            del self._cache[user_id]
            self._stats["misses"] += 1
            return None

    async def set(self, user_id: int, codes: Set[str]):
        """写入缓存（幂等）"""
        if not codes:
            return
        async with self._lock:
            # LRU 淘汰：淘汰最旧的条目
            if len(self._cache) >= self._max_entries and user_id not in self._cache:
                oldest_key = min(self._cache, key=lambda k: self._cache[k][0])
                del self._cache[oldest_key]
                self._stats["evictions"] += 1

            self._cache[user_id] = (time.monotonic(), frozenset(codes))

    async def invalidate(self, user_id: int):
        """主动失效指定用户缓存"""
        async with self._lock:
            self._cache.pop(user_id, None)

    async def clear(self):
        """清空全部缓存（管理操作）"""
        async with self._lock:
            self._cache.clear()
            self._stats = {"hits": 0, "misses": 0, "evictions": 0, "size": 0}

    async def stats(self) -> dict:
        """获取缓存统计"""
        async with self._lock:
            total = self._stats["hits"] + self._stats["misses"]
            hit_rate = (self._stats["hits"] / total * 100) if total > 0 else 0.0
            return {
                **self._stats,
                "size": len(self._cache),
                "hit_rate": round(hit_rate, 1),
            }


# 全局单例
_memory_cache = _MemoryCache()


# ============================================================
# 请求级缓存
# ============================================================

def _get_request_cache(request: Request) -> Optional[Set[str]]:
    """获取请求级权限缓存"""
    return getattr(request.state, '_perm_cache', None)


def _set_request_cache(request: Request, codes: Set[str]):
    """设置请求级权限缓存"""
    request.state._perm_cache = codes


# ============================================================
# Redis 客户端（懒加载 + 优雅降级）
# ============================================================

_redis_instance = None


def _get_redis():
    """获取 Redis 客户端单例（首次使用时懒加载，失败时返回 None）"""
    global _redis_instance
    if _redis_instance is None:
        try:
            from src.services.redis_service import RedisService
            _redis_instance = RedisService()
        except Exception as e:
            logger.warning(f"Redis 初始化失败，降级到内存缓存: {e}")
            _redis_instance = False
    return _redis_instance if _redis_instance is not False else None


_REDIS_PREFIX = "rbac:perms:"
_REDIS_TTL = 300
_REDIS_INVALIDATE_CHANNEL = "rbac:invalidate"


async def _redis_get_codes(user_id: int) -> Optional[Set[str]]:
    """从 Redis 获取用户权限集合"""
    redis = _get_redis()
    if not redis:
        return None
    try:
        raw = await redis.get(f"{_REDIS_PREFIX}{user_id}")
        if raw is None:
            return None
        if isinstance(raw, str):
            return set(raw.split(","))
        if isinstance(raw, list):
            return set(raw)
        return None
    except Exception as e:
        logger.warning(f"Redis 读取失败 (user={user_id}): {e}")
        return None


async def _redis_set_codes(user_id: int, codes: Set[str]):
    """写入 Redis，失败静默"""
    redis = _get_redis()
    if not redis:
        return
    try:
        await redis.set(
            f"{_REDIS_PREFIX}{user_id}",
            ",".join(sorted(codes)),
            expire=_REDIS_TTL,
        )
    except Exception as e:
        logger.warning(f"Redis 写入失败 (user={user_id}): {e}")


async def _redis_invalidate(user_id: int):
    """失效 Redis 缓存，失败静默"""
    redis = _get_redis()
    if not redis:
        return
    try:
        await redis.delete(f"{_REDIS_PREFIX}{user_id}")
    except Exception:
        pass


# ============================================================
# 核心：加载用户权限（三重缓存）
# ============================================================

async def _load_user_capability_codes(
    db: AsyncSession, user_id: int
) -> Set[str]:
    """
    加载用户权限代码集合。

    查询路径: request.state → _MemoryCache → Redis → DB
    写入路径: DB → Redis + _MemoryCache + request.state
    """
    # ── 1. 内存缓存 (进程级, Redis 不可用时兜底) ──
    cached = await _memory_cache.get(user_id)
    if cached is not None:
        logger.debug("[perm_cache=memory] user=%d hit=%d", user_id, len(cached))
        return cached

    # ── 2. Redis 缓存 (跨进程共享) ──
    cached = await _redis_get_codes(user_id)
    if cached is not None:
        logger.debug("[perm_cache=redis] user=%d hit=%d", user_id, len(cached))
        # 回写到内存缓存（为后续请求加速）
        await _memory_cache.set(user_id, cached)
        return cached

    # ── 3. DB 查询（终极回退） ──
    codes = await rbac_service.get_user_permission_codes(db, user_id)
    result = set(codes)
    logger.debug("[perm_cache=db] user=%d hit=%d", user_id, len(result))

    # 回写到两级缓存（异步、不阻塞）
    if result:
        await _memory_cache.set(user_id, result)
        await _redis_set_codes(user_id, result)

    return result


# ============================================================
# Permission 依赖类
# ============================================================

class Permission:
    """
    FastAPI 权限检查依赖。

    传入一个或多个权限代码（AND 语义），
    要求当前用户拥有全部指定权限。

    返回 UserModel（供路由函数直接使用）。
    """

    __slots__ = ('codes',)

    def __init__(self, *codes: str):
        if not codes:
            raise ValueError("Permission 至少需要一个权限代码")
        self.codes = codes

    async def __call__(
        self,
        request: Request,
        user: UserModel = Depends(get_current_user),
        db: AsyncSession = Depends(get_async_session),
    ) -> UserModel:
        try:
            await self._check(request, user, db)
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"权限检查异常 (user={user.id}, codes={self.codes}): {e}")
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="权限检查失败",
            )
        return user

    async def _check(self, request: Request, user: UserModel, db: AsyncSession):
        # ── 1. Superuser bypass — 0 查询 ──
        if user.is_superuser:
            logger.debug("[perm_cache=bypass] user=%d superuser", user.id)
            return

        # ── 2. 请求级缓存 — 同请求内 0 查询 ──
        cache = _get_request_cache(request)
        if cache is None:
            cache = await _load_user_capability_codes(db, user.id)
            _set_request_cache(request, cache)

        # ── 3. AND 权限校验（标准化冒号为点号以匹配 DB 格式） ──
        normalized_codes = tuple(c.replace(':', '.') for c in self.codes)
        if not all(c in cache for c in normalized_codes):
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail=f"权限不足: 需要 {', '.join(self.codes)}",
            )


class PermissionGuard:
    """
    权限守卫 — 只检查不返回 user。

    适合路由级 dependencies:

        router = APIRouter(dependencies=[Depends(PermissionGuard("admin:access"))])
    """

    __slots__ = ('codes',)

    def __init__(self, *codes: str):
        self.codes = codes

    async def __call__(
        self,
        request: Request,
        user: UserModel = Depends(get_current_user),
        db: AsyncSession = Depends(get_async_session),
    ):
        perm = Permission(*self.codes)
        await perm._check(request, user, db)


# ============================================================
# 装饰器形式
# ============================================================

def require_permission(code: str):
    """权限标记装饰器（元数据模式，配合 _resolve_permission_meta 使用）"""
    def decorator(func):
        existing = getattr(func, '__required_permissions__', [])
        existing.append(code)
        func.__required_permissions__ = existing
        return func
    return decorator


async def resolve_permission_meta(
    request: Request,
    user: UserModel = Depends(get_current_user),
    db: AsyncSession = Depends(get_async_session),
):
    """
    路由依赖: 读取 handler 的 __required_permissions__ 元数据并执行检查。

    用法:
        router = APIRouter(dependencies=[Depends(resolve_permission_meta)])
    """
    route = request.scope.get("route")
    if route and hasattr(route, "endpoint"):
        codes = getattr(route.endpoint, "__required_permissions__", None)
        if codes:
            perm = Permission(*codes)
            await perm._check(request, user, db)


# ============================================================
# Redis Pub/Sub 缓存广播失效
# ============================================================

_INVALIDATE_SUBSCRIBER_RUNNING = False


async def _redis_publish_invalidate(user_id: int):
    """向 Redis 频道广播用户权限失效事件"""
    redis = _get_redis()
    if not redis:
        return
    try:
        await redis.redis.publish(_REDIS_INVALIDATE_CHANNEL, str(user_id))
    except Exception as e:
        logger.debug(f"Redis 发布失效事件失败 (user={user_id}): {e}")


async def _redis_subscribe_invalidate():
    """后台任务：订阅 Redis 广播频道，处理其他实例的失效事件"""
    global _INVALIDATE_SUBSCRIBER_RUNNING
    if _INVALIDATE_SUBSCRIBER_RUNNING:
        return
    _INVALIDATE_SUBSCRIBER_RUNNING = True

    redis = _get_redis()
    if not redis:
        _INVALIDATE_SUBSCRIBER_RUNNING = False
        return

    try:
        pubsub = redis.redis.pubsub()
        await pubsub.subscribe(_REDIS_INVALIDATE_CHANNEL)
        logger.info(f"已订阅 Redis 频道: {_REDIS_INVALIDATE_CHANNEL}")

        async for message in pubsub.listen():
            if message["type"] != "message":
                continue
            try:
                user_id = int(message["data"])
                await _memory_cache.invalidate(user_id)
                logger.debug(f"[perm_cache] 收到广播失效: user={user_id}")
            except (ValueError, TypeError):
                pass
    except Exception as e:
        logger.warning(f"Redis 订阅失效频道失败: {e}")
    finally:
        _INVALIDATE_SUBSCRIBER_RUNNING = False


# ============================================================
# 缓存管理工具
# ============================================================

async def invalidate_permission_cache(user_id: int):
    """失效指定用户的全部权限缓存（本地内存 + Redis DB + Redis 广播）"""
    await _memory_cache.invalidate(user_id)
    await _redis_invalidate(user_id)
    await _redis_publish_invalidate(user_id)  # 通知其他实例


async def clear_permission_cache():
    """清空全部权限缓存（管理操作）"""
    await _memory_cache.clear()


async def get_cache_stats() -> dict:
    """获取缓存命中率等统计"""
    return await _memory_cache.stats()
