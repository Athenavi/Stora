"""
FastBlog Default Theme
默认主题 - 简洁、现代、响应式设计
"""

from shared.services.plugins.plugin_manager.theme_plugin import ThemePlugin


class DefaultTheme(ThemePlugin):
    """FastBlog 默认主题"""

    def __init__(self):
        super().__init__(
            plugin_id=1001,
            name="FastBlog Default",
            slug="fastblog-default",
            version="1.0.0",
            description="FastBlog 默认主题 - 简洁、现代、响应式设计",
            author="FastBlog Team",
            author_url="https://athenavi.github.io",
        )

    def subscribers(self) -> list:
        """注册 EventBus 订阅"""
        return [
            ("theme.activated", self.on_theme_activated),
        ]

    def on_theme_activated(self, payload: dict):
        """当此主题被激活时的处理"""
        print(f"[DefaultTheme] 默认主题已激活: {payload}")


# 模块级实例（插件系统通过此变量发现）
plugin_instance = DefaultTheme()
