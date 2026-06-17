"""
安装向导 — 站点设置模块
"""
from pathlib import Path
from typing import Dict, Any

from src.unified_logger import default_logger as logger


def configure_site_settings(project_root: Path, settings: Dict[str, str]) -> Dict[str, Any]:
    """配置站点基本信息"""
    site_name = settings.get("site_name", "FastBlog").strip()
    site_url = settings.get("site_url", "http://localhost:9421").strip()
    site_desc = settings.get("site_description", "").strip()
    lang = settings.get("default_language", "zh-CN").strip()

    if not site_name:
        return {"success": False, "message": "站点名称不能为空"}

    if not site_url.startswith(("http://", "https://")):
        return {"success": False, "message": "站点URL必须以 http:// 或 https:// 开头"}

    # 验证 URL 格式，防止路径遍历
    from urllib.parse import urlparse
    parsed = urlparse(site_url)
    if not parsed.netloc or ".." in parsed.path or ".." in parsed.netloc:
        return {"success": False, "message": "站点URL格式无效，包含非法路径"}
    if "\\" in site_url or "\x00" in site_url:
        return {"success": False, "message": "站点URL包含非法字符"}

    try:
        from src.utils.database.main import get_async_session_context
        from shared.models.system import SystemSettings
        import asyncio

        async def _save():
            async with get_async_session_context() as db:
                pairs = [
                    ("site_name", site_name),
                    ("site_url", site_url.rstrip("/")),
                    ("site_description", site_desc),
                    ("default_language", lang),
                ]
                for key, value in pairs:
                    query = __import__("sqlalchemy").select(SystemSettings).where(SystemSettings.setting_key == key)
                    result = await db.execute(query)
                    setting = result.scalar_one_or_none()
                    if setting:
                        setting.setting_value = value
                    else:
                        db.add(SystemSettings(setting_key=key, setting_value=value))
                await db.commit()

        asyncio.get_event_loop().run_until_complete(_save())
        return {"success": True, "message": "站点设置已保存"}
    except Exception as e:
        logger.exception(f"保存站点设置失败")
        return {"success": False, "message": f"保存失败: {str(e)}"}
