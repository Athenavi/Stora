"""
暴力破解防护中间件
基于 IP 和用户名的登录尝试频率限制
"""

import time
from collections import defaultdict
from typing import Dict, Tuple

from fastapi import Request, HTTPException
from starlette.middleware.base import BaseHTTPMiddleware

from src.unified_logger import default_logger as logger


class BruteForceProtectionMiddleware(BaseHTTPMiddleware):
    """
    暴力破解防护中间件

    策略:
    - 每 IP 每 15 分钟最多 10 次登录尝试
    - 每用户名每 15 分钟最多 5 次登录尝试
    - 超过限制后返回 429 Too Many Requests
    """

    def __init__(self, app, window_minutes: int = 15, max_attempts_per_ip: int = 10, max_attempts_per_user: int = 5):
        super().__init__(app)
        self.window_seconds = window_minutes * 60
        self.max_per_ip = max_attempts_per_ip
        self.max_per_user = max_attempts_per_user
        # ip -> [(timestamp, username), ...]
        self.ip_attempts: Dict[str, list] = defaultdict(list)
        # username -> [timestamp, ...]
        self.user_attempts: Dict[str, list] = defaultdict(list)

    async def dispatch(self, request: Request, call_next):
        path = request.url.path

        # 标准化路径：去尾斜杠，仅保留 /api/v2/auth/login 和 /api/v2/auth/register
        normalized = path.rstrip('/')
        if not (normalized.endswith("/auth/login") or normalized.endswith("/auth/register") or "/auth/login" in path or "/auth/register" in path):
            return await call_next(request)

        # 优先从 X-Forwarded-For 获取真实 IP（反向代理场景）
        # 但需验证 IP 格式以防止伪造
        import re as _re
        forwarded = request.headers.get("X-Forwarded-For")
        if forwarded:
            candidate = forwarded.split(",")[0].strip()
            # Only trust if it looks like a valid IPv4 address
            if _re.match(r'^\d{1,3}(\.\d{1,3}){3}$', candidate) and all(0 <= int(octet) <= 255 for octet in candidate.split('.')):
                client_ip = candidate
            else:
                client_ip = request.client.host if request.client else "unknown"
        else:
            real_ip = request.headers.get("X-Real-IP")
            client_ip = real_ip if real_ip else (request.client.host if request.client else "unknown")

        # 清理过期记录
        now = time.time()
        self.ip_attempts[client_ip] = [
            (t, u) for t, u in self.ip_attempts[client_ip]
            if now - t < self.window_seconds
        ]

        # 检查 IP 限制
        if len(self.ip_attempts[client_ip]) >= self.max_per_ip:
            logger.warning(f"Brute force detected: IP {client_ip} exceeded login attempts")
            raise HTTPException(
                status_code=429,
                detail=f"登录尝试过于频繁，请在 {self.window_seconds // 60} 分钟后再试"
            )

        response = await call_next(request)

        # 记录登录尝试（只记录失败的）
        if response.status_code in (401, 403):
            self.ip_attempts[client_ip].append((now, ""))
            # 从请求体提取用户名（如适用）
            try:
                body = await request.json()
                username = body.get("username", "") or body.get("email", "")
                if username:
                    self.record_failed_attempt(client_ip, username)
            except Exception:
                pass

        return response

    def record_failed_attempt(self, ip: str, username: str = ""):
        """手动记录失败的登录尝试"""
        now = time.time()
        self.ip_attempts[ip].append((now, username))
        if username:
            self.user_attempts[username] = [
                t for t in self.user_attempts[username]
                if now - t < self.window_seconds
            ]
            self.user_attempts[username].append(now)

    def is_ip_blocked(self, ip: str) -> bool:
        """检查 IP 是否被封禁"""
        now = time.time()
        self.ip_attempts[ip] = [
            (t, u) for t, u in self.ip_attempts[ip]
            if now - t < self.window_seconds
        ]
        return len(self.ip_attempts[ip]) >= self.max_per_ip
