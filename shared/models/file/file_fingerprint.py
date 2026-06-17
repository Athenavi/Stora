"""
SQLAlchemy 模型定义 - FileFingerprint
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class FileFingerprint(Base):
    """文件指纹/哈希模型（用于秒传和去重）模型"""
    __tablename__ = 'file_fingerprints'


    __table_args__ = (
        Index('idx_file_fingerprints_hash', 'hash', unique=True),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='指纹 ID')

    hash = Column(String(64), unique=True, nullable=True, doc='文件 SHA256 哈希')

    file_size = Column(BigInteger, doc='文件大小')


    mime_type = Column(String(100), nullable=True, doc='MIME 类型')

    storage_path = Column(String(500), nullable=True, doc='存储路径')

    reference_count = Column(Integer, default=1, doc='引用计数')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'hash': self.hash,
            'file_size': self.file_size,
            'mime_type': self.mime_type,
            'storage_path': self.storage_path,
            'reference_count': self.reference_count,
            'created_at': self.created_at,
            'updated_at': self.updated_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<FileFingerprint id={self.id}>'


