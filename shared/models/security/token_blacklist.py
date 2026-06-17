"""
SQLAlchemy 模型定义 - TokenBlacklist
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class TokenBlacklist(Base):
    """Token 黑名单模型模型"""
    __tablename__ = 'token_blacklist'


    __table_args__ = (
        Index('idx_token_blacklist_jti', 'token_jti', unique=True),
        Index('idx_token_blacklist_expires', 'expires_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='记录 ID')

    token_jti = Column(String(128), nullable=True, doc='Token JTI')

    token_type = Column(String(20), default='access', doc='Token 类型 (access/refresh)')

    user_id = Column(BigInteger, ForeignKey('users.id'), nullable=True, doc='用户 ID')


    expires_at = Column(String(255), nullable=True, doc='Token 过期时间')

    blacklisted_at = Column(String(255), nullable=True, doc='加入黑名单时间')

    reason = Column(String(255), nullable=True, doc='加入黑名单原因')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'token_jti': self.token_jti,
            'token_type': self.token_type,
            'user_id': self.user_id,
            'expires_at': self.expires_at,
            'blacklisted_at': self.blacklisted_at,
            'reason': self.reason,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<TokenBlacklist id={self.id}>'


