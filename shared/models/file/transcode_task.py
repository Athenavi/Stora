"""
SQLAlchemy 模型定义 - TranscodeTask
由代码生成器自动生成 - 请勿手动修改
"""
from sqlalchemy import Column, Integer, BigInteger, String, Text, Boolean, DateTime, ForeignKey, Index
from shared.models import Base


class TranscodeTask(Base):
    """视频转码任务模型"""
    __tablename__ = 'transcode_tasks'

    __table_args__ = (
        Index('idx_transcode_file', 'file_id'),
        Index('idx_transcode_user', 'user_id'),
        Index('idx_transcode_status', 'status'),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='任务 ID')
    file_id = Column(BigInteger, ForeignKey('file_items.id'), doc='源文件ID')
    user_id = Column(BigInteger, ForeignKey('users.id'), doc='用户ID')
    status = Column(String(20), default='pending', doc='pending/processing/completed/failed')
    progress = Column(Integer, default=0, doc='进度 (0-100)')
    source_resolution = Column(String(20), nullable=True, doc='原始分辨率')
    output_files = Column(Text, nullable=True, doc='JSON [{label,path,size}]')
    error_message = Column(Text, nullable=True, doc='错误信息')
    created_at = Column(String(255), nullable=True, doc='创建时间')
    completed_at = Column(String(255), nullable=True, doc='完成时间')

    def to_dict(self, exclude_sensitive=True):
        import json
        return {
            'id': self.id, 'file_id': self.file_id, 'user_id': self.user_id,
            'status': self.status, 'progress': self.progress,
            'source_resolution': self.source_resolution,
            'output_files': json.loads(self.output_files) if self.output_files else [],
            'error_message': self.error_message,
            'created_at': self.created_at, 'completed_at': self.completed_at,
        }
