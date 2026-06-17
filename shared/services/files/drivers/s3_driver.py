"""
Stora S3/MinIO 存储驱动

支持 AWS S3、MinIO、Cloudflare R2 等兼容 S3 API 的对象存储服务。
"""
import os
import uuid
from io import BytesIO
from datetime import datetime
from typing import Optional, BinaryIO
from urllib.parse import urlparse

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError

from src.setting import settings


class S3Driver:
    """S3 兼容对象存储驱动"""

    def __init__(
        self,
        endpoint: str = "",
        bucket: str = "",
        access_key: str = "",
        secret_key: str = "",
        region: str = "auto",
        public_url: str = "",
        base_path: str = "stora",
    ):
        self.endpoint = endpoint or getattr(settings, "S3_ENDPOINT", "")
        self.bucket = bucket or getattr(settings, "S3_BUCKET", "stora")
        self.access_key = access_key or getattr(settings, "S3_ACCESS_KEY", "")
        self.secret_key = secret_key or getattr(settings, "S3_SECRET_KEY", "")
        self.region = region or getattr(settings, "S3_REGION", "auto")
        self.public_url = public_url or getattr(settings, "S3_PUBLIC_URL", "")
        self.base_path = base_path

        if not self.endpoint or not self.access_key:
            raise ValueError("S3 配置不完整: 请设置 endpoint 和 access_key")

        self.client = boto3.client(
            "s3",
            endpoint_url=self.endpoint,
            aws_access_key_id=self.access_key,
            aws_secret_access_key=self.secret_key,
            region_name=self.region,
            config=Config(signature_version="s3v4"),
        )

    def _object_key(self, filename: str = "") -> str:
        """生成对象存储路径: base_path/YYYY/MM/uuid.ext"""
        ext = os.path.splitext(filename)[1] or ""
        now = datetime.utcnow()
        return f"{self.base_path}/{now.year}/{now.month:02d}/{uuid.uuid4().hex}{ext}"

    def save(self, content: bytes, filename: str = "") -> str:
        """上传文件，返回 object key"""
        key = self._object_key(filename)
        self.client.put_object(Bucket=self.bucket, Key=key, Body=content)
        return key

    def save_from_path(self, src_path: str, filename: str = "") -> str:
        """从本地路径上传文件"""
        key = self._object_key(filename)
        with open(src_path, "rb") as f:
            self.client.upload_fileobj(f, self.bucket, key)
        return key

    def delete(self, key: str) -> bool:
        """删除对象"""
        try:
            self.client.delete_object(Bucket=self.bucket, Key=key)
            return True
        except ClientError:
            return False

    def get_url(self, key: str) -> str:
        """获取文件访问 URL"""
        if self.public_url:
            return f"{self.public_url}/{key}"
        return f"{self.endpoint}/{self.bucket}/{key}"

    def get_download_url(self, key: str, expires_in: int = 3600) -> str:
        """获取预签名下载 URL"""
        return self.client.generate_presigned_url(
            "get_object",
            Params={"Bucket": self.bucket, "Key": key},
            ExpiresIn=expires_in,
        )

    def exists(self, key: str) -> bool:
        """检查对象是否存在"""
        try:
            self.client.head_object(Bucket=self.bucket, Key=key)
            return True
        except ClientError:
            return False


# Singleton instance
_instance: Optional[S3Driver] = None


def get_s3_driver() -> S3Driver:
    """获取 S3 驱动单例"""
    global _instance
    if _instance is None:
        _instance = S3Driver()
    return _instance
