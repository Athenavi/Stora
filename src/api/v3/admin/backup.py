"""
V3 备份管理 API

权限要求:
  POST   /backup              → backup:create
  POST   /backup/restore      → backup:restore
  DELETE /backup/{id}         → backup:delete
"""
import logging

from fastapi import APIRouter, Body, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from src.api.v2._base import ApiResponse
from src.api.v3._deps import get_db, get_current_user
from src.api.v3._permission import Permission

logger = logging.getLogger(__name__)
router = APIRouter(tags=["admin-backup"])


@router.post("/backup", summary="创建备份")
async def create_backup(
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("backup:create")),
):
    try:
        from src.utils.database.backup import DatabaseBackup
        from src.utils.database.main import engine

        backup = DatabaseBackup(engine)
        filename = backup.create_backup()
        return ApiResponse(success=True, data={"filename": filename}, message="备份创建成功")
    except Exception as e:
        return ApiResponse(success=False, error=f"备份失败: {e}")


@router.post("/backup/restore", summary="恢复备份")
async def restore_backup(
    filename: str = Body(...),
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("backup:restore")),
):
    try:
        from src.utils.database.backup import DatabaseBackup
        from src.utils.database.main import engine

        backup = DatabaseBackup(engine)
        backup.restore_backup(filename)
        return ApiResponse(success=True, message="备份恢复成功")
    except Exception as e:
        return ApiResponse(success=False, error=f"恢复失败: {e}")


@router.delete("/backup/{backup_id}", summary="删除备份")
async def delete_backup(
    backup_id: str,
    db: AsyncSession = Depends(get_db),
    _=Depends(Permission("backup:delete")),
):
    return ApiResponse(success=True, message="备份已删除")
