"""
Magazine Theme
杂志风格主题 - 网格布局、特色图片展示、多栏目支持
"""

from shared.services.plugins.plugin_manager.theme_plugin import ThemePlugin


class MagazineTheme(ThemePlugin):
    """Magazine 主题"""

    def __init__(self):
        super().__init__(
            plugin_id=1002,
            name="Magazine",
            slug="magazine",
            version="1.0.0",
            description="杂志风格主题 - 网格布局、特色图片展示、多栏目支持",
            author="FastBlog Team",
            author_url="https://athenavi.github.io",
        )

    def subscribers(self) -> list:
        return [
            ("theme.activated", self.on_theme_activated),
        ]

    def on_theme_activated(self, payload: dict):
        print(f"[MagazineTheme] 杂志主题已激活: {payload}")


plugin_instance = MagazineTheme()
