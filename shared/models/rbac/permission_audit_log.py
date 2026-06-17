"""
SQLAlchemy 模型定义 - PermissionAuditLog
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class PermissionAuditLog(Base):
    """权限审计日志模型模型"""
    __tablename__ = 'permission_audit_logs'


    __table_args__ = (
        Index('idx_perm_audit_user', 'user_id'),
        Index('idx_perm_audit_action', 'action'),
        Index('idx_perm_audit_created', 'created_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='日志 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='操作用户 ID')


    action = Column(String(50), nullable=True, doc='操作类型')

    target_type = Column(String(50), nullable=True, doc='目标类型')

    target_id = Column(BigInteger, nullable=True, doc='目标 ID')


    details = Column(Text, nullable=True, doc='操作详情')


    created_at = Column(String(255), nullable=True, doc='创建时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'action': self.action,
            'target_type': self.target_type,
            'target_id': self.target_id,
            'details': self.details,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<PermissionAuditLog id={self.id}>'


