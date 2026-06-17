"""
SQLAlchemy 模型定义 - DownloadToken
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class DownloadToken(Base):
    """一次性下载令牌模型模型"""
    __tablename__ = 'download_tokens'


    __table_args__ = (
        Index('idx_download_tokens_token', 'token', unique=True),
        Index('idx_download_tokens_file', 'file_id'),
        Index('idx_download_tokens_used', 'is_used'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='令牌 ID')

    token = Column(String(128), unique=True, nullable=True, doc='令牌字符串')

    file_id = Column(BigInteger, ForeignKey('file_items.id'), doc='文件 ID')


    user_id = Column(BigInteger, ForeignKey('users.id'), nullable=True, doc='请求用户 ID')


    share_link_id = Column(BigInteger, ForeignKey('share_links.id'), nullable=True, doc='关联的分享链接 ID')


    ip_address = Column(String(45), nullable=True, doc='请求 IP')

    is_used = Column(Boolean, default=False, doc='是否已使用')


    expires_at = Column(String(255), nullable=True, doc='过期时间')

    created_at = Column(String(255), nullable=True, doc='创建时间')

    used_at = Column(String(255), nullable=True, doc='使用时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'token': self.token,
            'file_id': self.file_id,
            'user_id': self.user_id,
            'share_link_id': self.share_link_id,
            'ip_address': self.ip_address,
            'is_used': self.is_used,
            'expires_at': self.expires_at,
            'created_at': self.created_at,
            'used_at': self.used_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<DownloadToken id={self.id}>'


