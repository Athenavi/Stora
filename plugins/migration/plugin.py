"""
Migration Plugin
独立 SQLite 持久化，迁移任务和日志管理
"""
import json
from datetime import datetime
from typing import Optional

from sqlalchemy import Column, Integer, String, Boolean, DateTime, Text, Float
from sqlalchemy.orm import declarative_base, sessionmaker

from shared.services.plugins.plugin_manager import BasePlugin, requires_capability

MigrationBase = declarative_base()


class Task(MigrationBase):
    __tablename__ = 'migration_tasks'
    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(200), nullable=False)
    source_platform = Column(String(50), default='wordpress')
    config = Column(Text, default='{}')
    status = Column(String(20), default='pending')
    progress = Column(Integer, default=0)
    created_at = Column(DateTime, default=datetime.utcnow)

class Log(MigrationBase):
    __tablename__ = 'migration_logs'
    id = Column(Integer, primary_key=True, autoincrement=True)
    task_id = Column(Integer, nullable=True)
    task_name = Column(String(200), default='')
    level = Column(String(20), default='info')
    message = Column(Text, default='')
    created_at = Column(DateTime, default=datetime.utcnow)


class MigrationPlugin(BasePlugin):
    """内容迁移插件"""

    def __init__(self):
        super().__init__(
            plugin_id=2002,
            name="Migration",
            slug="migration",
            version="1.0.0",
            description="内容迁移工具：从 WordPress、Halo 等平台迁移内容到 FastBlog",
            author="FastBlog Team",
        )
        self._session_factory = None

    def _get_session(self):
        if self._session_factory is None:
            engine = self.get_db_engine()
            MigrationBase.metadata.create_all(engine)
            self._session_factory = sessionmaker(bind=engine)
        return self._session_factory()

    # ─── 任务 ──────────────────────────────────

    @requires_capability("read:migration")
    def list_tasks(self, page: int = 1, per_page: int = 20):
        s = self._get_session()
        try:
            q = s.query(Task).order_by(Task.created_at.desc())
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    @requires_capability("write:migration")
    def create_task(self, name: str, source_platform: str = 'wordpress', config: dict = None):
        s = self._get_session()
        try:
            t = Task(name=name, source_platform=source_platform, config=json.dumps(config or {}))
            s.add(t)
            s.commit()
            return {'success': True, 'data': self._r(t)}
        finally:
            s.close()

    @requires_capability("write:migration")
    def update_task(self, id: int, name: str = None, status: str = None, progress: int = None):
        s = self._get_session()
        try:
            t = s.query(Task).filter(Task.id == id).first()
            if not t:
                return {'success': False, 'message': '任务不存在'}
            if name: t.name = name
            if status: t.status = status
            if progress is not None: t.progress = progress
            s.commit()
            return {'success': True, 'data': self._r(t)}
        finally:
            s.close()

    @requires_capability("delete:migration")
    def delete_task(self, id: int):
        s = self._get_session()
        try:
            t = s.query(Task).filter(Task.id == id).first()
            if not t:
                return {'success': False, 'message': '任务不存在'}
            s.delete(t)
            s.commit()
            return {'success': True}
        finally:
            s.close()

    # ─── 日志 ──────────────────────────────────

    @requires_capability("read:migration")
    def list_logs(self, page: int = 1, per_page: int = 20, task_id: int = None, level: str = None):
        s = self._get_session()
        try:
            q = s.query(Log).order_by(Log.created_at.desc())
            if task_id:
                q = q.filter(Log.task_id == task_id)
            if level:
                q = q.filter(Log.level == level)
            total = q.count()
            items = q.offset((page - 1) * per_page).limit(per_page).all()
            return {'items': [self._r(r) for r in items], 'total': total, 'page': page}
        finally:
            s.close()

    # ─── EventBus ───────────────────────────────

    def subscribers(self) -> list:
        return []

    # ─── 辅助 ───────────────────────────────────

    @staticmethod
    def _r(row):
        d = {c.name: getattr(row, c.name) for c in row.__table__.columns}
        if isinstance(d.get('created_at'), datetime):
            d['created_at'] = d['created_at'].isoformat()
        return d


plugin_instance = MigrationPlugin()
