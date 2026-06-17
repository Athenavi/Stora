"""
Stora 输入校验 — 文件名/路径/XSS 防护
"""
import re
import os
from typing import Optional

# ─── 文件名校验 ───

# Windows 保留名 (不区分大小写)
WINDOWS_RESERVED = {
    "con", "prn", "aux", "nul",
    "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
    "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9",
}

# 禁止的字符 (Windows + Linux)
INVALID_CHARS = re.compile(r'[\x00-\x1f<>:"/\\|?*\x7f]')

MAX_FILENAME_LENGTH = 255

# ─── 文件类型白名单 (基于 Magic Bytes) ───

FILE_SIGNATURES: dict[str, tuple[bytes, int]] = {
    # 图片
    "image/jpeg": (b"\xff\xd8\xff", 0),
    "image/png": (b"\x89PNG\r\n\x1a\n", 0),
    "image/gif": (b"GIF8", 0),
    "image/webp": (b"WEBP", 8),
    "image/bmp": (b"BM", 0),
    # 文档
    "application/pdf": (b"%PDF", 0),
    "application/zip": (b"PK\x03\x04", 0),
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": (b"PK\x03\x04", 0),
    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": (b"PK\x03\x04", 0),
    "application/vnd.openxmlformats-officedocument.presentationml.presentation": (b"PK\x03\x04", 0),
}

# 禁止上传的文件扩展名
BLOCKED_EXTENSIONS = {
    ".exe", ".bat", ".cmd", ".com", ".msi", ".scr", ".pif",
    ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh",
    ".ps1", ".psm1", ".psd1", ".sh", ".bash",
    ".reg", ".py", ".rb", ".pl",
}

# XSS 危险模式
XSS_PATTERNS = re.compile(
    r"<[^>]*script[^>]*>|"
    r"on\w+\s*=|"
    r"javascript\s*:|"
    r"vbscript\s*:|"
    r"<iframe|<embed|<object|<svg|<img[^>]+onerror",
    re.IGNORECASE,
)


def validate_filename(filename: str) -> Optional[str]:
    """验证文件名，返回错误信息或 None"""
    if not filename or not filename.strip():
        return "文件名不能为空"

    if len(filename) > MAX_FILENAME_LENGTH:
        return f"文件名不能超过 {MAX_FILENAME_LENGTH} 个字符"

    if INVALID_CHARS.search(filename):
        return "文件名包含非法字符"

    name_without_ext = os.path.splitext(filename)[0].lower()
    if name_without_ext in WINDOWS_RESERVED:
        return "文件名被系统保留"

    if filename.strip() != filename:
        return "文件名不能包含首尾空格"

    if filename.startswith("."):
        return "文件名不能以点号开头"

    return None


def check_extension(filename: str) -> Optional[str]:
    """检查文件扩展名是否被禁止"""
    ext = os.path.splitext(filename)[1].lower()
    if ext in BLOCKED_EXTENSIONS:
        return f"禁止上传 {ext} 文件"
    return None


def validate_path(path: str) -> Optional[str]:
    """验证路径无穿越风险"""
    normalized = os.path.normpath(path).replace("\\", "/")
    if ".." in normalized.split("/"):
        return "路径包含非法回溯"
    return None


def sanitize_text(text: str) -> str:
    """过滤 XSS 危险内容"""
    return XSS_PATTERNS.sub("", text)


def guess_mime_type(content: bytes) -> Optional[str]:
    """通过 magic bytes 猜测真实 MIME 类型"""
    for mime, (sig, offset) in FILE_SIGNATURES.items():
        if len(content) >= offset + len(sig):
            if content[offset : offset + len(sig)] == sig:
                return mime
    return None
