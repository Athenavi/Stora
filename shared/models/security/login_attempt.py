"""
SQLAlchemy 模型定义 - LoginAttempt
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class LoginAttempt(Base):
    """登录尝试记录模型模型"""
    __tablename__ = 'login_attempts'


    __table_args__ = (
        Index('idx_login_attempts_user', 'user_id'),
        Index('idx_login_attempts_ip', 'ip_address'),
        Index('idx_login_attempts_created', 'created_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='记录 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), nullable=True, doc='用户 ID')


    username = Column(String(255), nullable=True, doc='尝试登录的用户名')

    ip_address = Column(String(45), nullable=True, doc='IP 地址')

    user_agent = Column(Text, nullable=True, doc='用户代理')


    success = Column(Boolean, default=False, doc='是否成功')


    failure_reason = Column(String(255), nullable=True, doc='失败原因')

    created_at = Column(String(255), nullable=True, doc='尝试时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'username': self.username,
            'ip_address': self.ip_address,
            'user_agent': self.user_agent,
            'success': self.success,
            'failure_reason': self.failure_reason,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<LoginAttempt id={self.id}>'


