"""
SQLAlchemy 模型定义 - FileItem
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class FileItem(Base):
    """文件/存储项模型模型"""
    __tablename__ = 'file_items'


    __table_args__ = (
        Index('idx_file_items_user', 'user_id'),
        Index('idx_file_items_folder', 'folder_id'),
        Index('idx_file_items_user_folder', 'user_id', 'folder_id'),
        Index('idx_file_items_hash', 'file_hash'),
        Index('idx_file_items_file_type', 'file_type'),
        Index('idx_file_items_created', 'created_at'),
        Index('idx_file_items_favorite', 'user_id', 'is_favorite'),
        Index('idx_file_items_deleted', 'deleted_at'),
        Index('idx_file_items_user_filename', 'user_id', 'filename'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='文件 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='所有者用户 ID')


    folder_id = Column(BigInteger, ForeignKey('folders.id'), nullable=True, doc='所属文件夹 ID')


    filename = Column(String(255), nullable=True, doc='文件名')

    original_filename = Column(String(255), nullable=True, doc='原始文件名')

    file_path = Column(String(500), nullable=True, doc='存储路径')

    file_url = Column(String(500), nullable=True, doc='文件访问 URL')

    file_size = Column(BigInteger, default=0, doc='文件大小（字节）')


    mime_type = Column(String(100), nullable=True, doc='MIME 类型')

    file_type = Column(String(20), default='other', doc='文件类型 (image/video/audio/document/archive/other)')

    storage_driver = Column(String(50), default='local', doc='存储驱动 (local/s3/minio)')

    storage_bucket = Column(String(100), nullable=True, doc='存储桶名称')

    storage_key = Column(String(500), nullable=True, doc='存储驱动中的对象 key')

    file_hash = Column(String(64), nullable=True, doc='文件 SHA256 哈希')

    is_folder = Column(Boolean, default=False, doc='是否为文件夹')


    is_favorite = Column(Boolean, default=False, doc='是否收藏')


    is_encrypted = Column(Boolean, default=False, doc='是否加密存储')


    thumbnail_url = Column(String(500), nullable=True, doc='缩略图 URL')

    width = Column(Integer, nullable=True, doc='宽度（图片/视频）')


    height = Column(Integer, nullable=True, doc='高度（图片/视频）')


    duration = Column(Integer, nullable=True, doc='时长秒数（音视频）')


    description = Column(Text, nullable=True, doc='文件描述')


    download_count = Column(Integer, default=0, doc='下载次数')


    sort_order = Column(Integer, default=0, doc='排序顺序')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')

    deleted_at = Column(String(255), nullable=True, doc='删除时间（软删除）')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'folder_id': self.folder_id,
            'filename': self.filename,
            'original_filename': self.original_filename,
            'file_path': self.file_path,
            'file_url': self.file_url,
            'file_size': self.file_size,
            'mime_type': self.mime_type,
            'file_type': self.file_type,
            'storage_driver': self.storage_driver,
            'storage_bucket': self.storage_bucket,
            'storage_key': self.storage_key,
            'file_hash': self.file_hash,
            'is_folder': self.is_folder,
            'is_favorite': self.is_favorite,
            'is_encrypted': self.is_encrypted,
            'thumbnail_url': self.thumbnail_url,
            'width': self.width,
            'height': self.height,
            'duration': self.duration,
            'description': self.description,
            'download_count': self.download_count,
            'sort_order': self.sort_order,
            'created_at': self.created_at,
            'updated_at': self.updated_at,
            'deleted_at': self.deleted_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<FileItem id={self.id}>'


