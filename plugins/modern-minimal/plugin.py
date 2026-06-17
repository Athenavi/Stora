"""
Modern Minimal Theme
现代简约风格主题 - 干净、优雅、响应式设计
"""

from shared.services.plugins.plugin_manager.theme_plugin import ThemePlugin


class ModernMinimalTheme(ThemePlugin):
    """Modern Minimal 主题"""

    def __init__(self):
        super().__init__(
            plugin_id=1003,
            name="Modern Minimal",
            slug="modern-minimal",
            version="2.0.0",
            description="现代简约风格主题 - 干净、优雅、响应式设计，支持代码高亮和深色模式",
            author="FastBlog Team",
            author_url="https://athenavi.github.io",
        )

    def subscribers(self) -> list:
        return [
            ("theme.activated", self.on_theme_activated),
        ]

    def on_theme_activated(self, payload: dict):
        print(f"[ModernMinimalTheme] 简约主题已激活: {payload}")


plugin_instance = ModernMinimalTheme()
