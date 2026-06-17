"""
MCP 缓存/性能工具处理器 — 缓存管理/CDN/性能指标
"""
from src.mcp.tools._perms import require_superuser


@require_superuser
async def clear_cache(arguments: dict) -> dict:
    """清理缓存（类型: all/redis/page/object）"""
    cache_type = arguments.get("cache_type", "all")
    try:
        from src.extensions import redis_client
        if cache_type in ("all", "redis"):
            # 仅清理带应用前缀的 key，不 flushdb 整个 Redis
            prefix = arguments.get("prefix", "fastblog:")
            cursor = 0
            deleted = 0
            while True:
                cursor, keys = redis_client.scan(cursor=cursor, match=f"{prefix}*", count=500)
                if keys:
                    deleted += len(keys)
                    redis_client.delete(*keys)
                if cursor == 0:
                    break
            return {"success": True, "message": f"已清理 {deleted} 个缓存 key (前缀: {prefix})"}
        return {"success": True, "message": f"缓存清理完成: {cache_type}"}
    except Exception as e:
        return {"success": False, "error": f"清理失败: {e}"}


@require_superuser
async def get_cache_status(arguments: dict) -> dict:
    """查看缓存状态"""
    try:
        from src.extensions import redis_client
        info = redis_client.info()
        dbsize = redis_client.dbsize()
        return {"success": True, "data": {
            "redis_version": info.get("redis_version", "unknown"),
            "used_memory": info.get("used_memory_human", "unknown"),
            "connected_clients": info.get("connected_clients", 0),
            "keys_count": dbsize,
            "uptime_days": info.get("uptime_in_days", 0),
        }}
    except Exception as e:
        return {"success": False, "error": f"获取缓存状态失败: {e}"}


@require_superuser
async def get_performance_metrics(arguments: dict) -> dict:
    """获取性能指标概览"""
    try:
        from src.extensions import redis_client
        info = redis_client.info()
        return {"success": True, "data": {
            "hit_rate": "N/A",
            "memory_usage": info.get("used_memory_human", "unknown"),
            "total_commands": info.get("total_commands_processed", 0),
            "total_connections": info.get("total_connections_received", 0),
            "keyspace_hits": info.get("keyspace_hits", 0),
            "keyspace_misses": info.get("keyspace_misses", 0),
        }}
    except Exception as e:
        return {"success": False, "error": f"获取指标失败: {e}"}
