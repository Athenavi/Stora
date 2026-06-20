"""
SQLAlchemy 模型定义 - TranscriptionTask
"""
from sqlalchemy import Column, BigInteger, String, Text, Integer, DateTime, Index

from shared.models import Base


class TranscriptionTask(Base):
    """语音转字幕任务模型"""
    __tablename__ = 'transcription_tasks'

    __table_args__ = (
        Index('idx_transcription_task_file', 'file_id'),
        Index('idx_transcription_task_user', 'user_id'),
        Index('idx_transcription_task_status', 'status'),
    )

    id = Column(BigInteger, primary_key=True, autoincrement=True, doc='任务 ID')

    file_id = Column(BigInteger, nullable=False, doc='关联文件 ID')

    user_id = Column(BigInteger, nullable=False, doc='创建者用户 ID')

    status = Column(String(20), default='pending', doc='状态: pending/processing/completed/failed')

    language = Column(String(10), nullable=True, doc='检测到的语言代码 (zh/en/ja...)')

    subtitle_path = Column(String(500), nullable=True, doc='生成的 SRT 字幕文件路径')

    subtitle_format = Column(String(10), default='srt', doc='字幕格式: srt/vtt')

    error_message = Column(Text, nullable=True, doc='错误信息')

    progress = Column(Integer, default=0, doc='进度百分比 0-100')

    created_at = Column(DateTime, nullable=True, doc='创建时间')

    updated_at = Column(DateTime, nullable=True, doc='更新时间')
