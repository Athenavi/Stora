"""
SQLAlchemy 模型定义 - TrashItem
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class TrashItem(Base):
    """回收站项目模型模型"""
    __tablename__ = 'trash_items'


    __table_args__ = (
        Index('idx_trash_items_user', 'user_id'),
        Index('idx_trash_items_expires', 'expires_at'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='回收站记录 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='用户 ID')


    file_id = Column(BigInteger, ForeignKey('file_items.id'), nullable=True, doc='原始文件 ID')


    folder_id = Column(BigInteger, ForeignKey('folders.id'), nullable=True, doc='原始文件夹 ID')


    original_path = Column(String(1000), nullable=True, doc='原始路径')

    original_name = Column(String(255), nullable=True, doc='原始文件名')

    file_size = Column(BigInteger, default=0, doc='文件大小')


    deleted_at = Column(String(255), nullable=True, doc='删除时间')

    expires_at = Column(String(255), nullable=True, doc='过期自动清理时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'file_id': self.file_id,
            'folder_id': self.folder_id,
            'original_path': self.original_path,
            'original_name': self.original_name,
            'file_size': self.file_size,
            'deleted_at': self.deleted_at,
            'expires_at': self.expires_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<TrashItem id={self.id}>'


