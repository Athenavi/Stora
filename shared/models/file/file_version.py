"""
SQLAlchemy 模型定义 - FileVersion
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class FileVersion(Base):
    """文件版本历史模型模型"""
    __tablename__ = 'file_versions'


    __table_args__ = (
        Index('idx_file_versions_file', 'file_id'),
        Index('idx_file_versions_number', 'file_id', 'version_number', unique=True),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='版本 ID')

    file_id = Column(BigInteger, ForeignKey('file_items.id'), doc='文件 ID')


    user_id = Column(BigInteger, ForeignKey('users.id'), doc='上传者用户 ID')


    version_number = Column(Integer, doc='版本号')


    file_size = Column(BigInteger, default=0, doc='版本文件大小')


    file_hash = Column(String(64), nullable=True, doc='版本文件哈希')

    storage_path = Column(String(500), nullable=True, doc='版本存储路径')

    change_note = Column(String(500), nullable=True, doc='版本变更说明')

    created_at = Column(String(255), nullable=True, doc='创建时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'file_id': self.file_id,
            'user_id': self.user_id,
            'version_number': self.version_number,
            'file_size': self.file_size,
            'file_hash': self.file_hash,
            'storage_path': self.storage_path,
            'change_note': self.change_note,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<FileVersion id={self.id}>'


