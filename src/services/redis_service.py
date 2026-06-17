"""
Redis 缓存服务
提供异步 Redis 操作和缓存管理
"""
import json
from datetime import timedelta
from typing import Optional, Any, Union

import redis.asyncio as aioredis

from src.unified_logger import default_logger as logger


class RedisService:
    """Redis 缓存服务"""

    def __init__(self, url: str = "redis://localhost:6379/0"):
        self.url = url
        self._redis: Optional[aioredis.Redis] = None

    async def connect(self):
        """连接到 Redis"""
        try:
            self._redis = aioredis.from_url(
                self.url,
                encoding="utf-8",
                decode_responses=True,
                max_connections=50,
                socket_connect_timeout=3,  # 连接超时 3 秒，避免启动阻塞
                socket_timeout=3,  # 读写超时 3 秒
                retry_on_timeout=True,  # 超时时自动重试
            )
            # 测试连接
            await self._redis.ping()
            logger.info(f"Redis 连接成功: {self.url}")
        except Exception as e:
            logger.error(f"Redis 连接失败: {e}")
            raise

    async def disconnect(self):
        """断开 Redis 连接"""
        if self._redis:
            await self._redis.close()
            logger.info("Redis 连接已关闭")

    @property
    def redis(self) -> aioredis.Redis:
        """获取 Redis 客户端实例"""
        if not self._redis:
            raise RuntimeError("Redis 未连接，请先调用 connect()")
        return self._redis

    # ==================== 基本操作 ====================

    async def get(self, key: str) -> Optional[Any]:
        """获取缓存值"""
        try:
            value = await self.redis.get(key)
            if value is None:
                return None

            # 尝试解析 JSON
            try:
                return json.loads(value)
            except (json.JSONDecodeError, TypeError):
                return value
        except Exception as e:
            logger.error(f"Redis GET 错误 ({key}): {e}")
            return None

    async def set(
            self,
            key: str,
            value: Any,
            expire: Optional[Union[int, timedelta]] = None,
    ) -> bool:
        """设置缓存值"""
        try:
            # 序列化值
            if isinstance(value, (dict, list)):
                serialized = json.dumps(value, ensure_ascii=False, default=str)
            else:
                serialized = str(value)

            # 设置过期时间
            if expire is None:
                await self.redis.set(key, serialized)
            elif isinstance(expire, timedelta):
                await self.redis.setex(key, int(expire.total_seconds()), serialized)
            else:
                await self.redis.setex(key, expire, serialized)

            return True
        except Exception as e:
            logger.error(f"Redis SET 错误 ({key}): {e}")
            return False

    async def delete(self, *keys: str) -> int:
        """删除缓存键"""
        try:
            return await self.redis.delete(*keys)
        except Exception as e:
            logger.error(f"Redis DELETE 错误: {e}")
            return 0

    async def exists(self, key: str) -> bool:
        """检查键是否存在"""
        try:
            return await self.redis.exists(key) > 0
        except Exception as e:
            logger.error(f"Redis EXISTS 错误: {e}")
            return False

    async def expire(self, key: str, seconds: int) -> bool:
        """设置过期时间"""
        try:
            return await self.redis.expire(key, seconds)
        except Exception as e:
            logger.error(f"Redis EXPIRE 错误: {e}")
            return False

    # ==================== 批量操作 ====================

    async def mget(self, *keys: str) -> list:
        """批量获取"""
        try:
            values = await self.redis.mget(*keys)
            result = []
            for value in values:
                if value is None:
                    result.append(None)
                else:
                    try:
                        result.append(json.loads(value))
                    except (json.JSONDecodeError, TypeError):
                        result.append(value)
            return result
        except Exception as e:
            logger.error(f"Redis MGET 错误: {e}")
            return []

    async def mset(self, mapping: dict, expire: Optional[int] = None) -> bool:
        """批量设置"""
        try:
            # 序列化所有值
            serialized = {}
            for key, value in mapping.items():
                if isinstance(value, (dict, list)):
                    serialized[key] = json.dumps(value, ensure_ascii=False, default=str)
                else:
                    serialized[key] = str(value)

            await self.redis.mset(serialized)

            # 设置过期时间
            if expire:
                for key in mapping.keys():
                    await self.expire(key, expire)

            return True
        except Exception as e:
            logger.error(f"Redis MSET 错误: {e}")
            return False

    # ==================== 计数器 ====================

    async def incr(self, key: str, amount: int = 1) -> int:
        """递增计数器"""
        try:
            return await self.redis.incr(key, amount)
        except Exception as e:
            logger.error(f"Redis INCR 错误: {e}")
            return 0

    async def decr(self, key: str, amount: int = 1) -> int:
        """递减计数器"""
        try:
            return await self.redis.decr(key, amount)
        except Exception as e:
            logger.error(f"Redis DECR 错误: {e}")
            return 0

    # ==================== 列表操作 ====================

    async def lpush(self, key: str, *values: Any) -> int:
        """左侧推入列表"""
        try:
            serialized = [
                json.dumps(v, ensure_ascii=False, default=str) if isinstance(v, (dict, list)) else str(v)
                for v in values
            ]
            return await self.redis.lpush(key, *serialized)
        except Exception as e:
            logger.error(f"Redis LPUSH 错误: {e}")
            return 0

    async def rpush(self, key: str, *values: Any) -> int:
        """右侧推入列表"""
        try:
            serialized = [
                json.dumps(v, ensure_ascii=False, default=str) if isinstance(v, (dict, list)) else str(v)
                for v in values
            ]
            return await self.redis.rpush(key, *serialized)
        except Exception as e:
            logger.error(f"Redis RPUSH 错误: {e}")
            return 0

    async def lrange(self, key: str, start: int = 0, end: int = -1) -> list:
        """获取列表范围"""
        try:
            values = await self.redis.lrange(key, start, end)
            result = []
            for value in values:
                try:
                    result.append(json.loads(value))
                except (json.JSONDecodeError, TypeError):
                    result.append(value)
            return result
        except Exception as e:
            logger.error(f"Redis LRANGE 错误: {e}")
            return []

    # ==================== 集合操作 ====================

    async def sadd(self, key: str, *members: Any) -> int:
        """添加到集合"""
        try:
            serialized = [
                json.dumps(m, ensure_ascii=False, default=str) if isinstance(m, (dict, list)) else str(m)
                for m in members
            ]
            return await self.redis.sadd(key, *serialized)
        except Exception as e:
            logger.error(f"Redis SADD 错误: {e}")
            return 0

    async def smembers(self, key: str) -> set:
        """获取集合所有成员"""
        try:
            members = await self.redis.smembers(key)
            result = set()
            for member in members:
                try:
                    result.add(json.loads(member))
                except (json.JSONDecodeError, TypeError):
                    result.add(member)
            return result
        except Exception as e:
            logger.error(f"Redis SMEMBERS 错误: {e}")
            return set()

    # ==================== 缓存装饰器 ====================

    def cache_key(self, prefix: str, *args, **kwargs) -> str:
        """生成缓存键"""
        key_parts = [prefix]

        # 添加位置参数
        for arg in args:
            if isinstance(arg, (dict, list)):
                key_parts.append(json.dumps(arg, sort_keys=True, default=str))
            else:
                key_parts.append(str(arg))

        # 添加关键字参数
        if kwargs:
            sorted_kwargs = sorted(kwargs.items())
            key_parts.append(json.dumps(sorted_kwargs, sort_keys=True, default=str))

        return ":".join(key_parts)

    async def get_or_set(
            self,
            key: str,
            fallback_func,
            expire: Optional[int] = 300,
    ) -> Any:
        """
        获取缓存，如果不存在则执行回调函数并缓存结果

        Args:
            key: 缓存键
            fallback_func: 回调函数（异步或同步）
            expire: 过期时间（秒）

        Returns:
            缓存值或回调结果
        """
        # 尝试从缓存获取
        value = await self.get(key)
        if value is not None:
            return value

        # 缓存未命中，执行回调
        import asyncio
        if asyncio.iscoroutinefunction(fallback_func):
            value = await fallback_func()
        else:
            value = fallback_func()

        # 存入缓存
        await self.set(key, value, expire)

        return value

    # ==================== 统计信息 ====================

    async def get_stats(self) -> dict:
        """获取 Redis 统计信息"""
        try:
            info = await self.redis.info()
            return {
                "connected": True,
                "used_memory": info.get("used_memory_human", "N/A"),
                "used_memory_peak": info.get("used_memory_peak_human", "N/A"),
                "total_connections_received": info.get("total_connections_received", 0),
                "connected_clients": info.get("connected_clients", 0),
                "keyspace_hits": info.get("keyspace_hits", 0),
                "keyspace_misses": info.get("keyspace_misses", 0),
            }
        except Exception as e:
            logger.error(f"获取 Redis 统计信息失败: {e}")
            return {
                "connected": False,
                "error": str(e),
            }

    async def flushdb(self) -> bool:
        """清空当前数据库（谨慎使用）"""
        try:
            await self.redis.flushdb()
            logger.warning("Redis 数据库已清空")
            return True
        except Exception as e:
            logger.error(f"Redis FLUSHDB 错误: {e}")
            return False

    # ==================== Pub/Sub 缓存失效广播 ====================

    async def publish(self, channel: str, message: str) -> int:
        """
        发布消息到频道

        Args:
            channel: 频道名称
            message: 消息内容（建议 JSON 格式）

        Returns:
            收到消息的订阅者数量
        """
        try:
            count = await self.redis.publish(channel, message)
            logger.debug(f"Redis PUBLISH {channel}: {count} subscribers")
            return count
        except Exception as e:
            logger.error(f"Redis PUBLISH 错误 ({channel}): {e}")
            return 0

    async def subscribe(self, channel: str):
        """
        订阅频道（返回 PubSub 对象，用于消息循环）

        Args:
            channel: 频道名称

        Returns:
            aioredis.client.PubSub 对象
        """
        try:
            pubsub = self.redis.pubsub()
            await pubsub.subscribe(channel)
            logger.info(f"Redis SUBSCRIBE {channel}")
            return pubsub
        except Exception as e:
            logger.error(f"Redis SUBSCRIBE 错误 ({channel}): {e}")
            raise

    async def psubscribe(self, pattern: str):
        """
        订阅模式匹配频道

        Args:
            pattern: 模式（如 "cache:*"）
        """
        try:
            pubsub = self.redis.pubsub()
            await pubsub.psubscribe(pattern)
            logger.info(f"Redis PSUBSCRIBE {pattern}")
            return pubsub
        except Exception as e:
            logger.error(f"Redis PSUBSCRIBE 错误 ({pattern}): {e}")
            raise

    async def publish_cache_invalidation(self, tags: list) -> int:
        """
        发布缓存失效广播

        多实例部署时，一个实例的缓存更新通知其他实例失效

        Args:
            tags: 缓存标签列表

        Returns:
            收到消息的订阅者数量
        """
        import json
        message = json.dumps({"action": "invalidate", "tags": tags, "timestamp": __import__('time').time()})
        return await self.publish("cache:invalidate", message)

    async def _cache_invalidation_handler(self, channel: str, data: str):
        """处理缓存失效消息"""
        import json
        try:
            payload = json.loads(data)
            if payload.get("action") == "invalidate":
                tags = payload.get("tags", [])
                # 延迟导入避免循环依赖
                from shared.services.core.cache_service import cache_service
                for tag in tags:
                    await cache_service.delete(tag)
                logger.info(f"Cache invalidation received: {tags}")
        except Exception as e:
            logger.warning(f"Cache invalidation handler error: {e}")

    async def start_cache_invalidation_listener(self):
        """启动缓存失效广播监听（后台任务）"""
        import asyncio
        logger.info("Starting cache:invalidate listener...")
        task = asyncio.create_task(self.listen_cache_invalidation(self._cache_invalidation_handler))
        # 让任务名可追踪
        task.set_name("cache_invalidation_listener")

    async def listen_cache_invalidation(self, handler, retry_delay: float = 5.0):
        """
        监听缓存失效广播（带自动重连）

        Args:
            handler: 回调函数，接收 (channel, data) 参数
            retry_delay: 断线重连等待秒数
        """
        import asyncio
        while True:
            try:
                pubsub = await self.psubscribe("cache:invalidate")
                logger.info("Redis pub/sub listener started for cache:invalidate")
                async for message in pubsub.listen():
                    if message["type"] == "pmessage":
                        import json
                        try:
                            data = json.loads(message["data"])
                            await handler(message["channel"], data)
                        except (json.JSONDecodeError, Exception) as e:
                            logger.error(f"缓存失效消息解析失败: {e}")
            except asyncio.CancelledError:
                logger.info("Redis pub/sub listener cancelled")
                break
            except Exception as e:
                logger.warning(f"Redis pub/sub listener disconnected: {e}, retrying in {retry_delay}s...")
                await asyncio.sleep(retry_delay)


# 全局 Redis 服务实例
redis_service = RedisService()
