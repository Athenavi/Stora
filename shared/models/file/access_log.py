"""
SQLAlchemy 模型定义 - AccessLog
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class AccessLog(Base):
    """文件访问日志模型模型"""
    __tablename__ = 'access_logs'


    __table_args__ = (
        Index('idx_access_logs_file', 'file_id'),
        Index('idx_access_logs_user', 'user_id'),
        Index('idx_access_logs_action', 'action'),
        Index('idx_access_logs_created', 'created_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='日志 ID')

    file_id = Column(BigInteger, ForeignKey('file_items.id'), nullable=True, doc='文件 ID')


    user_id = Column(BigInteger, ForeignKey('users.id'), nullable=True, doc='访问用户 ID')


    anonymous_id = Column(String(64), nullable=True, doc='匿名访问标识')

    action = Column(String(50), nullable=True, doc='访问动作 (view/download/upload/delete/share/preview)')

    ip_address = Column(String(45), nullable=True, doc='访问 IP')

    user_agent = Column(Text, nullable=True, doc='用户代理')


    referrer = Column(String(500), nullable=True, doc='来源页面')

    duration_ms = Column(Integer, nullable=True, doc='访问耗时（毫秒）')


    success = Column(Boolean, default=True, doc='是否成功')


    created_at = Column(String(255), nullable=True, doc='访问时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'file_id': self.file_id,
            'user_id': self.user_id,
            'anonymous_id': self.anonymous_id,
            'action': self.action,
            'ip_address': self.ip_address,
            'user_agent': self.user_agent,
            'referrer': self.referrer,
            'duration_ms': self.duration_ms,
            'success': self.success,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<AccessLog id={self.id}>'


