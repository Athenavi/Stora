"""
SQLAlchemy 模型定义 - SensitiveWord
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class SensitiveWord(Base):
    """敏感词模型模型"""
    __tablename__ = 'sensitive_words'


    __table_args__ = (
        Index('idx_sensitive_words_word', 'word', unique=True),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='敏感词 ID')

    word = Column(String(200), nullable=True, doc='敏感词内容')

    replacement = Column(String(200), nullable=True, doc='替换内容')

    level = Column(Integer, default=1, doc='级别')


    is_active = Column(Boolean, default=True, doc='是否激活')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'word': self.word,
            'replacement': self.replacement,
            'level': self.level,
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
        return f'<SensitiveWord id={self.id}>'


