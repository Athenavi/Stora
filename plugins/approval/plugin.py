"""
Content Approval Plugin
独立 SQLite 持久化，通过 action 端点与系统交互
"""
import json
from datetime import datetime
from pathlib import Path
from typing import Optional

from sqlalchemy import Column, Integer, String, Boolean, DateTime, Text, create_engine
from sqlalchemy.orm import declarative_base, sessionmaker

from shared.services.plugins.plugin_manager import BasePlugin, requires_capability

ApprovalBase = declarative_base()


class ApprovalRecord(ApprovalBase):
    """审批记录"""
    __tablename__ = 'approval_records'
    id = Column(Integer, primary_key=True, autoincrement=True)
    content_type = Column(String(50), nullable=False)
    content_id = Column(Integer, nullable=False)
    content_title = Column(String(200), default='')
    applicant_id = Column(Integer, nullable=False)
    applicant_name = Column(String(100), default='')
    status = Column(String(20), default='pending')        # pending/approved/rejected/cancelled
    current_level = Column(Integer, default=1)
    max_level = Column(Integer, default=1)
    notes = Column(Text, default='')
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class ContentApprovalPlugin(BasePlugin):
    """内容审批插件"""

    def __init__(self):
        super().__init__(
            plugin_id=2003,
            name="Content Approval",
            slug="approval",
            version="1.0.0",
            description="内容审批工作流：待审批、我的申请、统计历史",
            author="FastBlog Team",
        )
        self._session_factory = None

    def _get_session(self):
        """获取插件本地 SQLite 会话"""
        if self._session_factory is None:
            engine = self.get_db_engine()
            ApprovalBase.metadata.create_all(engine)
            self._session_factory = sessionmaker(bind=engine)
        return self._session_factory()

    # ─── 公开方法（通过 action 端点调用）──────────

    @requires_capability("read:approval")
    def list_pending(self, page: int = 1, per_page: int = 15, content_type: str = None):
        """获取待审批列表"""
        session = self._get_session()
        try:
            q = session.query(ApprovalRecord).filter(ApprovalRecord.status == 'pending')
            if content_type:
                q = q.filter(ApprovalRecord.content_type == content_type)
            total = q.count()
            items = q.order_by(ApprovalRecord.created_at.desc()).offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._row_to_dict(r) for r in items], 'total': total, 'page': page}
        finally:
            session.close()

    @requires_capability("read:approval")
    def list_my_requests(self, page: int = 1, per_page: int = 15, status: str = None, applicant_id: int = 0):
        """获取我的申请"""
        session = self._get_session()
        try:
            q = session.query(ApprovalRecord).filter(ApprovalRecord.applicant_id == applicant_id)
            if status and status != 'all':
                q = q.filter(ApprovalRecord.status == status)
            total = q.count()
            items = q.order_by(ApprovalRecord.created_at.desc()).offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._row_to_dict(r) for r in items], 'total': total, 'page': page}
        finally:
            session.close()

    @requires_capability("read:approval")
    def get_stats(self):
        """获取审批统计"""
        session = self._get_session()
        try:
            total = session.query(ApprovalRecord).count()
            pending = session.query(ApprovalRecord).filter(ApprovalRecord.status == 'pending').count()
            approved = session.query(ApprovalRecord).filter(ApprovalRecord.status == 'approved').count()
            rejected = session.query(ApprovalRecord).filter(ApprovalRecord.status == 'rejected').count()
            return {'total_pending': pending, 'total_approved': approved, 'total_rejected': rejected, 'total': total}
        finally:
            session.close()

    @requires_capability("write:approval")
    def create_request(self, content_type: str, content_id: int, content_title: str = '',
                       applicant_id: int = 0, applicant_name: str = '', max_level: int = 1):
        """创建审批请求"""
        session = self._get_session()
        try:
            record = ApprovalRecord(
                content_type=content_type, content_id=content_id,
                content_title=content_title, applicant_id=applicant_id,
                applicant_name=applicant_name, max_level=max_level,
            )
            session.add(record)
            session.commit()
            return self._row_to_dict(record)
        finally:
            session.close()

    @requires_capability("write:approval")
    def approve(self, record_id: int, notes: str = ''):
        """审批通过"""
        session = self._get_session()
        try:
            record = session.query(ApprovalRecord).filter(ApprovalRecord.id == record_id).first()
            if not record:
                return {'success': False, 'message': '记录不存在'}
            record.status = 'approved'
            if notes:
                record.notes = notes
            record.updated_at = datetime.utcnow()
            session.commit()
            return {'success': True, 'data': self._row_to_dict(record)}
        finally:
            session.close()

    @requires_capability("write:approval")
    def reject(self, record_id: int, notes: str = ''):
        """拒绝"""
        session = self._get_session()
        try:
            record = session.query(ApprovalRecord).filter(ApprovalRecord.id == record_id).first()
            if not record:
                return {'success': False, 'message': '记录不存在'}
            record.status = 'rejected'
            if notes:
                record.notes = notes
            record.updated_at = datetime.utcnow()
            session.commit()
            return {'success': True, 'data': self._row_to_dict(record)}
        finally:
            session.close()

    @requires_capability("write:approval")
    def cancel(self, record_id: int):
        """取消审批"""
        session = self._get_session()
        try:
            record = session.query(ApprovalRecord).filter(ApprovalRecord.id == record_id).first()
            if not record:
                return {'success': False, 'message': '记录不存在'}
            record.status = 'cancelled'
            record.updated_at = datetime.utcnow()
            session.commit()
            return {'success': True}
        finally:
            session.close()

    # ─── EventBus ───────────────────────────────

    def subscribers(self) -> list:
        return []

    # ─── 辅助方法 ───────────────────────────────

    @staticmethod
    def _row_to_dict(row):
        return {
            'id': row.id,
            'content_type': row.content_type,
            'content_id': row.content_id,
            'content_title': row.content_title,
            'applicant_id': row.applicant_id,
            'applicant_name': row.applicant_name,
            'status': row.status,
            'current_level': row.current_level,
            'max_level': row.max_level,
            'notes': row.notes,
            'created_at': row.created_at.isoformat() if row.created_at else None,
            'updated_at': row.updated_at.isoformat() if row.updated_at else None,
        }


plugin_instance = ContentApprovalPlugin()
