"""
Newsletter 订阅插件
提供邮件订阅功能：
- 公开订阅/退订接口
- 新文章发布时自动推送
- 管理员后台管理订阅者 + 手动发送
- 数据使用本地 SQLite 持久化
"""

import smtplib
from datetime import datetime, timezone
from email.mime.text import MIMEText
from typing import Dict, List, Any, Optional

from sqlalchemy import Column, Integer, String, DateTime, Boolean, Text, create_engine
from sqlalchemy.orm import declarative_base, sessionmaker

from shared.services.plugins.plugin_manager.core import BasePlugin
from shared.services.plugins.plugin_manager import requires_capability
from shared.services.plugins.event_bus import event_bus, ArticlePublishedPayload

# ── 插件本地 ORM ──
NewsletterBase = declarative_base()


class SubscriberModel(NewsletterBase):
    __tablename__ = "subscribers"

    id = Column(Integer, primary_key=True, autoincrement=True)
    email = Column(String(255), nullable=False, unique=True, index=True)
    name = Column(String(100), default="")
    source = Column(String(50), default="homepage")  # homepage / admin / embed
    subscribed_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))
    unsubscribed_at = Column(DateTime, nullable=True)
    is_active = Column(Boolean, default=True)
    confirm_token = Column(String(64), default="")  # 可选双确认


class NewsletterPlugin(BasePlugin):
    """
    Newsletter 邮件订阅插件

    功能:
    1. 公开订阅（输入邮箱即可）
    2. 一键退订
    3. 新文章自动推送
    4. 管理员手动发送邮件
    5. 订阅者列表管理
    """

    def __init__(self):
        super().__init__(
            plugin_id=3002,
            name="Newsletter",
            slug="newsletter",
            version="1.0.0"
        )

        self.settings = {
            'enabled': True,
            # SMTP 配置（可选，不配置则只记录订阅不发邮件）
            'smtp_host': '',
            'smtp_port': 587,
            'smtp_user': '',
            'smtp_password': '',
            'smtp_from_email': '',
            'smtp_from_name': 'FastBlog',
            'site_url': '',
            # 自动推送
            'auto_send_on_publish': True,
            'email_template': '''
<h2>{title}</h2>
<p>{excerpt}</p>
<p><a href="{url}">阅读全文 →</a></p>
<hr/>
<p style="color:#999;font-size:12px;">
  如果你不再想收到这些邮件，请<a href="{unsubscribe_url}">点击这里退订</a>
</p>
'''.strip(),
        }

        self._session_factory = None

    def _get_session(self):
        if self._session_factory is None:
            engine = self.get_db_engine()
            self._session_factory = sessionmaker(bind=engine)
        return self._session_factory()

    def subscribers(self) -> list:
        return [
            ("article.published", self.on_article_published),
        ]

    def activate(self):
        super().activate()
        self.init_db(NewsletterBase)
        print("[Newsletter] Plugin activated")

    def deactivate(self):
        super().deactivate()
        if self._session_factory:
            self._session_factory.close_all_sessions()
            self._session_factory = None
        print("[Newsletter] Plugin deactivated")

    # ── 公开 API 动作 ──

    def subscribe(self, email: str, name: str = "", source: str = "homepage") -> Dict[str, Any]:
        """订阅邮件"""
        if not email or '@' not in email:
            return {'success': False, 'error': 'Invalid email'}

        session = self._get_session()
        try:
            existing = session.query(SubscriberModel).filter_by(email=email).first()
            if existing:
                if existing.is_active:
                    return {'success': True, 'message': 'Already subscribed'}
                # 重新激活
                existing.is_active = True
                existing.unsubscribed_at = None
                existing.source = source
                session.commit()
                return {'success': True, 'message': 'Subscribed again'}
            else:
                sub = SubscriberModel(email=email, name=name, source=source)
                session.add(sub)
                session.commit()
                return {'success': True, 'message': 'Subscription successful'}
        except Exception as e:
            session.rollback()
            return {'success': False, 'error': str(e)}
        finally:
            session.close()

    def unsubscribe(self, email: str) -> Dict[str, Any]:
        """退订"""
        session = self._get_session()
        try:
            sub = session.query(SubscriberModel).filter_by(email=email).first()
            if not sub:
                return {'success': False, 'error': 'Email not found'}
            sub.is_active = False
            sub.unsubscribed_at = datetime.now(timezone.utc)
            session.commit()
            return {'success': True, 'message': 'Unsubscribed successfully'}
        except Exception as e:
            session.rollback()
            return {'success': False, 'error': str(e)}
        finally:
            session.close()

    @requires_capability("read:newsletter")
    def list_subscribers(self, page: int = 1, per_page: int = 50) -> Dict[str, Any]:
        """列出订阅者（仅活跃）"""
        session = self._get_session()
        try:
            offset = (page - 1) * per_page
            total = session.query(SubscriberModel).filter_by(is_active=True).count()
            subs = (session.query(SubscriberModel)
                    .filter_by(is_active=True)
                    .order_by(SubscriberModel.subscribed_at.desc())
                    .offset(offset).limit(per_page)
                    .all())
            return {
                'success': True,
                'data': [self._model_to_dict(s) for s in subs],
                'total': total,
                'page': page,
                'per_page': per_page,
            }
        finally:
            session.close()

    def stats(self) -> Dict[str, Any]:
        """订阅统计"""
        session = self._get_session()
        try:
            total = session.query(SubscriberModel).count()
            active = session.query(SubscriberModel).filter_by(is_active=True).count()
            return {
                'success': True,
                'total': total,
                'active': active,
                'unsubscribed': total - active,
            }
        finally:
            session.close()

    @requires_capability("write:newsletter")
    def admin_unsubscribe(self, subscriber_id: int) -> Dict[str, Any]:
        """管理员从后台取消订阅"""
        session = self._get_session()
        try:
            sub = session.query(SubscriberModel).filter_by(id=subscriber_id).first()
            if not sub:
                return {'success': False, 'error': 'Subscriber not found'}
            sub.is_active = False
            sub.unsubscribed_at = datetime.now(timezone.utc)
            session.commit()
            return {'success': True, 'message': f'{sub.email} unsubscribed'}
        except Exception as e:
            session.rollback()
            return {'success': False, 'error': str(e)}
        finally:
            session.close()

    # ── EventBus 事件处理 ──

    async def on_article_published(self, payload: ArticlePublishedPayload):
        """新文章发布时自动发送邮件给所有订阅者"""
        if not self.settings.get('enabled') or not self.settings.get('auto_send_on_publish'):
            return
        if not self.settings.get('smtp_host'):
            print("[Newsletter] SMTP not configured, skipping auto-send")
            return

        session = self._get_session()
        try:
            subscribers = session.query(SubscriberModel).filter_by(is_active=True).all()
            if not subscribers:
                return

            site_url = self.settings.get('site_url', 'https://fastblog.example.com').rstrip('/')
            article_url = f"{site_url}/blog/detail?slug={payload.slug}"
            unsubscribe_url = f"{site_url}/unsubscribe?email="
            html = self.settings['email_template'].format(
                title=payload.title,
                excerpt=payload.excerpt or payload.title,
                url=article_url,
                unsubscribe_url=unsubscribe_url,
            )

            emails = [s.email for s in subscribers]
            print(f"[Newsletter] Sending to {len(emails)} subscribers for article: {payload.title}")

            # 分批发送
            batch_size = 50
            for i in range(0, len(emails), batch_size):
                batch = emails[i:i + batch_size]
                for email in batch:
                    try:
                        self._send_email(email, f"[FastBlog] {payload.title}", html)
                    except Exception as e:
                        print(f"[Newsletter] Failed to send to {email}: {e}")
        finally:
            session.close()

    # ── 邮箱发送 ──

    def _send_email(self, to_email: str, subject: str, html: str) -> bool:
        """发送单封邮件（同步）"""
        host = self.settings['smtp_host']
        if not host:
            return False

        msg = MIMEText(html, 'html', 'utf-8')
        msg['Subject'] = subject
        msg['From'] = f"{self.settings['smtp_from_name']} <{self.settings['smtp_from_email']}>"
        msg['To'] = to_email
        msg['List-Unsubscribe'] = f"<mailto:{self.settings['smtp_from_email']}?subject=unsubscribe>"

        try:
            with smtplib.SMTP(host, self.settings['smtp_port']) as server:
                server.starttls()
                if self.settings['smtp_user']:
                    server.login(self.settings['smtp_user'], self.settings['smtp_password'])
                server.send_message(msg)
            return True
        except Exception as e:
            print(f"[Newsletter] SMTP error: {e}")
            return False

    # ── 工具方法 ──

    @staticmethod
    def _model_to_dict(m: SubscriberModel) -> Dict[str, Any]:
        return {
            'id': m.id,
            'email': m.email,
            'name': m.name,
            'source': m.source,
            'is_active': m.is_active,
            'subscribed_at': m.subscribed_at.isoformat() if m.subscribed_at else None,
            'unsubscribed_at': m.unsubscribed_at.isoformat() if m.unsubscribed_at else None,
        }

    def get_settings_ui(self) -> Dict[str, Any]:
        return {
            'fields': [
                {'key': 'enabled', 'type': 'boolean', 'label': '启用 Newsletter'},
                {'key': 'auto_send_on_publish', 'type': 'boolean', 'label': '新文章自动推送'},
                {'key': 'smtp_host', 'type': 'text', 'label': 'SMTP 服务器'},
                {'key': 'smtp_port', 'type': 'number', 'label': 'SMTP 端口'},
                {'key': 'smtp_user', 'type': 'text', 'label': 'SMTP 用户名'},
                {'key': 'smtp_password', 'type': 'password', 'label': 'SMTP 密码'},
                {'key': 'smtp_from_email', 'type': 'text', 'label': '发件邮箱'},
                {'key': 'smtp_from_name', 'type': 'text', 'label': '发件名称'},
            ],
            'actions': [
                {'type': 'button', 'label': '查看统计', 'action': 'stats', 'variant': 'outline'},
            ]
        }


# 插件实例
plugin_instance = NewsletterPlugin()
