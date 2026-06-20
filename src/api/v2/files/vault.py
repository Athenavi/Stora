"""
Stora Vault API — 私密空间管理 (独立于主文件系统)
Vault 使用 AES-256-GCM 加密文件内容，文件名也被混淆。
访问需要 JWT (用户身份) + X-Vault-Token (已解锁证明)。
"""
import os
import secrets
import uuid
from datetime import datetime, timedelta

from fastapi import APIRouter, Depends, Form, Header
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from shared.models import User
from shared.models.vault.vault import Vault
from shared.models.vault.vault_item import VaultItem
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from src.utils.security.vault_crypto import (
    derive_key, encrypt_file, decrypt_file,
    encrypt_name, decrypt_name, hash_vault_password,
    HAS_CRYPTOGRAPHY,
)

router = APIRouter(tags=["vault"])

VAULT_STORAGE = os.path.join(os.getcwd(), "storage", "vault")

# ─── In-memory vault token store ───
_vault_tokens: dict = {}


def _create_vault_token(vault_id: int, user_id: int, key: bytes, timeout: int = 300) -> str:
    token = secrets.token_hex(32)
    _vault_tokens[token] = {
        "vault_id": vault_id,
        "user_id": user_id,
        "key": key,
        "expires_at": datetime.utcnow() + timedelta(seconds=timeout),
    }
    return token


def _get_vault_key(token: str, vault_id: int, user_id: int):
    entry = _vault_tokens.get(token)
    if not entry or entry["vault_id"] != vault_id or entry["user_id"] != user_id:
        return None
    if entry["expires_at"] < datetime.utcnow():
        _vault_tokens.pop(token, None)
        return None
    return entry["key"]


# ─── Vault CRUD ───

@router.post("")
async def create_vault(
    name: str = Form(...),
    password: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """创建私密空间"""
    if not HAS_CRYPTOGRAPHY:
        return fail("请安装 cryptography 库: pip install cryptography")
    if len(password) < 6:
        return fail("密码至少 6 位")
    vault_dir = os.path.join(VAULT_STORAGE, str(current_user.id))
    os.makedirs(vault_dir, exist_ok=True)

    salt = os.urandom(32)
    derived = derive_key(password, salt)
    master_key = AESGCM.generate_key(bit_length=256)
    nonce, enc_master = encrypt_file(master_key, derived)

    
    pw_key, pw_salt = hash_vault_password(password, salt)
    vault = Vault(
        user_id=current_user.id,
        name=name,
        password_hash=pw_key,
        salt=pw_salt,
        master_key_encrypted=enc_master.hex(),
        master_key_nonce=nonce.hex(),
        lock_timeout=300,
    )
    db.add(vault)
    await db.commit()
    await db.refresh(vault)
    return ok({"id": vault.id, "name": vault.name})


@router.get("")
async def list_vaults(
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出用户的私密空间（不含加密字段）"""
    rows = (await db.execute(
        select(Vault).where(Vault.user_id == current_user.id).order_by(Vault.created_at.desc())
    )).scalars().all()
    return ok([{
        "id": v.id, "name": v.name,
        "file_count": v.file_count, "total_size": v.total_size,
        "lock_timeout": v.lock_timeout,
        "created_at": str(v.created_at) if v.created_at else None,
    } for v in rows])


@router.delete("/{vault_id}")
async def delete_vault(
    vault_id: int,
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """删除私密空间（同时删除所有加密文件）"""
    vault = await db.get(Vault, vault_id)
    if not vault or vault.user_id != current_user.id:
        return fail("Vault 不存在")
    # Delete physical files
    vault_dir = os.path.join(VAULT_STORAGE, str(current_user.id), str(vault_id))
    import shutil
    if os.path.exists(vault_dir):
        shutil.rmtree(vault_dir)
    # Delete DB records
        items = (await db.execute(
        select(VaultItem).where(VaultItem.vault_id == vault_id)
    )).scalars().all()
    for item in items:
        await db.delete(item)
    await db.delete(vault)
    await db.commit()
    return ok(msg="Vault 已删除")


# ─── Password verification & token management ───

@router.post("/{vault_id}/verify-password")
async def verify_vault_password(
    vault_id: int,
    password: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """验证密码并返回短期访问 token"""
    vault = await db.get(Vault, vault_id)
    if not vault or vault.user_id != current_user.id:
        return fail("Vault 不存在")
    salt = bytes.fromhex(vault.salt)
    pw_hash = derive_key(password, salt).hex()
    if pw_hash != vault.password_hash:
        return fail("密码错误")
    # Decrypt master key
    derived = derive_key(password, bytes.fromhex(vault.salt))
    master_key = decrypt_file(
        bytes.fromhex(vault.master_key_nonce),
        bytes.fromhex(vault.master_key_encrypted),
        derived,
    )
    token = _create_vault_token(vault_id, current_user.id, master_key, vault.lock_timeout)
    return ok({"token": token, "expires_in": vault.lock_timeout})


@router.put("/{vault_id}/password")
async def change_vault_password(
    vault_id: int,
    old_password: str = Form(...),
    new_password: str = Form(...),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """修改私密空间密码（仅重新加密主密钥，无需重新加密文件）"""
    vault = await db.get(Vault, vault_id)
    if not vault or vault.user_id != current_user.id:
        return fail("Vault 不存在")
    # Verify old password
    old_derived = derive_key(old_password, bytes.fromhex(vault.salt))
    old_hash = old_derived.hex()
    if old_hash != vault.password_hash:
        return fail("原密码错误")
    # Decrypt master key with old password
    master_key = decrypt_file(
        bytes.fromhex(vault.master_key_nonce),
        bytes.fromhex(vault.master_key_encrypted),
        old_derived,
    )
    # Re-encrypt with new password
    new_salt = os.urandom(32)
    new_derived = derive_key(new_password, new_salt)
    nonce, enc_master = encrypt_file(master_key, new_derived)
    vault.password_hash = new_derived.hex()
    vault.salt = new_salt.hex()
    vault.master_key_encrypted = enc_master.hex()
    vault.master_key_nonce = nonce.hex()
    await db.commit()
    return ok(msg="密码已修改")


# ─── Vault Items (file CRUD inside vault) ───

@router.get("/{vault_id}/items")
async def list_vault_items(
    vault_id: int,
    x_vault_token: str = Header(""),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """列出 Vault 内文件（需 X-Vault-Token）"""
    key = _get_vault_key(x_vault_token, vault_id, current_user.id)
    if not key:
        return fail("Vault 未解锁，请先验证密码")
    items = (await db.execute(
        select(VaultItem).where(VaultItem.vault_id == vault_id).order_by(VaultItem.created_at.desc())
    )).scalars().all()
    result = []
    for item in items:
        try:
            name = decrypt_name(item.filename_nonce, item.filename_encrypted, key)
        except Exception:
            name = "(解密失败)"
        result.append({
            "id": item.id, "filename": name,
            "file_size": item.file_size, "mime_type": item.mime_type,
            "created_at": str(item.created_at) if item.created_at else None,
        })
    return ok(result)


@router.post("/{vault_id}/items/upload")
async def upload_vault_item(
    vault_id: int,
    filename: str = Form(...),
    file_size: int = Form(0),
    mime_type: str = Form("application/octet-stream"),
    file_content: str = Form(...),  # base64 encoded
    x_vault_token: str = Header(""),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """上传文件到 Vault（加密后存储）"""
    key = _get_vault_key(x_vault_token, vault_id, current_user.id)
    if not key:
        return fail("Vault 未解锁")
    import base64
    try:
        plaintext = base64.b64decode(file_content)
    except Exception:
        return fail("文件内容格式错误 (base64)")

    # Encrypt file content
    file_nonce, ciphertext = encrypt_file(plaintext, key)

    # Encrypt filename
    name_nonce, name_ct = encrypt_name(filename, key)

    # Save to disk
    vault_dir = os.path.join(VAULT_STORAGE, str(current_user.id), str(vault_id))
    os.makedirs(vault_dir, exist_ok=True)
    storage_name = f"{uuid.uuid4().hex}.enc"
    storage_path = os.path.join(vault_dir, storage_name)
    with open(storage_path, "wb") as f:
        f.write(ciphertext)

    item = VaultItem(
        vault_id=vault_id,
        user_id=current_user.id,
        filename_encrypted=name_ct,
        filename_nonce=name_nonce,
        mime_type=mime_type,
        file_size=len(plaintext),
        encrypted_size=len(ciphertext),
        storage_path=storage_name,
        file_nonce=file_nonce.hex(),
    )
    db.add(item)

    # Update vault counters
    vault = await db.get(Vault, vault_id)
    if vault:
        vault.file_count = (vault.file_count or 0) + 1
        vault.total_size = (vault.total_size or 0) + len(plaintext)

    await db.commit()
    await db.refresh(item)
    return ok({"id": item.id, "filename": filename})


@router.get("/{vault_id}/items/{item_id}")
async def download_vault_item(
    vault_id: int,
    item_id: int,
    x_vault_token: str = Header(""),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """下载 Vault 文件（解密后返回）"""
    key = _get_vault_key(x_vault_token, vault_id, current_user.id)
    if not key:
        return fail("Vault 未解锁")
    item = await db.get(VaultItem, item_id)
    if not item or item.vault_id != vault_id or item.user_id != current_user.id:
        return fail("文件不存在")

    vault_dir = os.path.join(VAULT_STORAGE, str(current_user.id), str(vault_id))
    file_path = os.path.join(vault_dir, item.storage_path)
    if not os.path.exists(file_path):
        return fail("文件已丢失")

    with open(file_path, "rb") as f:
        ciphertext = f.read()
    nonce = bytes.fromhex(item.file_nonce)
    plaintext = decrypt_file(nonce, ciphertext, key)

    # Decrypt filename
    try:
        original_name = decrypt_name(item.filename_nonce, item.filename_encrypted, key)
    except Exception:
        original_name = "unknown"

    from fastapi.responses import Response
    return Response(
        content=plaintext,
        media_type=item.mime_type or "application/octet-stream",
        headers={"Content-Disposition": f'attachment; filename="{original_name}"'},
    )


@router.delete("/{vault_id}/items/{item_id}")
async def delete_vault_item(
    vault_id: int,
    item_id: int,
    x_vault_token: str = Header(""),
    db: AsyncSession = Depends(get_async_db),
    current_user: User = Depends(jwt_required),
):
    """删除 Vault 中的文件"""
    key = _get_vault_key(x_vault_token, vault_id, current_user.id)
    if not key:
        return fail("Vault 未解锁")
    item = await db.get(VaultItem, item_id)
    if not item or item.vault_id != vault_id or item.user_id != current_user.id:
        return fail("文件不存在")

    # Delete physical file
    file_path = os.path.join(VAULT_STORAGE, str(current_user.id), str(vault_id), item.storage_path)
    if os.path.exists(file_path):
        os.remove(file_path)

    # Update vault counters
    vault = await db.get(Vault, vault_id)
    if vault:
        vault.file_count = max(0, (vault.file_count or 0) - 1)
        vault.total_size = max(0, (vault.total_size or 0) - (item.file_size or 0))

    await db.delete(item)
    await db.commit()
    return ok(msg="文件已删除")
