"""
SQLAlchemy 模型定义 - Folder
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class Folder(Base):
    """文件夹模型模型"""
    __tablename__ = 'folders'


    __table_args__ = (
        Index('idx_folders_user', 'user_id'),
        Index('idx_folders_parent', 'parent_id'),
        Index('idx_folders_user_parent', 'user_id', 'parent_id'),
        Index('idx_folders_user_name', 'user_id', 'parent_id', 'name', unique=True),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='文件夹 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='所有者用户 ID')


    parent_id = Column(BigInteger, ForeignKey('folders.id'), nullable=True, doc='父文件夹 ID')


    name = Column(String(255), nullable=True, doc='文件夹名称')

    description = Column(String(255), nullable=True, doc='文件夹描述')

    color = Column(String(10), nullable=True, doc='文件夹颜色标记')

    icon = Column(String(50), nullable=True, doc='文件夹图标')

    is_shared = Column(Boolean, default=False, doc='是否已共享')


    sort_order = Column(Integer, default=0, doc='排序顺序')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'parent_id': self.parent_id,
            'name': self.name,
            'description': self.description,
            'color': self.color,
            'icon': self.icon,
            'is_shared': self.is_shared,
            'sort_order': self.sort_order,
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
        return f'<Folder id={self.id}>'


