"""
SQLAlchemy 模型定义 - Capability
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class Capability(Base):
    """权限能力模型模型"""
    __tablename__ = 'capabilities'


    __table_args__ = (
        Index('idx_capabilities_codename', 'codename', unique=True),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='能力 ID')

    codename = Column(String(100), unique=True, nullable=True, doc='能力代码')

    name = Column(String(100), nullable=True, doc='能力名称')

    description = Column(String(255), nullable=True, doc='能力描述')

    module = Column(String(50), nullable=True, doc='所属模块')

    created_at = Column(String(255), nullable=True, doc='创建时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'codename': self.codename,
            'name': self.name,
            'description': self.description,
            'module': self.module,
            'created_at': self.created_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<Capability id={self.id}>'


