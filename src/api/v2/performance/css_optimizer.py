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
from src.api.v2.auth_v1pack import get_current_user


def _check_admin(user):
    if not (getattr(user, 'is_superuser', False) or getattr(user, 'is_staff', False)):
        raise HTTPException(status_code=403, detail="Permission denied")


router = APIRouter()


@router.post("/extract", summary="提取关键CSS",
             description="从HTML内容中提取Above-the-fold关键CSS(仅管理员)")
@_catch
async def extract_critical_css_api(
        request: Request, data: Dict[str, Any] = Body(...),
        current_user: User = Depends(get_current_user)
):
    """提取关键CSS API"""
    _check_admin(current_user)
    html_content = data.get('html_content', '')
    css_files = data.get('css_files', [])
    page_type = data.get('page_type', 'article')
    if not html_content:
        return fail('缺少HTML内容')
    result = css_optimizer_service.optimize_page_css(html_content, css_files, page_type)
    return ok(data=result)
