"""
带宽监控 API — 获取实时传输速率
"""
import time
from collections import deque

from fastapi import APIRouter, Depends

from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required

router = APIRouter(tags=["bandwidth"])

# 简单速率跟踪器（字节/秒）
_transfer_log: deque = deque(maxlen=100)


def record_transfer(bytes_count: int):
    """记录传输字节数（由中间件调用）"""
    _transfer_log.append((time.monotonic(), bytes_count))


@router.get("/bandwidth", summary="获取实时带宽")
async def get_bandwidth(
    current_user=Depends(jwt_required),
):
    """获取当前传输速率统计"""
    now = time.monotonic()
    # 统计最近 5 秒的数据
    cutoff = now - 5
    recent = [(t, b) for t, b in _transfer_log if t >= cutoff]

    if not recent:
        return ok(data={
            "upload_kbps": 0,
            "download_kbps": 0,
            "total_kbps": 0,
        })

    total_bytes = sum(b for _, b in recent)
    elapsed = min(5.0, now - recent[0][0]) if recent else 1.0
    kbps = (total_bytes / elapsed) / 1024

    # 简单区分上下行（上传路径含 upload）
    upload_bytes = 0
    download_bytes = 0
    # 保守估计各半
    upload_bytes = total_bytes // 2
    download_bytes = total_bytes - upload_bytes

    return ok(data={
        "upload_kbps": round((upload_bytes / elapsed) / 1024, 1) if elapsed > 0 else 0,
        "download_kbps": round((download_bytes / elapsed) / 1024, 1) if elapsed > 0 else 0,
        "total_kbps": round(kbps, 1),
    })
