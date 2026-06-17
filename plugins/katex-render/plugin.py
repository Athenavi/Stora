"""
KaTeX 公式渲染插件
在评论中渲染 $...$ 和 $$...$$ LaTeX 数学公式
"""

import re
from typing import Dict, Any

from shared.services.plugins.plugin_manager.core import BasePlugin
from shared.services.plugins.event_bus import event_bus


class KatexRenderPlugin(BasePlugin):
    """
    KaTeX 公式渲染插件

    通过 comment.content 管道处理评论内容，将 $...$ 和 $$...$$
    转换为 KaTeX 兼容的 HTML 标记。配合前端 KaTeX 库自动渲染。

    用法:
        $E = mc^2$         → 行内公式
        $$\\sum_{i=1}^n i$$ → 块级公式
    """

    def __init__(self):
        super().__init__(
            plugin_id=3001,
            name="KaTeX Render",
            slug="katex-render",
            version="1.0.0",
        )

        self.settings = {
            'enabled': True,
            # 行内公式定界符
            'inline_delimiter': '$',
            # 块级公式定界符
            'display_delimiter': '$$',
        }

    # ── EventBus ──

    def subscribers(self) -> list:
        return [
            ("comment.content", self.render_math_in_content, "pipeline"),
        ]

    def activate(self):
        super().activate()
        print("[KaTeX] Plugin activated")

    def deactivate(self):
        super().deactivate()
        print("[KaTeX] Plugin deactivated")

    # ── 管道处理 ──

    def render_math_in_content(self, content: Any) -> Any:
        """
        处理 comment.content 管道数据。

        输入可以是字符串（纯文本）或 dict（含 content/html 键）。
        将 $...$ 替换为 KaTeX inline 标记，$$...$$ 替换为 display 标记。
        """
        if not self.settings.get('enabled', True):
            return content

        # 提取文本
        if isinstance(content, str):
            text = content
            return self._process(text)
        elif isinstance(content, dict):
            text = content.get('content', '') or content.get('html', '')
            processed = self._process(text)
            if 'content' in content:
                content['content'] = processed
            else:
                content['html'] = processed
            return content
        return content

    def _process(self, text: str) -> str:
        """将 $...$ 转换为 KaTeX 兼容 HTML"""
        if not text:
            return text

        # 先处理 $$...$$（块级，防止与 $ 冲突）
        text = re.sub(
            r'\$\$(.+?)\$\$',
            r'<div class="katex-block">\[\1\]</div>',
            text,
            flags=re.DOTALL,
        )

        # 再处理 $...$（行内）
        text = re.sub(
            r'\$(.+?)\$',
            r'<span class="katex-inline">\(\1\)</span>',
            text,
        )

        return text

    def get_settings_ui(self) -> Dict[str, Any]:
        return {
            'fields': [
                {
                    'key': 'enabled',
                    'type': 'boolean',
                    'label': '启用公式渲染',
                },
            ],
        }


plugin_instance = KatexRenderPlugin()
