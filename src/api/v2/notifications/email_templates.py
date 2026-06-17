"""
邮件模板管理 API — 获取/保存自定义邮件 HTML 模板
模板存储在 system_settings 中，key 为 email_template_{name}
"""

from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from shared.models.system.system_settings import SystemSettings
from src.api.v2._helpers import ok, fail
from src.auth import jwt_required_dependency as jwt_required
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["email-templates"])

DEFAULT_TEMPLATES = {
    "welcome": {
        "label": "欢迎邮件",
        "subject": "欢迎来到 {{ site_name }}",
        "html": "<!DOCTYPE html><html><head><meta charset='utf-8'></head><body style='font-family:Arial,sans-serif;padding:20px;background:#f5f5f5'><div style='max-width:600px;margin:auto;background:white;border-radius:12px;padding:30px'><h1 style='color:#333'>欢迎, {{ username }}!</h1><p style='color:#666'>感谢注册 {{ site_name }}，开始你的创作之旅。</p><a href='{{ site_url }}' style='display:inline-block;padding:12px 24px;background:#3b82f6;color:white;border-radius:8px;text-decoration:none'>开始使用</a></div></body></html>",
    },
    "comment_notification": {
        "label": "评论通知",
        "subject": "{{ author }} 回复了你的评论",
        "html": "<!DOCTYPE html><html><head><meta charset='utf-8'></head><body style='font-family:Arial,sans-serif;padding:20px;background:#f5f5f5'><div style='max-width:600px;margin:auto;background:white;border-radius:12px;padding:30px'><h2 style='color:#333'>新的评论回复</h2><p style='color:#666'><strong>{{ author }}</strong> 在文章 <strong>{{ article_title }}</strong> 中回复了你:</p><div style='background:#f9fafb;border-radius:8px;padding:15px;margin:15px 0;border-left:4px solid #3b82f6'>{{ comment_content }}</div><a href='{{ comment_url }}' style='display:inline-block;padding:12px 24px;background:#3b82f6;color:white;border-radius:8px;text-decoration:none'>查看回复</a></div></body></html>",
    },
    "newsletter": {
        "label": "新闻通讯",
        "subject": "{{ site_name }} - {{ article_title }}",
        "html": "<!DOCTYPE html><html><head><meta charset='utf-8'></head><body style='font-family:Arial,sans-serif;padding:20px;background:#f5f5f5'><div style='max-width:600px;margin:auto;background:white;border-radius:12px;padding:30px'><h1 style='color:#333'>{{ article_title }}</h1><p style='color:#666'>{{ article_excerpt }}</p><a href='{{ article_url }}' style='display:inline-block;padding:12px 24px;background:#3b82f6;color:white;border-radius:8px;text-decoration:none'>阅读全文</a><p style='margin-top:30px;font-size:12px;color:#999'><a href='{{ unsubscribe_url }}'>退订</a></p></div></body></html>",
    },
}


@router.get("")
async def list_templates(
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """获取所有邮件模板"""
    try:
        result = await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key.like("email_template_%"))
        )
        stored = {row.setting_key.replace("email_template_", ""): row.setting_value for row in result.scalars().all()}

        templates = []
        for name, default in DEFAULT_TEMPLATES.items():
            templates.append({
                "name": name,
                "label": default["label"],
                "subject": default["subject"],
                "html": stored.get(name, default["html"]),
                "is_custom": name in stored,
            })

        return ok(data={"templates": templates})
    except Exception as e:
        return fail(str(e))


@router.get("/{name}")
async def get_template(
    name: str,
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """获取单个邮件模板"""
    try:
        if name not in DEFAULT_TEMPLATES:
            return fail("模板不存在")

        result = await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key == f"email_template_{name}")
        )
        stored = result.scalar_one_or_none()
        default = DEFAULT_TEMPLATES[name]

        return ok(data={
            "name": name,
            "label": default["label"],
            "subject": stored.setting_value if stored else default["subject"],
            "html": stored.setting_value if stored else default["html"],
            "is_custom": stored is not None,
        })
    except Exception as e:
        return fail(str(e))


@router.put("/{name}")
async def save_template(
    name: str,
    subject: str = "",
    html: str = "",
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """保存邮件模板"""
    try:
        if name not in DEFAULT_TEMPLATES:
            return fail("模板不存在")

        # 查找已有记录
        result = await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key == f"email_template_{name}")
        )
        setting = result.scalar_one_or_none()

        if setting:
            setting.setting_value = html
        else:
            setting = SystemSettings(
                setting_key=f"email_template_{name}",
                setting_value=html,
                setting_type="text",
                description=f"Email template: {DEFAULT_TEMPLATES[name]['label']}",
            )
            db.add(setting)

        await db.commit()
        return ok(data={"message": f"模板「{DEFAULT_TEMPLATES[name]['label']}」已保存"})
    except Exception as e:
        await db.rollback()
        return fail(str(e))


@router.delete("/{name}/reset")
async def reset_template(
    name: str,
    db: AsyncSession = Depends(get_async_db),
    _=Depends(jwt_required),
):
    """重置邮件模板为默认"""
    try:
        if name not in DEFAULT_TEMPLATES:
            return fail("模板不存在")

        result = await db.execute(
            select(SystemSettings).where(SystemSettings.setting_key == f"email_template_{name}")
        )
        setting = result.scalar_one_or_none()
        if setting:
            await db.delete(setting)
            await db.commit()

        return ok(data={"message": f"模板「{DEFAULT_TEMPLATES[name]['label']}」已重置"})
    except Exception as e:
        await db.rollback()
        return fail(str(e))
