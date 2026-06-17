"""
Enterprise Plugin
独立 SQLite 持久化，许可证/工单/脚本/日志/告警管理
"""
import json
from datetime import datetime
from typing import Optional

from sqlalchemy import Column, Integer, String, Boolean, DateTime, Text, Float
from sqlalchemy.orm import declarative_base, sessionmaker

from shared.services.plugins.plugin_manager import BasePlugin, requires_capability

EnterpriseBase = declarative_base()


class License(EnterpriseBase):
    __tablename__ = 'enterprise_licenses'
    id = Column(Integer, primary_key=True, autoincrement=True)
    license_key = Column(String(200), unique=True, nullable=False)
    is_active = Column(Boolean, default=True)
    expires_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)

class Ticket(EnterpriseBase):
    __tablename__ = 'enterprise_tickets'
    id = Column(Integer, primary_key=True, autoincrement=True)
    title = Column(String(200), nullable=False)
    status = Column(String(20), default='open')   # open/in_progress/resolved/closed
    priority = Column(String(20), default='normal')
    created_at = Column(DateTime, default=datetime.utcnow)

class Script(EnterpriseBase):
    __tablename__ = 'enterprise_scripts'
    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(200), nullable=False)
    version = Column(String(20), default='1.0')
    content = Column(Text, default='')
    created_at = Column(DateTime, default=datetime.utcnow)

class DeployLog(EnterpriseBase):
    __tablename__ = 'enterprise_deploy_logs'
    id = Column(Integer, primary_key=True, autoincrement=True)
    script_id = Column(Integer, nullable=True)
    script_name = Column(String(200), default='')
    status = Column(String(20), default='pending')  # pending/success/failed
    error_message = Column(Text, default='')
    created_at = Column(DateTime, default=datetime.utcnow)

class Alert(EnterpriseBase):
    __tablename__ = 'enterprise_alerts'
    id = Column(Integer, primary_key=True, autoincrement=True)
    message = Column(String(500), nullable=False)
    severity = Column(String(20), default='info')  # info/warning/critical
    resolved = Column(Boolean, default=False)
    created_at = Column(DateTime, default=datetime.utcnow)


class EnterprisePlugin(BasePlugin):
    """企业功能插件"""

    def __init__(self):
        super().__init__(
            plugin_id=2001,
            name="Enterprise",
            slug="enterprise",
            version="1.0.0",
            description="企业管理功能：许可证、支持工单、部署脚本、监控告警",
            author="FastBlog Team",
        )
        self._session_factory = None

    def _get_session(self):
        if self._session_factory is None:
            engine = self.get_db_engine()
            EnterpriseBase.metadata.create_all(engine)
            self._session_factory = sessionmaker(bind=engine)
        return self._session_factory()

    # ─── 概览 ──────────────────────────────────

    @requires_capability("read:enterprise")
    def get_overview(self):
        s = self._get_session()
        try:
            return {
                'total_licenses': s.query(License).count(),
                'active_licenses': s.query(License).filter(License.is_active == True).count(),
                'open_tickets': s.query(Ticket).filter(Ticket.status == 'open').count(),
                'in_progress_tickets': s.query(Ticket).filter(Ticket.status == 'in_progress').count(),
                'total_scripts': s.query(Script).count(),
                'total_deployments': s.query(DeployLog).count(),
                'failed_deployments': s.query(DeployLog).filter(DeployLog.status == 'failed').count(),
                'unresolved_alerts': s.query(Alert).filter(Alert.resolved == False).count(),
            }
        finally:
            s.close()

    # ─── 许可证 ────────────────────────────────

    @requires_capability("read:enterprise")
    def list_licenses(self, page: int = 1, per_page: int = 20):
        s = self._get_session()
        try:
            q = s.query(License).order_by(License.created_at.desc())
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    # ─── 工单 ──────────────────────────────────

    @requires_capability("read:enterprise")
    def list_tickets(self, page: int = 1, per_page: int = 20):
        s = self._get_session()
        try:
            q = s.query(Ticket).order_by(Ticket.created_at.desc())
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    @requires_capability("write:enterprise")
    def update_ticket(self, id: int, status: str = None, priority: str = None):
        s = self._get_session()
        try:
            t = s.query(Ticket).filter(Ticket.id == id).first()
            if not t:
                return {'success': False, 'message': '工单不存在'}
            if status: t.status = status
            if priority: t.priority = priority
            s.commit()
            return {'success': True, 'data': self._r(t)}
        finally:
            s.close()

    # ─── 脚本 ──────────────────────────────────

    @requires_capability("read:enterprise")
    def list_scripts(self, page: int = 1, per_page: int = 20):
        s = self._get_session()
        try:
            q = s.query(Script).order_by(Script.created_at.desc())
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    # ─── 部署日志 ──────────────────────────────

    @requires_capability("read:enterprise")
    def list_logs(self, page: int = 1, per_page: int = 20):
        s = self._get_session()
        try:
            q = s.query(DeployLog).order_by(DeployLog.created_at.desc())
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    # ─── 告警 ──────────────────────────────────

    @requires_capability("read:enterprise")
    def list_alerts(self, page: int = 1, per_page: int = 20):
        s = self._get_session()
        try:
            q = s.query(Alert).filter(Alert.resolved == False).order_by(Alert.created_at.desc())
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    @requires_capability("write:enterprise")
    def resolve_alert(self, id: int):
        s = self._get_session()
        try:
            a = s.query(Alert).filter(Alert.id == id).first()
            if not a:
                return {'success': False, 'message': '告警不存在'}
            a.resolved = True
            s.commit()
            return {'success': True}
        finally:
            s.close()

    @requires_capability("write:enterprise")
    def delete_alert(self, id: int):
        s = self._get_session()
        try:
            a = s.query(Alert).filter(Alert.id == id).first()
            if not a:
                return {'success': False, 'message': '告警不存在'}
            s.delete(a)
            s.commit()
            return {'success': True}
        finally:
            s.close()

    # ─── EventBus ───────────────────────────────

    def subscribers(self) -> list:
        return []

    # ─── 辅助 ───────────────────────────────────

    @staticmethod
    def _r(row):
        d = {c.name: getattr(row, c.name) for c in row.__table__.columns}
        for k in ('created_at', 'expires_at'):
            if isinstance(d.get(k), datetime):
                d[k] = d[k].isoformat()
        return d


plugin_instance = EnterprisePlugin()
