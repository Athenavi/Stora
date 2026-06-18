"""
缓存管理器包
提供多层级缓存服
"""
from shared.services.core.cache_service import CacheService, AssetMinifier, LazyLoadService

# 全局实例
cache_service = CacheService()
lazy_load_service = LazyLoadService()
asset_minifier = AssetMinifier()

__all__ = [
    'CacheService',
    'LazyLoadService',
    'AssetMinifier',
    'cache_service',
    'lazy_load_service',
    'asset_minifier',
]
