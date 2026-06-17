"""
时区和本地化 API
提供时区检测、日期格式化、货币格式化等功能
"""
from functools import wraps
from datetime import datetime
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Header

from shared.models.user import User as UserModel
from shared.services.translation.localization_service import localization_service
from src.api.v2._helpers import ok, fail, _catch
from src.auth.auth_deps import get_current_active_user

router = APIRouter(tags=["i18n"])


@router.get("/detect-timezone", summary="检测用户时区")
@_catch
async def detect_timezone(
        locale: str = Query(None, description="语言区域"),
        accept_language: Optional[str] = Header(None, description="Accept-Language头"),
):
    """检测用户时区"""
    detected_locale = locale
    if not detected_locale and accept_language:
        detected_locale = accept_language.split(',')[0].split(';')[0]
    timezone = localization_service.detect_timezone(locale=detected_locale)
    return ok(data={
        'timezone': timezone, 'locale': detected_locale or 'en-US',
        'offset': localization_service.get_timezone_offset(timezone),
    })


@router.get("/format-date", summary="格式化日期")
@_catch
async def format_date(
        date: str = Query(..., description="日期字符串"),
        from_format: str = Query("iso", description="输入格式"),
        to_format: str = Query("short", description="输出格式"),
        locale: str = Query("en-US", description="语言区域"),
):
    """格式化日期"""
    formatted = localization_service.format_date(date, from_format, to_format, locale)
    return ok(data={
        'original_date': date, 'formatted_date': formatted,
        'locale': locale, 'format': to_format,
    })


@router.get("/format-currency", summary="格式化货币")
@_catch
async def format_currency(
        amount: float = Query(..., description="金额"),
        currency: str = Query("USD", description="货币代码"),
        locale: str = Query("en-US", description="语言区域"),
):
    """格式化货币"""
    formatted = localization_service.format_currency(amount, currency, locale)
    return ok(data={
        'amount': amount, 'currency': currency,
        'formatted': formatted, 'locale': locale,
    })


@router.get("/format-number", summary="格式化数字")
@_catch
async def format_number(
        number: float = Query(..., description="要格式化的数字"),
        locale: str = Query("en-US", description="语言区域"),
        decimals: int = Query(2, ge=0, le=10, description="小数位数"),
):
    """格式化数字"""
    formatted = localization_service.format_number(number, locale, decimals)
    return ok(data={
        'number': number, 'formatted': formatted,
        'locale': locale, 'decimals': decimals,
    })


@router.get("/locales", summary="获取支持的语言列表")
@_catch
async def get_supported_locales():
    """获取系统支持的所有语言区域"""
    locales = localization_service.get_supported_locales()
    return ok(data={'locales': locales, 'count': len(locales)})


@router.post("/set-timezone", summary="设置用户时区")
@_catch
async def set_user_timezone(
        timezone: str = Query(..., description="时区名称"),
        current_user: UserModel = Depends(get_current_active_user)
):
    """设置当前用户的时区"""
    success = localization_service.set_user_timezone(current_user.id, timezone)
    if success:
        return ok(data={'timezone': timezone}, message='时区设置成功')
    return fail('时区设置失败')


@router.get("/timezone-list", summary="获取时区列表")
@_catch
async def get_timezone_list():
    """获取所有可用时区"""
    timezones = localization_service.get_timezone_list()
    return ok(data={'timezones': timezones, 'count': len(timezones)})
