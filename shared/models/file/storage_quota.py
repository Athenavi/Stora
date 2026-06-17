"""
SQLAlchemy 模型定义 - StorageQuota
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class StorageQuota(Base):
    """存储配额记录模型模型"""
    __tablename__ = 'storage_quotas'


    __table_args__ = (
        Index('idx_storage_quotas_user', 'user_id', unique=True),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='配额记录 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='用户 ID')


    max_storage = Column(BigInteger, default=1073741824, doc='最大存储空间（字节）')


    used_storage = Column(BigInteger, default=0, doc='已用存储空间（字节）')


    max_file_size = Column(BigInteger, default=104857600, doc='单文件大小限制（字节，默认100MB）')


    max_files_count = Column(Integer, default=10000, doc='最大文件数')


    files_count = Column(Integer, default=0, doc='当前文件数')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'max_storage': self.max_storage,
            'used_storage': self.used_storage,
            'max_file_size': self.max_file_size,
            'max_files_count': self.max_files_count,
            'files_count': self.files_count,
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
        return f'<StorageQuota id={self.id}>'


