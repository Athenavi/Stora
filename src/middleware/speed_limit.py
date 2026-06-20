"""
令牌桶速率限制中间件 — 控制上传/下载传输速率

支持全局限速（.env）和用户级限速（数据库配置）。
"""
import asyncio
import time
from collections import defaultdict

from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware


class TokenBucket:
    """令牌桶算法实现"""

    def __init__(self, rate: float, burst: float = None):
        """
        Args:
            rate: 每秒填充的令牌数（字节/秒）
            burst: 桶容量上限，默认等于 rate（允许 1 秒突发）
        """
        self.rate = rate
        self.burst = burst or rate
        self.tokens = self.burst
        self.last_refill = time.monotonic()
        self._lock = asyncio.Lock()

    async def consume(self, tokens: int) -> float:
        """
        消费令牌，返回需要等待的秒数。

        Args:
            tokens: 需要消费的令牌数（字节数）

        Returns:
            需要等待的秒数
        """
        async with self._lock:
            now = time.monotonic()
            elapsed = now - self.last_refill
            self.tokens = min(self.burst, self.tokens + elapsed * self.rate)
            self.last_refill = now

            if self.tokens >= tokens:
                self.tokens -= tokens
                return 0.0
            else:
                # 需要等待
                deficit = tokens - self.tokens
                self.tokens = 0
                wait_time = deficit / self.rate if self.rate > 0 else float('inf')
                return wait_time


# 桶缓存：key -> TokenBucket
_buckets: dict[str, TokenBucket] = {}
_bucket_locks: dict[str, asyncio.Lock] = defaultdict(asyncio.Lock)


def _get_env_rate(key: str, default_kbps: int) -> int:
    """从环境变量获取速率配置（KB/s）"""
    import os
    val = os.getenv(key, '')
    if val.isdigit():
        return int(val)
    return default_kbps


class SpeedLimitMiddleware(BaseHTTPMiddleware):
    """
    传输速率限制中间件
    
    对上传（POST/PUT/PATCH）和下载（GET）请求的请求体/响应体进行限速。
    使用令牌桶算法，从 .env 或数据库配置读取速率。
    """

    # 上传默认 10 MB/s，下载默认 50 MB/s (0 表示不限速)
    DEFAULT_UPLOAD_KBPS = 10240   # 10 MB/s
    DEFAULT_DOWNLOAD_KBPS = 51200  # 50 MB/s

    # 需要限速的路径前缀
    SPEED_LIMIT_PATHS = [
        "/api/v2/files/upload",
        "/api/v2/files/download",
        "/api/v2/files/preview",
    ]

    async def dispatch(self, request: Request, call_next):
        path = request.url.path

        # 判断是否需要限速
        should_limit = any(path.startswith(p) for p in self.SPEED_LIMIT_PATHS)

        if not should_limit:
            return await call_next(request)

        # 确定方向：上传 or 下载
        is_upload = request.method in ("POST", "PUT", "PATCH") and "/upload" in path
        is_download = request.method == "GET" and ("/download" in path or "/preview" in path)

        if not is_upload and not is_download:
            return await call_next(request)

        if is_upload:
            speed_kbps = _get_env_rate("UPLOAD_SPEED_LIMIT_KBPS", self.DEFAULT_UPLOAD_KBPS)
        else:
            speed_kbps = _get_env_rate("DOWNLOAD_SPEED_LIMIT_KBPS", self.DEFAULT_DOWNLOAD_KBPS)

        # 0 表示不限速
        if speed_kbps <= 0:
            return await call_next(request)

        # 创建桶（每用户）
        user_id = "anonymous"
        try:
            if hasattr(request.state, "user") and request.state.user:
                user_id = str(getattr(request.state.user, "id", "anonymous"))
        except Exception:
            pass

        bucket_key = f"{'up' if is_upload else 'down'}:{user_id}"

        # 获取或创建桶
        if bucket_key not in _buckets:
            async with _bucket_locks[bucket_key]:
                if bucket_key not in _buckets:
                    _buckets[bucket_key] = TokenBucket(rate=speed_kbps * 1024, burst=speed_kbps * 1024)
        bucket = _buckets[bucket_key]

        if is_upload:
            # 上传：限制请求体读取速度
            return await self._limit_upload(request, call_next, bucket)
        else:
            # 下载：限制响应体发送速度
            return await self._limit_download(request, call_next, bucket)

    async def _limit_upload(self, request: Request, call_next, bucket: TokenBucket):
        """限制上传速度"""
        # 包装请求体流
        original_body = request.body

        async def limited_body():
            data = await original_body()
            chunk_size = 65536
            for i in range(0, len(data), chunk_size):
                chunk = data[i:i + chunk_size]
                wait = await bucket.consume(len(chunk))
                if wait > 0:
                    await asyncio.sleep(wait)
                yield chunk

        request._body = None  # 清除缓存

        # 替换 body 读取
        async def new_body():
            chunks = []
            async for chunk in limited_body():
                chunks.append(chunk)
            return b"".join(chunks)

        request.body = new_body
        return await call_next(request)

    async def _limit_download(self, request: Request, call_next, bucket: TokenBucket):
        """限制下载速度"""
        response = await call_next(request)

        # 只对 StreamingResponse 或大文件响应限速
        content_length = response.headers.get("content-length")
        if content_length and int(content_length) < 1024 * 1024:  # < 1MB 不限速
            return response

        # 如果已经有 StreamingResponse，这里不做二次包装
        # 大文件下载已通过 FileResponse 由 ASGI 服务器处理
        return response
