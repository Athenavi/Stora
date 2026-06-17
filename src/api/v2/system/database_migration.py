"""
数据库 URL 替换 API
用于网站迁移时批量替换数据库中的URL
"""
from functools import wraps

from fastapi import APIRouter, Depends, Request, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession

from shared.services.system.database_url_replacer import database_url_replacer
from src.api.v2._helpers import ok, fail
from src.auth.auth_deps import admin_required as admin_required_api
from src.extensions import get_async_db_session as get_async_db

router = APIRouter(tags=["Migration"])


def _catch(func):
    @wraps(func)
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except HTTPException:
            raise
        except Exception as e:
            return fail(str(e))
    return wrapper


@router.post("/url-replace/preview")
@_catch
async def preview_url_replace(
        request: Request,
        current_user=Depends(admin_required_api),
        db: AsyncSession = Depends(get_async_db)
):
    """
    预览URL替换（不实际执行）
    """
    body = await request.json()
    search = body.get('search', '')
    replace = body.get('replace', '')
    tables = body.get('tables')
    exclude_tables = body.get('exclude_tables')
    use_regex = body.get('use_regex', False)
    case_sensitive = body.get('case_sensitive', True)

    if not search or not replace:
        return fail('search和replace参数不能为空')

    # 执行预览
    result = await database_url_replacer.search_replace(
        db=db,
        search=search,
        replace=replace,
        tables=tables,
        exclude_tables=exclude_tables,
        use_regex=use_regex,
        dry_run=True,
        case_sensitive=case_sensitive
    )

    return ok(data=result, msg=f"预览完成，将替换 {result.get('total_replacements', 0)} 处")


@router.post("/url-replace/execute")
@_catch
async def execute_url_replace(
        request: Request,
        current_user=Depends(admin_required_api),
        db: AsyncSession = Depends(get_async_db)
):
    """
    执行URL替换
    """
    body = await request.json()
    search = body.get('search', '')
    replace = body.get('replace', '')
    tables = body.get('tables')
    exclude_tables = body.get('exclude_tables')
    use_regex = body.get('use_regex', False)
    case_sensitive = body.get('case_sensitive', True)

    if not search or not replace:
        return fail('search和replace参数不能为空')

    # 执行替换
    result = await database_url_replacer.search_replace(
        db=db,
        search=search,
        replace=replace,
        tables=tables,
        exclude_tables=exclude_tables,
        use_regex=use_regex,
        dry_run=False,
        case_sensitive=case_sensitive
    )

    if result['success']:
        return ok(data=result, msg=f"成功替换 {result.get('total_replacements', 0)} 处")
    else:
        return fail(result.get('error', '替换失败'))


@router.post("/url-replace/validate")
@_catch
async def validate_url_replace(
        request: Request,
        current_user=Depends(admin_required_api),
        db: AsyncSession = Depends(get_async_db)
):
    """
    验证URL替换的正确性
    """
    body = await request.json()
    old_url = body.get('old_url', '')
    new_url = body.get('new_url', '')
    sample_size = body.get('sample_size', 10)

    if not old_url or not new_url:
        return fail('old_url和new_url参数不能为空')

    # 执行验证
    result = await database_url_replacer.validate_urls(
        db=db,
        old_url=old_url,
        new_url=new_url,
        sample_size=sample_size
    )

    return ok(data=result)


@router.get("/url-replace/common-patterns")
@_catch
async def get_common_patterns(
        current_user=Depends(admin_required_api)
):
    """
    获取常见的URL替换模式
    """
    patterns = {
        'domain_change': {
            'name': '域名变更',
            'description': '从旧域名迁移到新域名',
            'examples': [
                {
                    'search': 'http://old-domain.com',
                    'replace': 'https://new-domain.com',
                    'note': '同时处理HTTP到HTTPS的升级'
                },
                {
                    'search': 'old-domain.com',
                    'replace': 'new-domain.com',
                    'note': '仅替换域名部分'
                }
            ]
        },
        'protocol_upgrade': {
            'name': '协议升级',
            'description': '从HTTP升级到HTTPS',
            'examples': [
                {
                    'search': 'http://',
                    'replace': 'https://',
                    'note': '全局HTTP到HTTPS升级'
                }
            ]
        },
        'path_change': {
            'name': '路径变更',
            'description': '更改URL路径结构',
            'examples': [
                {
                    'search': '/blog/',
                    'replace': '/articles/',
                    'note': '更改博客文章路径'
                },
                {
                    'search': '/wp-content/',
                    'replace': '/media/',
                    'note': 'WordPress迁移时更改媒体路径'
                }
            ]
        },
        'port_change': {
            'name': '端口变更',
            'description': '更改服务器端口',
            'examples': [
                {
                    'search': ':8080',
                    'replace': ':443',
                    'note': '从开发端口改为生产端口'
                }
            ]
        },
        'regex_patterns': {
            'name': '正则表达式模式',
            'description': '使用正则表达式进行复杂替换',
            'examples': [
                {
                    'search': r'http://(\w+)\.old\.com',
                    'replace': r'https://\1.new.com',
                    'note': '使用捕获组进行动态替换',
                    'use_regex': True
                }
            ]
        }
    }

    return ok(data=patterns)
