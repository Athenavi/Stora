"""
SQLAlchemy 模型定义 - FileOptimization
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class FileOptimization(Base):
    """文件优化配置模型（缩略图、转码、CDN）模型"""
    __tablename__ = 'file_optimizations'


    __table_args__ = (
        Index('idx_file_optimizations_file', 'file_id'),
        Index('idx_file_optimizations_status', 'optimization_status'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='优化配置 ID')

    file_id = Column(BigInteger, ForeignKey('file_items.id'), doc='文件 ID')


    thumbnail_url = Column(String(500), nullable=True, doc='缩略图 URL')

    preview_url = Column(String(500), nullable=True, doc='预览 URL')

    cdn_url = Column(String(500), nullable=True, doc='CDN URL')

    sizes_json = Column(Text, nullable=True, doc='多尺寸 JSON')


    optimization_status = Column(String(20), default='pending', doc='优化状态 (pending/processing/completed/failed)')

    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'file_id': self.file_id,
            'thumbnail_url': self.thumbnail_url,
            'preview_url': self.preview_url,
            'cdn_url': self.cdn_url,
            'sizes_json': self.sizes_json,
            'optimization_status': self.optimization_status,
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
        return f'<FileOptimization id={self.id}>'


