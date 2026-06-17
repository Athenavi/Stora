"""
CSS优化API
提供关键CSS提取和缓存管理功能
"""
from functools import wraps
from typing import Dict, Any

from fastapi import APIRouter, Depends, HTTPException, Request, Body

from shared.models.user import User
from shared.services.performance.css_optimizer import css_optimizer_service
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import admin_required as admin_required_api

router = APIRouter()


@router.post("/extract", summary="提取关键CSS",
             description="从HTML内容中提取Above-the-fold关键CSS(仅管理员)")
@_catch
async def extract_critical_css_api(
        request: Request, data: Dict[str, Any] = Body(...),
        current_user: User = Depends(admin_required_api)
):
    """提取关键CSS API"""
    html_content = data.get('html_content', '')
    css_files = data.get('css_files', [])
    page_type = data.get('page_type', 'article')
    if not html_content:
        return fail('缺少HTML内容')
    result = css_optimizer_service.optimize_page_css(html_content, css_files, page_type)
    return ok(data=result)
