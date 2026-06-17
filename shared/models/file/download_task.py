"""
SQLAlchemy 模型定义 - DownloadTask
由代码生成器自动生成 (基于 models.yaml / routes.yaml) - 请勿手动修改
生成时间：2026-06-17 15:54:31
"""

from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index

from shared.models import Base  # 使用统一的 Base（跨子包引用）



class DownloadTask(Base):
    """下载任务模型模型"""
    __tablename__ = 'download_tasks'


    __table_args__ = (
        Index('idx_download_tasks_user', 'user_id'),
        Index('idx_download_tasks_status', 'status'),
    )


    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='任务 ID')

    user_id = Column(BigInteger, ForeignKey('users.id'), doc='用户 ID')


    file_id = Column(BigInteger, ForeignKey('file_items.id'), nullable=True, doc='文件 ID')


    download_token = Column(String(128), nullable=True, doc='下载令牌')

    status = Column(String(20), default='pending', doc='状态 (pending/downloading/completed/failed/cancelled)')

    progress = Column(Integer, default=0, doc='进度 (0-100)')


    error_message = Column(Text, nullable=True, doc='错误信息')


    source_url = Column(String(2048), nullable=True, doc='源 URL（离线下载）')

    total_size = Column(BigInteger, nullable=True, doc='总大小')


    downloaded_size = Column(BigInteger, default=0, doc='已下载大小')


    created_at = Column(String(255), nullable=True, doc='创建时间')

    updated_at = Column(String(255), nullable=True, doc='更新时间')

    completed_at = Column(String(255), nullable=True, doc='完成时间')


    def to_dict(self, exclude_sensitive=True):
        """转换为字典

        Args:
            exclude_sensitive: 是否排除敏感字段（密码、密钥、token 等）
        """
        data = {
            'id': self.id,
            'user_id': self.user_id,
            'file_id': self.file_id,
            'download_token': self.download_token,
            'status': self.status,
            'progress': self.progress,
            'error_message': self.error_message,
            'source_url': self.source_url,
            'total_size': self.total_size,
            'downloaded_size': self.downloaded_size,
            'created_at': self.created_at,
            'updated_at': self.updated_at,
            'completed_at': self.completed_at,
        }

        if not exclude_sensitive:
            sensitive_data = {
            }
            data.update(sensitive_data)

        return data

    def __repr__(self):
        """字符串表示"""
        return f'<DownloadTask id={self.id}>'


