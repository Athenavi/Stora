"""
SQLAlchemy 模型定义 - AuditLog
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class AuditLog(Base):
    """操作审计日志模型模型"""
    __tablename__ = 'audit_logs'


    __table_args__ = (
        Index('idx_audit_logs_user_id', 'user_id'),
        Index('idx_audit_logs_action', 'action'),
        Index('idx_audit_logs_created_at', 'created_at'),
        Index('idx_audit_logs_resource', 'resource_type', 'resource_id'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='日志 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), nullable=True, doc='操作用户 ID')


    user_name = Column(String(100), nullable=True, doc='操作用户名称')

    level = Column(String(20), default='info', doc='日志级别 (info/warning/error/critical)')

    description = Column(Text, nullable=True, doc='日志详细描述')


    action = Column(String(100), nullable=True, doc='操作动作')

    resource_type = Column(String(50), nullable=True, doc='资源类型')

    resource_id = Column(BigInteger, nullable=True, doc='资源 ID')


    ip_address = Column(String(45), nullable=True, doc='操作 IP 地址')

    user_agent = Column(Text, nullable=True, doc='用户代理字符串')


    request_data = Column(Text, nullable=True, doc='请求数据快照 (JSON)')


    status = Column(String(20), default='success', doc='操作状态 (success/failure)')

    error_message = Column(Text, nullable=True, doc='错误信息')


    created_at = Column(String(255), nullable=True, doc='创建时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'user_name': self.user_name,
            'level': self.level,
            'description': self.description,
            'action': self.action,
            'resource_type': self.resource_type,
            'resource_id': self.resource_id,
            'ip_address': self.ip_address,
            'user_agent': self.user_agent,
            'request_data': self.request_data,
            'status': self.status,
            'error_message': self.error_message,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<AuditLog id={self.id}>'


