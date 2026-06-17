"""
Stora 文件加密 — AES-256-GCM
"""
import os
import hashlib
import hmac
from typing import Tuple

from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from src.setting import settings

# 从环境变量读取主密钥，不存在则生成 (32 字节 hex)
_MASTER_KEY_HEX = getattr(settings, "STORA_ENCRYPTION_KEY", os.environ.get("STORA_ENCRYPTION_KEY", ""))
MASTER_KEY = bytes.fromhex(_MASTER_KEY_HEX) if _MASTER_KEY_HEX else None
AESGCM_NONCE_LENGTH = 12  # 96 bits


def is_enabled() -> bool:
    """加密功能是否启用"""
    return MASTER_KEY is not None


def _derive_file_key(file_id: int) -> bytes:
    """从主密钥衍生每文件密钥: HMAC-SHA256(master_key, str(file_id))[:16]"""
    if not MASTER_KEY:
        raise RuntimeError("Encryption not configured: STORA_ENCRYPTION_KEY not set")
    return hmac.new(MASTER_KEY, str(file_id).encode(), hashlib.sha256).digest()[:16]


def encrypt_file(file_id: int, data: bytes) -> bytes:
    """
    加密文件内容
    返回: nonce(12) + ciphertext
    """
    if not data:
        return b""
    key = _derive_file_key(file_id)
    aesgcm = AESGCM(key)
    nonce = os.urandom(AESGCM_NONCE_LENGTH)
    ciphertext = aesgcm.encrypt(nonce, data, None)
    return nonce + ciphertext


def decrypt_file(file_id: int, data: bytes) -> bytes:
    """
    解密文件内容
    输入: nonce(12) + ciphertext
    """
    if not data or len(data) < AESGCM_NONCE_LENGTH + 1:
        return data
    key = _derive_file_key(file_id)
    aesgcm = AESGCM(key)
    nonce = data[:AESGCM_NONCE_LENGTH]
    ciphertext = data[AESGCM_NONCE_LENGTH:]
    return aesgcm.decrypt(nonce, ciphertext, None)
