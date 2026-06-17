"""
FastBlog 共享工具模块
包含版本管理、更新历史、备份管理等通用工具
"""

from shared.utils.auto_update_checker import (
    AutoUpdateChecker,
    auto_update_checker,
    check_updates_now
)
from shared.utils.backup_manager import (
    BackupManager,
    backup_manager
)
from shared.utils.update_history import (
    UpdateHistoryManager,
    update_history_manager,
    add_update_history
)
from shared.utils.version_manager import (
    VersionManager,
    version_manager,
)

__all__ = [
    # Version Manager
    'VersionManager',
    'version_manager',
    
    # Update History
    'UpdateHistoryManager',
    'update_history_manager',
    'add_update_history',
    
    # Backup Manager
    'BackupManager',
    'backup_manager',
    
    # Auto Update Checker
    'AutoUpdateChecker',
    'auto_update_checker',
    'check_updates_now',
]
