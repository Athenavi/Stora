"""
Vault 加解密工具 — 使用 cryptography 库实现 AES-256-GCM
提供密钥派生、文件加密/解密、文件名混淆功能
"""
import hashlib
import os
from typing import Tuple

try:
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    HAS_CRYPTOGRAPHY = True
except ImportError:
    AESGCM = None
    HAS_CRYPTOGRAPHY = False


def derive_key(password: str, salt: bytes) -> bytes:
    """PBKDF2-HMAC-SHA256 派生 256bit 密钥"""
    return hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, 100_000, dklen=32)


def encrypt_file(data: bytes, key: bytes) -> Tuple[bytes, bytes]:
    """AES-256-GCM 加密文件内容，返回 (nonce, ciphertext_with_tag)"""
    if not HAS_CRYPTOGRAPHY:
        raise RuntimeError("cryptography library required: pip install cryptography")
    nonce = os.urandom(12)
    aesgcm = AESGCM(key)
    ct = aesgcm.encrypt(nonce, data, None)
    return nonce, ct


def decrypt_file(nonce: bytes, ciphertext: bytes, key: bytes) -> bytes:
    """AES-256-GCM 解密"""
    if not HAS_CRYPTOGRAPHY:
        raise RuntimeError("cryptography library required: pip install cryptography")
    aesgcm = AESGCM(key)
    return aesgcm.decrypt(nonce, ciphertext, None)


def encrypt_name(name: str, key: bytes) -> Tuple[str, str]:
    """加密文件名，返回 (hex_nonce, hex_ciphertext)"""
    nonce, ct = encrypt_file(name.encode("utf-8"), key)
    return nonce.hex(), ct.hex()


def decrypt_name(nonce_hex: str, ct_hex: str, key: bytes) -> str:
    """解密文件名"""
    return decrypt_file(bytes.fromhex(nonce_hex), bytes.fromhex(ct_hex), key).decode("utf-8")


def hash_vault_password(password: str, salt: bytes = None) -> Tuple[str, str]:
    """哈希 vault 密码，返回 (hex_key, hex_salt)"""
    if salt is None:
        salt = os.urandom(32)
    key = derive_key(password, salt)
    return key.hex(), salt.hex()
