"""
Article Likes Plugin
独立 SQLite 持久化，点赞/取消点赞/状态/热门排行
"""
from datetime import datetime, timezone
from typing import Dict, Any, Optional

from sqlalchemy import Column, Integer, BigInteger, DateTime, func
from sqlalchemy.orm import declarative_base, sessionmaker

from shared.services.plugins.plugin_manager.core import BasePlugin
from shared.services.plugins.plugin_manager import requires_capability

LikesBase = declarative_base()


class LikeModel(LikesBase):
    __tablename__ = "article_likes"
    id = Column(Integer, primary_key=True, autoincrement=True)
    user_id = Column(BigInteger, nullable=False, index=True)
    article_id = Column(BigInteger, nullable=False, index=True)
    created_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))


class ArticleLikesPlugin(BasePlugin):
    """文章点赞插件"""

    def __init__(self):
        super().__init__(
            plugin_id=3003,
            name="Article Likes",
            slug="article-likes",
            version="1.0.0",
        )
        self.settings = {'enabled': True}
        self._session_factory = None

    def _get_session(self):
        if self._session_factory is None:
            engine = self.get_db_engine()
            self._session_factory = sessionmaker(bind=engine)
        return self._session_factory()

    def activate(self):
        super().activate()
        self.init_db(LikesBase)

    def deactivate(self):
        super().deactivate()
        if self._session_factory:
            self._session_factory.close_all_sessions()
            self._session_factory = None

    def subscribers(self) -> list:
        return []

    @requires_capability("write:article-likes")
    def like(self, article_id: int, user_id: int) -> Dict[str, Any]:
        session = self._get_session()
        try:
            existing = session.query(LikeModel).filter_by(user_id=user_id, article_id=article_id).first()
            if existing:
                return {'success': True, 'liked': True, 'count': self._count(session, article_id)}
            session.add(LikeModel(user_id=user_id, article_id=article_id))
            session.commit()
            return {'success': True, 'liked': True, 'count': self._count(session, article_id)}
        except Exception as e:
            session.rollback()
            return {'success': False, 'error': str(e)}
        finally:
            session.close()

    @requires_capability("write:article-likes")
    def unlike(self, article_id: int, user_id: int) -> Dict[str, Any]:
        session = self._get_session()
        try:
            existing = session.query(LikeModel).filter_by(user_id=user_id, article_id=article_id).first()
            if not existing:
                return {'success': True, 'liked': False, 'count': self._count(session, article_id)}
            session.delete(existing)
            session.commit()
            return {'success': True, 'liked': False, 'count': self._count(session, article_id)}
        except Exception as e:
            session.rollback()
            return {'success': False, 'error': str(e)}
        finally:
            session.close()

    @requires_capability("read:article-likes")
    def status(self, article_id: int, user_id: Optional[int] = 0) -> Dict[str, Any]:
        session = self._get_session()
        try:
            liked = bool(user_id and session.query(LikeModel).filter_by(user_id=user_id, article_id=article_id).first())
            return {'success': True, 'article_id': article_id, 'liked': liked, 'count': self._count(session, article_id)}
        finally:
            session.close()

    @requires_capability("read:article-likes")
    def popular(self, limit: int = 10) -> Dict[str, Any]:
        session = self._get_session()
        try:
            rows = session.query(LikeModel.article_id, func.count(LikeModel.id).label('cnt')) \
                .group_by(LikeModel.article_id).order_by(func.count(LikeModel.id).desc()).limit(limit).all()
            return {'success': True, 'data': [{'article_id': r.article_id, 'likes': r.cnt} for r in rows]}
        finally:
            session.close()

    @staticmethod
    def _count(session, article_id: int) -> int:
        return session.query(func.count(LikeModel.id)).filter_by(article_id=article_id).scalar() or 0


plugin_instance = ArticleLikesPlugin()
