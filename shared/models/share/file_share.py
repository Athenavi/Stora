"""
SQLAlchemy 模型定义 - FileShare
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class FileShare(Base):
    """文件/文件夹分享记录模型模型"""
    __tablename__ = 'file_shares'


    __table_args__ = (
        Index('idx_file_shares_owner', 'owner_id'),
        Index('idx_file_shares_shared_with', 'shared_with_id'),
        Index('idx_file_shares_file', 'file_id'),
        Index('idx_file_shares_folder', 'folder_id'),
        Index('idx_file_shares_expires', 'expires_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='分享记录 ID')

    file_id = Column(BigInteger, ForeignKey('file_items.id'), nullable=True, doc='分享的文件 ID')


    folder_id = Column(BigInteger, ForeignKey('folders.id'), nullable=True, doc='分享的文件夹 ID')


    owner_id = Column(BigInteger, ForeignKey('users.id'), doc='分享者用户 ID')


    shared_with_id = Column(BigInteger, ForeignKey('users.id'), nullable=True, doc='分享给的用户 ID（null=公开链接）')


    permission = Column(String(20), default='read', doc='权限 (read/write/admin)')

    expires_at = Column(String(255), nullable=True, doc='过期时间')

    is_link_share = Column(Boolean, default=False, doc='是否链接分享')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'file_id': self.file_id,
            'folder_id': self.folder_id,
            'owner_id': self.owner_id,
            'shared_with_id': self.shared_with_id,
            'permission': self.permission,
            'expires_at': self.expires_at,
            'is_link_share': self.is_link_share,
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
        return f'<FileShare id={self.id}>'


