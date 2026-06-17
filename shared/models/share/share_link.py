"""
SQLAlchemy 模型定义 - ShareLink
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class ShareLink(Base):
    """外部分享链接模型模型"""
    __tablename__ = 'share_links'


    __table_args__ = (
        Index('idx_share_links_code', 'short_code', unique=True),
        Index('idx_share_links_user', 'user_id'),
        Index('idx_share_links_active', 'is_active'),
        Index('idx_share_links_expires', 'expires_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='链接 ID')

    share_id = Column(BigInteger, ForeignKey('file_shares.id'), nullable=True, doc='关联的分享记录 ID')


    file_id = Column(BigInteger, ForeignKey('file_items.id'), nullable=True, doc='文件 ID')


    folder_id = Column(BigInteger, ForeignKey('folders.id'), nullable=True, doc='文件夹 ID')


    user_id = Column(BigInteger, ForeignKey('users.id'), doc='创建者用户 ID')


    short_code = Column(String(32), unique=True, nullable=True, doc='短链接代码')

    password_hash = Column(String(128), nullable=True, doc='访问密码哈希')

    permission = Column(String(20), default='read', doc='权限 (read/download/write)')

    max_downloads = Column(Integer, default=0, doc='最大下载次数（0=不限）')


    download_count = Column(Integer, default=0, doc='已下载次数')


    max_views = Column(Integer, default=0, doc='最大浏览次数（0=不限）')


    view_count = Column(Integer, default=0, doc='已浏览次数')


    expires_at = Column(String(255), nullable=True, doc='过期时间')

    is_active = Column(Boolean, default=True, doc='是否有效')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'share_id': self.share_id,
            'file_id': self.file_id,
            'folder_id': self.folder_id,
            'user_id': self.user_id,
            'short_code': self.short_code,
            'password_hash': self.password_hash,
            'permission': self.permission,
            'max_downloads': self.max_downloads,
            'download_count': self.download_count,
            'max_views': self.max_views,
            'view_count': self.view_count,
            'expires_at': self.expires_at,
            'is_active': self.is_active,
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
        return f'<ShareLink id={self.id}>'


