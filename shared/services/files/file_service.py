"""
Stora 文件存储服务 — 处理文件/文件夹的核心业务逻辑和存储驱动抽象
"""
import os
import uuid
import hashlib
import shutil
from datetime import datetime
from typing import Optional, BinaryIO

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_

from shared.models import FileItem, Folder, FileFingerprint, StorageQuota


STORAGE_ROOT = os.path.join(os.getcwd(), "storage", "files")


class StorageDriver:
    """存储驱动抽象（支持 local 和 S3/MinIO 扩展）"""

    def __init__(self, base_path: str = STORAGE_ROOT):
        self.base_path = base_path
        os.makedirs(base_path, exist_ok=True)

    def save(self, content: bytes, filename: str = "") -> str:
        """保存文件，返回相对路径"""
        ext = os.path.splitext(filename)[1] or ""
        name = f"{uuid.uuid4().hex}{ext}"
        path = self._date_path(name)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(content)
        return path

    def save_from_path(self, src_path: str, filename: str = "") -> str:
        """从已有路径复制文件"""
        ext = os.path.splitext(filename)[1] or ""
        name = f"{uuid.uuid4().hex}{ext}"
        path = self._date_path(name)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        shutil.copy2(src_path, path)
        return path

    def delete(self, path: str) -> bool:
        """删除文件"""
        full_path = os.path.join(self.base_path, path) if not os.path.isabs(path) else path
        if os.path.exists(full_path):
            os.remove(full_path)
            return True
        return False

    def get_full_path(self, path: str) -> str:
        """获取完整文件路径"""
        return os.path.join(self.base_path, path) if not os.path.isabs(path) else path

    def file_exists(self, path: str) -> bool:
        return os.path.exists(self.get_full_path(path))

    def _date_path(self, filename: str) -> str:
        """按日期分目录存储"""
        now = datetime.utcnow()
        return os.path.join(
            self.base_path,
            str(now.year),
            f"{now.month:02d}",
            filename,
        )


driver = StorageDriver()


# ─── File Operations ───

async def create_file(
    db: AsyncSession,
    user_id: int,
    filename: str,
    content: bytes,
    folder_id: Optional[int] = None,
    mime_type: str = "application/octet-stream",
) -> FileItem:
    """创建文件（自动秒传检测+配额检查）"""
    # Check quota
    quota = await _get_or_create_quota(db, user_id)
    file_size = len(content)

    if (quota.used_storage or 0) + file_size > (quota.max_storage or 0):
        raise ValueError("存储空间不足")

    # Calculate hash for dedup
    file_hash = hashlib.sha256(content).hexdigest()

    # Check instant upload
    fingerprint = (await db.execute(
        select(FileFingerprint).where(
            FileFingerprint.hash == file_hash,
            FileFingerprint.file_size == file_size,
        )
    )).scalar_one_or_none()

    if fingerprint:
        fingerprint.reference_count += 1
        storage_path = fingerprint.storage_path
    else:
        storage_path = driver.save(content, filename)
        fingerprint = FileFingerprint(
            hash=file_hash,
            file_size=file_size,
            mime_type=mime_type,
            storage_path=storage_path,
        )
        db.add(fingerprint)

    file_type = _detect_type(filename, mime_type)
    file = FileItem(
        user_id=user_id,
        folder_id=folder_id,
        filename=filename,
        original_filename=filename,
        file_size=file_size,
        mime_type=mime_type,
        file_type=file_type,
        file_hash=file_hash,
        storage_path=storage_path,
        storage_driver="local",
        file_url=f"/api/v2/files/download/{os.path.basename(storage_path)}",
    )
    db.add(file)

    # Update quota
    quota.used_storage = (quota.used_storage or 0) + file_size
    quota.files_count = (quota.files_count or 0) + 1

    await db.commit()
    await db.refresh(file)
    return file


async def delete_file(
    db: AsyncSession,
    file: FileItem,
    permanent: bool = False,
):
    """删除文件"""
    if permanent:
        # Decrement fingerprint ref count
        if file.file_hash:
            fp = (await db.execute(
                select(FileFingerprint).where(FileFingerprint.hash == file.file_hash)
            )).scalar_one_or_none()
            if fp:
                fp.reference_count -= 1
                if fp.reference_count <= 0:
                    driver.delete(fp.storage_path)
                    await db.delete(fp)
        # Update quota
        quota = await _get_or_create_quota(db, file.user_id)
        quota.used_storage = max(0, (quota.used_storage or 0) - (file.file_size or 0))
        quota.files_count = max(0, (quota.files_count or 0) - 1)
        await db.delete(file)
    else:
        file.deleted_at = func.now()
    await db.commit()


async def move_file_to_folder(
    db: AsyncSession,
    file: FileItem,
    target_folder_id: Optional[int],
) -> FileItem:
    """移动文件到文件夹"""
    file.folder_id = target_folder_id
    await db.commit()
    await db.refresh(file)
    return file


async def rename_file(
    db: AsyncSession,
    file: FileItem,
    new_name: str,
) -> FileItem:
    """重命名文件"""
    file.filename = new_name
    await db.commit()
    await db.refresh(file)
    return file


# ─── Folder Operations ───

async def create_folder(
    db: AsyncSession,
    user_id: int,
    name: str,
    parent_id: Optional[int] = None,
) -> Folder:
    """创建文件夹"""
    folder = Folder(user_id=user_id, parent_id=parent_id, name=name)
    db.add(folder)
    await db.commit()
    await db.refresh(folder)
    return folder


async def get_folder_path(
    db: AsyncSession,
    folder_id: Optional[int],
) -> list:
    """获取文件夹路径（面包屑）"""
    path = []
    current_id = folder_id
    while current_id:
        folder = await db.get(Folder, current_id)
        if not folder:
            break
        path.append({"id": folder.id, "name": folder.name})
        current_id = folder.parent_id
    path.reverse()
    return path


# ─── Quota ───

async def _get_or_create_quota(db: AsyncSession, user_id: int) -> StorageQuota:
    """获取或创建用户存储配额"""
    quota = (await db.execute(
        select(StorageQuota).where(StorageQuota.user_id == user_id)
    )).scalar_one_or_none()
    if not quota:
        quota = StorageQuota(user_id=user_id)
        db.add(quota)
        await db.flush()
    return quota


async def get_user_quota(db: AsyncSession, user_id: int) -> dict:
    """获取用户存储配额信息"""
    quota = await _get_or_create_quota(db, user_id)
    return {
        "max_storage": quota.max_storage,
        "used_storage": quota.used_storage,
        "max_file_size": quota.max_file_size,
        "max_files_count": quota.max_files_count,
        "files_count": quota.files_count,
        "usage_percent": round(
            (quota.used_storage or 0) / max(quota.max_storage or 1, 1) * 100, 1
        ),
    }


# ─── Helpers ───

def _detect_type(filename: str, mime_type: str) -> str:
    ext = os.path.splitext(filename)[1].lower()
    if ext in {".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp"} or mime_type.startswith("image/"):
        return "image"
    if ext in {".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".webm"} or mime_type.startswith("video/"):
        return "video"
    if ext in {".mp3", ".wav", ".ogg", ".flac", ".aac"} or mime_type.startswith("audio/"):
        return "audio"
    if ext in {".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".md", ".csv"}:
        return "document"
    if ext in {".zip", ".rar", ".7z", ".tar", ".gz", ".bz2"}:
        return "archive"
    return "other"
