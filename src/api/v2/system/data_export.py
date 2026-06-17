"""
数据导出 API
提供CSV、Excel等格式的数据导出功能
"""

from functools import wraps
from typing import Optional

from fastapi import APIRouter, Depends, Query, Response, HTTPException

from shared.services.system.data_export_service import data_export_service
from src.api.v2._helpers import ok, fail

from shared.models.user import User as UserModel
from src.auth.auth_deps import admin_required as admin_required_api

router = APIRouter(tags=["export"])


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


@router.get("/users", summary="导出用户列表")
@_catch
async def export_users(
        format: str = Query('csv', enum=['csv', 'excel'], description="导出格式"),
        limit: int = Query(1000, ge=1, le=10000, description="导出数量"),
        current_user: UserModel = Depends(admin_required_api)
):
    """
    导出用户列表为CSV或Excel文件
    
    Args:
        format: 导出格式(csv/excel)
        limit: 导出数量(1-10000)
        
    Returns:
        文件下载
    """
    # Query real user data from database
    # Example implementation:
    # from shared.models.user import User
    # from sqlalchemy import select, func
    # 
    # stmt = select(User).limit(limit)
    # result = await db.execute(stmt)
    # users_db = result.scalars().all()
    # 
    # users = [{
    #     'id': user.id,
    #     'username': user.username,
    #     'email': user.email,
    #     'phone': user.phone,
    #     'is_active': user.is_active,
    #     'is_verified': user.is_verified,
    #     'created_at': user.created_at.isoformat() if user.created_at else None,
    #     'last_login': user.last_login.isoformat() if user.last_login else None,
    #     'article_count': await get_user_article_count(user.id),
    #     'follower_count': await get_user_follower_count(user.id),
    # } for user in users_db]

    # For now, use sample data for demonstration
    users = [
        {
            'id': i,
            'username': f'user_{i}',
            'email': f'user_{i}@example.com',
            'phone': f'1380000{i:04d}',
            'is_active': True,
            'is_verified': True,
            'created_at': '2024-01-01T00:00:00',
            'last_login': '2024-01-15T10:30:00',
            'article_count': 10,
            'follower_count': 100,
        }
        for i in range(1, min(limit + 1, 11))
    ]

    # 导出数据
    if format == 'excel':
        file_bytes = data_export_service.export_user_list(users, format='excel')
        media_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        filename = 'users.xlsx'
    else:
        file_bytes = data_export_service.export_user_list(users, format='csv')
        media_type = 'text/csv'
        filename = 'users.csv'

    return Response(
        content=file_bytes,
        media_type=media_type,
        headers={
            'Content-Disposition': f'attachment; filename="{filename}"',
        }
    )


@router.get("/articles", summary="导出文章列表")
@_catch
async def export_articles(
        format: str = Query('csv', enum=['csv', 'excel'], description="导出格式"),
        status: Optional[str] = Query(None, description="文章状态过滤"),
        limit: int = Query(1000, ge=1, le=10000, description="导出数量"),
        current_user: UserModel = Depends(admin_required_api)
):
    """
    导出文章列表为CSV或Excel文件
    
    Args:
        format: 导出格式(csv/excel)
        status: 文章状态过滤(published/draft/archived)
        limit: 导出数量(1-10000)
        
    Returns:
        文件下载
    """
    # For now, use sample data for demonstration
    articles = [
        {
            'id': i,
            'title': f'Article Title {i}',
            'author_id': 1,
            'category_id': 1,
            'status': 'published',
            'view_count': 100 * i,
            'like_count': 10 * i,
            'comment_count': 5 * i,
            'created_at': '2024-01-01T00:00:00',
            'updated_at': '2024-01-15T10:30:00',
            'published_at': '2024-01-02T00:00:00',
        }
        for i in range(1, min(limit + 1, 11))
    ]

    # 导出数据
    if format == 'excel':
        file_bytes = data_export_service.export_articles(articles, format='excel')
        media_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        filename = 'articles.xlsx'
    else:
        file_bytes = data_export_service.export_articles(articles, format='csv')
        media_type = 'text/csv'
        filename = 'articles.csv'

    return Response(
        content=file_bytes,
        media_type=media_type,
        headers={
            'Content-Disposition': f'attachment; filename="{filename}"',
        }
    )


@router.get("/comments", summary="导出评论列表")
@_catch
async def export_comments(
        format: str = Query('csv', enum=['csv', 'excel'], description="导出格式"),
        article_id: Optional[int] = Query(None, description="文章ID过滤"),
        limit: int = Query(1000, ge=1, le=10000, description="导出数量"),
        current_user: UserModel = Depends(admin_required_api)
):
    """
    导出评论列表为CSV或Excel文件
    
    Args:
        format: 导出格式(csv/excel)
        article_id: 文章ID过滤
        limit: 导出数量(1-10000)
        
    Returns:
        文件下载
    """
    # For now, use sample data for demonstration
    comments = [
        {
            'id': i,
            'article_id': 1,
            'user_id': i % 5 + 1,
            'content': f'This is comment {i}',
            'parent_id': None,
            'like_count': i % 10,
            'created_at': '2024-01-15T10:30:00',
            'is_approved': True,
        }
        for i in range(1, min(limit + 1, 11))
    ]

    # 导出数据
    if format == 'excel':
        file_bytes = data_export_service.export_comments(comments, format='excel')
        media_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        filename = 'comments.xlsx'
    else:
        file_bytes = data_export_service.export_comments(comments, format='csv')
        media_type = 'text/csv'
        filename = 'comments.csv'

    return Response(
        content=file_bytes,
        media_type=media_type,
        headers={
            'Content-Disposition': f'attachment; filename="{filename}"',
        }
    )


@router.get("/analytics", summary="导出分析数据")
@_catch
async def export_analytics(
        format: str = Query('csv', enum=['csv', 'excel'], description="导出格式"),
        report_type: str = Query('visits', enum=['visits', 'users', 'articles'], description="报表类型"),
        start_date: Optional[str] = Query(None, description="开始日期(YYYY-MM-DD)"),
        end_date: Optional[str] = Query(None, description="结束日期(YYYY-MM-DD)"),
        current_user: UserModel = Depends(admin_required_api)
):
    """
    导出分析数据报表
    
    Args:
        format: 导出格式(csv/excel)
        report_type: 报表类型(visits/users/articles)
        start_date: 开始日期
        end_date: 结束日期
        
    Returns:
        文件下载
    """
    # For now, use sample data for demonstration
    analytics_data = [
        {
            'date': f'2024-01-{i:02d}',
            'visits': 100 * i,
            'unique_visitors': 50 * i,
            'page_views': 200 * i,
            'bounce_rate': f'{10 + i}%',
            'avg_session_duration': f'{i * 60}s',
        }
        for i in range(1, 31)
    ]

    sheet_name = f'{report_type.title()} Report'

    # 导出数据
    if format == 'excel':
        file_bytes = data_export_service.export_analytics(
            analytics_data,
            format='excel',
            sheet_name=sheet_name
        )
        media_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        filename = f'{report_type}_report.xlsx'
    else:
        file_bytes = data_export_service.export_analytics(
            analytics_data,
            format='csv'
        )
        media_type = 'text/csv'
        filename = f'{report_type}_report.csv'

    return Response(
        content=file_bytes,
        media_type=media_type,
        headers={
            'Content-Disposition': f'attachment; filename="{filename}"',
        }
    )


@router.get("/templates", summary="获取导出模板")
@_catch
async def get_export_templates(
        current_user: UserModel = Depends(admin_required_api)
):
    """
    获取可用的导出模板和字段
    
    Returns:
        模板列表
    """
    templates = data_export_service.get_export_templates()

    return ok(data={
        'templates': templates,
    })


@router.post("/custom", summary="自定义数据导出")
@_catch
async def export_custom_data(
        data_type: str = Query(..., description="数据类型"),
        format: str = Query('csv', enum=['csv', 'excel'], description="导出格式"),
        fields: list = Query(..., description="导出字段列表"),
        filters: dict = None,
        current_user: UserModel = Depends(admin_required_api)
):
    """
    自定义数据导出
    
    Args:
        data_type: 数据类型(users/articles/comments/analytics)
        format: 导出格式(csv/excel)
        fields: 导出字段列表
        filters: 过滤条件
        
    Returns:
        文件下载
    """
    # For now, return sample data for demonstration
    sample_data = [
        {field: f'{field}_{i}' for field in fields}
        for i in range(1, 11)
    ]

    # 导出数据
    if format == 'excel':
        file_bytes = data_export_service.export_to_excel(
            sample_data,
            sheet_name=data_type.title()
        )
        media_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        filename = f'{data_type}_export.xlsx'
    else:
        file_bytes = data_export_service.export_to_csv(sample_data)
        media_type = 'text/csv'
        filename = f'{data_type}_export.csv'

    return Response(
        content=file_bytes,
        media_type=media_type,
        headers={
            'Content-Disposition': f'attachment; filename="{filename}"',
        }
    )
