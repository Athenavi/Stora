"""
密码验证模块
使用 argon2-cffi 进行密码哈希和验证
"""
import re

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError

# 创建全局密码哈希器实例
ph = PasswordHasher()


def hash_password(password: str) -> str:
    """
    对密码进行 Argon2 哈希处理
    
    Args:
        password: 明文密码
    
    Returns:
        str: Argon2 哈希后的密码字符串
    """
    return ph.hash(password)


def verify_password(plain_password: str, hashed_password: str) -> bool:
    """
    验证明文密码与哈希密码是否匹配
    
    Args:
        plain_password: 明文密码
        hashed_password: Argon2 哈希后的密码
    
    Returns:
        bool: 验证成功返回 True，否则返回 False
    """
    try:
        ph.verify(hashed_password, plain_password)
        return True
    except VerifyMismatchError:
        return False
    except Exception:
        # 捕获其他可能的异常（如哈希格式错误）
        return False


def validate_password_strength(password: str) -> tuple[bool, str]:
    """校验密码强度，与前端 Zod schema 规则保持一致

    规则:
      - 至少 8 个字符
      - 至少包含一个字母和一个数字

    Returns:
        (is_valid, error_message)
    """
    if not password or len(password) < 8:
        return False, '密码长度至少为 8 个字符'
    if len(password) > 128:
        return False, '密码长度不能超过 128 个字符'
    if not re.search(r'[a-zA-Z]', password):
        return False, '密码必须包含字母'
    if not re.search(r'\d', password):
        return False, '密码必须包含数字'
    return True, ''
