"""
Models package - Lazy loading version

All model classes are imported on demand via __getattr__ to avoid loading
all model files at startup. Base remains eagerly imported (required for
SQLAlchemy metadata initialization).

Auto-generated from config/models.yaml - Do not edit manually
"""

from sqlalchemy.orm import declarative_base

Base = declarative_base()

# ==================== Lazy import mapping ====================
_LAZY_IMPORTS = {
    'AccessLog': '.file.access_log',
    'AuditLog': '.system.audit_log',
    'Capability': '.rbac.capability',
    'DownloadTask': '.file.download_task',
    'DownloadToken': '.file.download_token',
    'FieldPermission': '.security.field_permission',
    'FileFingerprint': '.file.file_fingerprint',
    'FileItem': '.file.file_item',
    'FileOptimization': '.file.file_optimization',
    'FileShare': '.share.file_share',
    'FileTag': '.file.file_tag',
    'FileTagAssignment': '.file.file_tag_assignment',
    'FileVersion': '.file.file_version',
    'Folder': '.file.folder',
    'GDPRConsent': '.security.gdpr_consent',
    'LoginAttempt': '.security.login_attempt',
    'Notification': '.notification.notification',
    'PermissionAuditLog': '.rbac.permission_audit_log',
    'Role': '.rbac.role',
    'RoleCapability': '.rbac.role_capability',
    'SearchHistory': '.search.search_history',
    'SensitiveWord': '.security.sensitive_word',
    'ShareLink': '.share.share_link',
    'StorageQuota': '.file.storage_quota',
    'SystemSettings': '.system.system_settings',
    'TokenBlacklist': '.security.token_blacklist',
    'TrashItem': '.file.trash_item',
    'UploadChunk': '.file.upload_chunk',
    'UploadTask': '.file.upload_task',
    'User': '.user.user',
    'UserBlock': '.user.user_block',
    'UserRole': '.rbac.user_role',
    'UserSession': '.user.user_session',
}

# ==================== Dynamic import support ====================


def __getattr__(name):
    if name in _LAZY_IMPORTS:
        import importlib
        module = importlib.import_module(_LAZY_IMPORTS[name], __package__)
        return getattr(module, name)
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")


def __dir__():
    return list(_LAZY_IMPORTS.keys()) + ['Base', '_LAZY_IMPORTS']


__all__ = ['Base'] + list(_LAZY_IMPORTS.keys())
