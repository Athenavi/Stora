"""
代码片段收藏插件
允许用户收藏和分享代码片段，支持多种编程语言高亮
数据使用本地 SQLite 持久化，不依赖主数据库
"""

import re
import time
from datetime import datetime, timezone
from typing import Dict, List, Any, Optional

from sqlalchemy import Column, Integer, String, Text, DateTime, create_engine
from sqlalchemy.orm import declarative_base, sessionmaker

from shared.services.plugins.plugin_manager.core import BasePlugin
from shared.services.plugins.event_bus import event_bus

# ── 插件本地 ORM 模型（SQLite，独立于主库）──
SnippetBase = declarative_base()


class SnippetModel(SnippetBase):
    __tablename__ = "snippets"

    id = Column(Integer, primary_key=True, autoincrement=True)
    title = Column(String(255), nullable=False, default="Untitled")
    code = Column(Text, nullable=False)
    language = Column(String(50), nullable=False, default="python")
    description = Column(Text, default="")
    tags = Column(String(500), default="")  # 逗号分隔
    visibility = Column(String(20), nullable=False, default="private")  # public / private / unlisted
    user_id = Column(Integer, nullable=False, index=True)
    view_count = Column(Integer, default=0)
    embed_count = Column(Integer, default=0)
    created_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))
    updated_at = Column(DateTime, default=lambda: datetime.now(timezone.utc), onupdate=lambda: datetime.now(timezone.utc))


class CodeSnippetsPlugin(BasePlugin):
    """
    代码片段收藏插件

    功能:
    1. 代码片段管理（增删改查）
    2. 多语言语法高亮
    3. 代码片段分类标签
    4. 公开/私有设置
    5. 搜索和过滤
    6. 嵌入代码功能
    """

    def __init__(self):
        super().__init__(
            plugin_id=0,
            name="代码片段收藏",
            slug="code-snippets",
            version="1.0.0"
        )

        self.settings = {
            'enable_snippets': True,
            'allow_public_sharing': True,
            'allow_embedding': True,
            'max_snippet_length': 10000,
            'supported_languages': [
                'python', 'javascript', 'typescript', 'java', 'cpp', 'c',
                'html', 'css', 'sql', 'bash', 'json', 'yaml', 'markdown'
            ],
            'default_language': 'python',
            'enable_syntax_highlighting': True,
            'theme': 'default',
        }

        self._session_factory = None

    # ── 会话管理 ──
    def _get_session(self):
        """获取插件本地 SQLite 会话"""
        if self._session_factory is None:
            engine = self.get_db_engine()
            self._session_factory = sessionmaker(bind=engine)
        return self._session_factory()

    # ── 插件生命周期 ──
    def subscribers(self) -> list:
        return [
            ("article.content", self.embed_snippets_in_content, "pipeline"),
        ]

    def register_hooks(self):
        pass

    def activate(self):
        super().activate()
        self.init_db(SnippetBase)
        print("[CodeSnippets] Plugin activated")

    def deactivate(self):
        super().deactivate()
        if self._session_factory:
            self._session_factory.close_all_sessions()
            self._session_factory = None
        print("[CodeSnippets] Plugin deactivated")

    # ── CRUD ──
    def create_snippet(self, snippet_data: Dict[str, Any]) -> Dict[str, Any]:
        if not self.settings.get('enable_snippets'):
            raise Exception("代码片段功能已禁用")

        validation = self._validate_snippet(snippet_data)
        if not validation['valid']:
            raise ValueError(validation['error'])

        tags = snippet_data.get('tags', [])
        if isinstance(tags, list):
            tags_str = ','.join(tags)
        else:
            tags_str = str(tags)

        model = SnippetModel(
            title=snippet_data.get('title', 'Untitled'),
            code=snippet_data.get('code', ''),
            language=snippet_data.get('language', self.settings.get('default_language', 'python')),
            description=snippet_data.get('description', ''),
            tags=tags_str,
            visibility=snippet_data.get('visibility', 'private'),
            user_id=snippet_data.get('user_id', 0),
        )

        session = self._get_session()
        try:
            session.add(model)
            session.commit()
            result = self._model_to_dict(model)
            print(f"[CodeSnippets] Created snippet: {result['id']}")
            return result
        except Exception as e:
            session.rollback()
            raise
        finally:
            session.close()

    def update_snippet(self, snippet_id: int, updates: Dict[str, Any], user_id: int) -> bool:
        session = self._get_session()
        try:
            model = session.query(SnippetModel).filter_by(id=snippet_id).first()
            if not model:
                return False
            if model.user_id != user_id:
                raise PermissionError("无权修改此代码片段")

            if 'title' in updates:
                model.title = updates['title']
            if 'code' in updates:
                model.code = updates['code']
            if 'language' in updates:
                model.language = updates['language']
            if 'description' in updates:
                model.description = updates['description']
            if 'tags' in updates:
                tags = updates['tags']
                model.tags = ','.join(tags) if isinstance(tags, list) else str(tags)
            if 'visibility' in updates:
                model.visibility = updates['visibility']

            model.updated_at = datetime.now(timezone.utc)
            session.commit()
            return True
        except Exception as e:
            session.rollback()
            raise
        finally:
            session.close()

    def delete_snippet(self, snippet_id: int, user_id: int) -> bool:
        session = self._get_session()
        try:
            model = session.query(SnippetModel).filter_by(id=snippet_id).first()
            if not model:
                return False
            if model.user_id != user_id:
                raise PermissionError("无权删除此代码片段")
            session.delete(model)
            session.commit()
            print(f"[CodeSnippets] Deleted snippet: {snippet_id}")
            return True
        except Exception as e:
            session.rollback()
            raise
        finally:
            session.close()

    def get_snippet(self, snippet_id: int, user_id: Optional[int] = None) -> Optional[Dict[str, Any]]:
        session = self._get_session()
        try:
            model = session.query(SnippetModel).filter_by(id=snippet_id).first()
            if not model:
                return None
            if model.visibility == 'private' and model.user_id != user_id:
                return None

            model.view_count += 1
            session.commit()

            return self._model_to_dict(model)
        except Exception as e:
            session.rollback()
            raise
        finally:
            session.close()

    def get_user_snippets(self, user_id: int, limit: int = 20, offset: int = 0) -> List[Dict[str, Any]]:
        session = self._get_session()
        try:
            models = (session.query(SnippetModel)
                      .filter_by(user_id=user_id)
                      .order_by(SnippetModel.created_at.desc())
                      .offset(offset).limit(limit)
                      .all())
            return [self._model_to_dict(m) for m in models]
        finally:
            session.close()

    def search_snippets(self, query: str, language: Optional[str] = None,
                        tags: Optional[List[str]] = None, limit: int = 20) -> List[Dict[str, Any]]:
        session = self._get_session()
        try:
            q = session.query(SnippetModel).filter(
                SnippetModel.visibility.in_(['public', 'unlisted'])
            )

            if query:
                like = f"%{query}%"
                q = q.filter(
                    SnippetModel.title.ilike(like) |
                    SnippetModel.description.ilike(like)
                )

            if language:
                q = q.filter(SnippetModel.language == language)

            if tags:
                for tag in tags:
                    q = q.filter(SnippetModel.tags.ilike(f"%{tag}%"))

            models = q.order_by(SnippetModel.view_count.desc()).limit(limit).all()
            return [self._model_to_dict(m) for m in models]
        finally:
            session.close()

    def get_popular_snippets(self, limit: int = 10) -> List[Dict[str, Any]]:
        session = self._get_session()
        try:
            models = (session.query(SnippetModel)
                      .filter(SnippetModel.visibility.in_(['public', 'unlisted']))
                      .order_by(SnippetModel.view_count.desc())
                      .limit(limit)
                      .all())
            return [self._model_to_dict(m) for m in models]
        finally:
            session.close()

    def get_snippets_by_tag(self, tag: str, limit: int = 20) -> List[Dict[str, Any]]:
        session = self._get_session()
        try:
            models = (session.query(SnippetModel)
                      .filter(
                SnippetModel.visibility.in_(['public', 'unlisted']),
                SnippetModel.tags.ilike(f"%{tag}%")
            )
                      .limit(limit)
                      .all())
            return [self._model_to_dict(m) for m in models]
        finally:
            session.close()

    def get_all_tags(self) -> List[str]:
        """获取所有标签（去重）"""
        session = self._get_session()
        try:
            rows = session.query(SnippetModel.tags).filter(
                SnippetModel.tags != '',
                SnippetModel.visibility.in_(['public', 'unlisted'])
            ).all()
            tag_set: set = set()
            for (tags_str,) in rows:
                for t in tags_str.split(','):
                    t = t.strip()
                    if t:
                        tag_set.add(t)
            return sorted(tag_set)
        finally:
            session.close()

    # ── 嵌入功能 ──
    def generate_embed_code(self, snippet_id: int) -> str:
        if not self.settings.get('allow_embedding'):
            return '<!-- Embedding is disabled -->'

        session = self._get_session()
        try:
            model = session.query(SnippetModel).filter_by(id=snippet_id).first()
            if not model:
                return '<!-- Snippet not found -->'

            model.embed_count += 1
            session.commit()

            code_escaped = (model.code
                            .replace('&', '&amp;')
                            .replace('<', '&lt;')
                            .replace('>', '&gt;')
                            .replace('"', '&quot;')
                            .replace("'", '&#39;'))

            return f'''
<div class="code-snippet-embed" data-snippet-id="{model.id}">
    <div class="snippet-header">
        <span class="snippet-title">{model.title}</span>
        <span class="snippet-language">{model.language}</span>
    </div>
    <pre class="snippet-code"><code class="language-{model.language}">{code_escaped}</code></pre>
</div>'''
        finally:
            session.close()

    def embed_snippets_in_content(self, content_data):
        # 兼容 str（纯 HTML）和 dict（含 html 键）两种输入
        if isinstance(content_data, str):
            html = content_data
            dict_mode = False
        else:
            html = content_data.get('html', '')
            dict_mode = True

        pattern = r'\[snippet:(\d+)\]'

        def replace_snippet(match):
            snippet_id = int(match.group(1))
            return self.generate_embed_code(snippet_id)

        replaced = re.sub(pattern, replace_snippet, html)

        if dict_mode:
            content_data['html'] = replaced
            return content_data
        return replaced

    # ── 工具方法 ──
    @staticmethod
    def _model_to_dict(model: SnippetModel) -> Dict[str, Any]:
        tags_list = [t.strip() for t in model.tags.split(',') if t.strip()] if model.tags else []
        return {
            'id': model.id,
            'title': model.title,
            'code': model.code,
            'language': model.language,
            'description': model.description,
            'tags': tags_list,
            'visibility': model.visibility,
            'user_id': model.user_id,
            'view_count': model.view_count,
            'embed_count': model.embed_count,
            'created_at': model.created_at.isoformat() if model.created_at else None,
            'updated_at': model.updated_at.isoformat() if model.updated_at else None,
        }

    def _validate_snippet(self, snippet_data: Dict[str, Any]) -> Dict[str, Any]:
        if not snippet_data.get('code'):
            return {'valid': False, 'error': '代码不能为空'}

        max_length = self.settings.get('max_snippet_length', 10000)
        if len(snippet_data['code']) > max_length:
            return {'valid': False, 'error': f'代码长度不能超过{max_length}字符'}

        language = snippet_data.get('language', self.settings.get('default_language'))
        supported = self.settings.get('supported_languages', [])
        if language and language not in supported:
            return {'valid': False, 'error': f'不支持的编程语言: {language}'}

        return {'valid': True, 'error': ''}

    def get_settings_ui(self) -> Dict[str, Any]:
        return {
            'fields': [
                {'key': 'enable_snippets', 'type': 'boolean', 'label': '启用代码片段'},
                {'key': 'allow_public_sharing', 'type': 'boolean', 'label': '允许公开分享'},
                {'key': 'allow_embedding', 'type': 'boolean', 'label': '允许嵌入'},
                {'key': 'max_snippet_length', 'type': 'number', 'label': '最大代码长度', 'min': 1000, 'max': 50000},
                {'key': 'enable_syntax_highlighting', 'type': 'boolean', 'label': '启用语法高亮'},
            ],
            'actions': [
                {'type': 'button', 'label': '查看热门片段', 'action': 'view_popular', 'variant': 'outline'},
            ]
        }


# 插件实例
plugin_instance = CodeSnippetsPlugin()
