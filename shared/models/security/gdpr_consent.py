"""
SQLAlchemy 模型定义 - GDPRConsent
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class GDPRConsent(Base):
    """GDPR 用户同意记录模型模型"""
    __tablename__ = 'gdpr_consents'


    __table_args__ = (
        Index('idx_gdpr_consents_user', 'user_id'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='记录 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='用户 ID')


    consent_type = Column(String(100), nullable=True, doc='同意类型')

    granted = Column(Boolean, default=True, doc='是否同意')


    ip_address = Column(String(45), nullable=True, doc='IP 地址')

    created_at = Column(String(255), nullable=True, doc='创建时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'consent_type': self.consent_type,
            'granted': self.granted,
            'ip_address': self.ip_address,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<GDPRConsent id={self.id}>'


