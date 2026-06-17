"""
MCP 工具定义注册表

所有工具的名称、描述、参数模式和处理器函数集中定义在此。
MCPServer 初始化时调用 register_all() 一次性注册全部工具。
"""
from src.mcp.tools import article, content, analytics, media
from src.mcp.tools import system, users_tools, security_tools
from src.mcp.tools import workflow, notifications_tools, cache_tools


# 工具定义列表：(name, description, parameters_dict, handler_func)
# parameters_dict: {param_name: {type, description, required, enum?}, ...}
_TOOL_DEFS = [
    # ─── 文章管理 (4) ───
    ("create_article", "创建新文章", {
        "title": {"type": "string", "description": "文章标题", "required": True},
        "content": {"type": "string", "description": "文章内容 (Markdown)", "required": True},
        "category_id": {"type": "integer", "description": "分类ID"},
        "tags": {"type": "string", "description": "标签列表（逗号分隔）"},
        "status": {"type": "string", "description": "状态 (draft/published)", "enum": ["draft", "published"]},
    }, article.create_article),
    ("update_article", "更新现有文章", {
        "article_id": {"type": "integer", "description": "文章ID", "required": True},
        "title": {"type": "string", "description": "新标题"},
        "content": {"type": "string", "description": "新内容 (Markdown)"},
        "status": {"type": "string", "description": "新状态 (draft/published)"},
    }, article.update_article),
    ("delete_article", "删除文章（软删除）", {
        "article_id": {"type": "integer", "description": "文章ID", "required": True},
    }, article.delete_article),
    ("search_articles", "搜索文章（关键词搜索，支持 MeiliSearch）", {
        "query": {"type": "string", "description": "搜索关键词", "required": True},
        "limit": {"type": "integer", "description": "返回数量（默认 10，最多 50）"},
    }, article.search_articles),

    # ─── 分类管理 (4) ───
    ("list_categories", "获取所有分类列表", {}, content.list_categories),
    ("create_category", "创建新分类", {
        "name": {"type": "string", "description": "分类名称", "required": True},
        "slug": {"type": "string", "description": "分类别名"},
        "description": {"type": "string", "description": "分类描述"},
    }, content.create_category),
    ("update_category", "更新分类信息", {
        "category_id": {"type": "integer", "description": "分类ID", "required": True},
        "name": {"type": "string", "description": "新分类名称"},
        "description": {"type": "string", "description": "新分类描述"},
    }, content.update_category),
    ("delete_category", "删除分类", {
        "category_id": {"type": "integer", "description": "分类ID", "required": True},
    }, content.delete_category),

    # ─── 标签 (1) ───
    ("list_tags", "获取所有标签列表（从文章中聚合）", {}, content.list_tags),

    # ─── 评论管理 (4) ───
    ("list_comments", "获取评论列表，支持按状态筛选", {
        "status": {"type": "string", "description": "筛选状态: pending/approved/rejected",
                    "enum": ["pending", "approved", "rejected"]},
        "limit": {"type": "integer", "description": "返回数量（默认 20，最多 50）"},
    }, content.list_comments),
    ("approve_comment", "审核通过评论", {
        "comment_id": {"type": "integer", "description": "评论ID", "required": True},
    }, content.approve_comment),
    ("reject_comment", "拒绝评论", {
        "comment_id": {"type": "integer", "description": "评论ID", "required": True},
    }, content.reject_comment),
    ("delete_comment", "删除评论（管理员）", {
        "comment_id": {"type": "integer", "description": "评论ID", "required": True},
    }, content.delete_comment),

    # ─── 分析工具 (3) ───
    ("get_analytics", "获取博客分析概况（文章数/用户数/评论数/浏览量等）", {}, analytics.get_analytics),
    ("get_trending_articles", "获取热门文章排行", {
        "limit": {"type": "integer", "description": "返回数量（默认 10，最多 30）"},
        "days": {"type": "integer", "description": "统计天数（默认 7）"},
    }, analytics.get_trending_articles),
    ("get_system_stats", "获取系统统计信息", {}, analytics.get_system_stats),

    # ─── 媒体管理 (2) ───
    ("list_media", "获取媒体文件列表", {
        "limit": {"type": "integer", "description": "返回数量（默认 20，最多 50）"},
        "media_type": {"type": "string", "description": "筛选类型: image/video/audio/document"},
    }, media.list_media),
    ("delete_media", "删除媒体文件", {
        "media_id": {"type": "integer", "description": "媒体ID", "required": True},
    }, media.delete_media),

    # ─── SEO (1) ───
    ("generate_seo_description", "为文章生成 SEO 描述（从摘要截取前 160 字符）", {
        "article_id": {"type": "integer", "description": "文章ID", "required": True},
    }, analytics.generate_seo_description),

    # ========== 以下为 V3 新增工具 ==========

    # ─── 系统管理 (9) [仅 superuser] ───
    ("get_settings", "获取系统设置（管理员）", {
        "key": {"type": "string", "description": "设置键名（可选，留空返回全部）"},
    }, system.get_settings),
    ("update_settings", "更新系统设置（管理员）", {
        "settings": {"type": "object", "description": "设置键值对，如 {\"site_name\": \"My Blog\"}", "required": True},
    }, system.update_settings),
    ("list_backups", "列出可用数据库备份（管理员）", {}, system.list_backups),
    ("create_backup", "创建数据库备份（管理员）", {
        "name": {"type": "string", "description": "备份文件名（可选）"},
    }, system.create_backup),
    ("get_system_info", "获取系统信息（版本/Python/平台/数据库）（管理员）", {}, system.get_system_info),
    ("list_webhooks", "列出所有 Webhook 配置（管理员）", {}, system.list_webhooks),
    ("run_migration", "执行数据库迁移（管理员）", {}, system.run_migration),
    ("get_maintenance_mode", "查看维护模式状态（管理员）", {}, system.get_maintenance_mode),
    ("set_maintenance_mode", "设置维护模式开/关（管理员）", {
        "enabled": {"type": "boolean", "description": "是否开启维护模式", "required": True},
    }, system.set_maintenance_mode),

    # ─── 用户管理 (6) ───
    ("list_users", "获取用户列表（管理员）", {
        "page": {"type": "integer", "description": "页码"},
        "limit": {"type": "integer", "description": "每页数量"},
    }, users_tools.list_users),
    ("create_user", "创建新用户（管理员）", {
        "username": {"type": "string", "description": "用户名", "required": True},
        "email": {"type": "string", "description": "邮箱"},
        "password": {"type": "string", "description": "密码", "required": True},
        "is_superuser": {"type": "boolean", "description": "是否超级管理员"},
    }, users_tools.create_user),
    ("update_user_role", "更新用户角色（管理员）", {
        "user_id": {"type": "integer", "description": "用户ID", "required": True},
        "role": {"type": "string", "description": "角色: user/admin/superuser"},
    }, users_tools.update_user_role),
    ("ban_user", "封禁/解封用户（管理员）", {
        "user_id": {"type": "integer", "description": "用户ID", "required": True},
        "ban": {"type": "boolean", "description": "True=封禁 False=解封"},
        "reason": {"type": "string", "description": "封禁原因"},
    }, users_tools.ban_user),
    ("get_user_stats", "获取用户统计数据", {
        "user_id": {"type": "integer", "description": "用户ID", "required": True},
    }, users_tools.get_user_stats),
    ("list_user_activity", "获取用户最近活动", {
        "user_id": {"type": "integer", "description": "用户ID", "required": True},
        "limit": {"type": "integer", "description": "返回数量"},
    }, users_tools.list_user_activity),

    # ─── 安全管理 (6) [管理员] ───
    ("query_audit_log", "查询审计日志（管理员）", {
        "page": {"type": "integer", "description": "页码"},
        "limit": {"type": "integer", "description": "每页数量"},
        "user_id": {"type": "integer", "description": "按用户筛选"},
        "action": {"type": "string", "description": "按操作筛选"},
        "level": {"type": "string", "description": "按级别筛选: INFO/WARNING/ERROR/CRITICAL"},
    }, security_tools.query_audit_log),
    ("export_audit_log", "导出审计日志为 CSV（管理员）", {
        "days": {"type": "integer", "description": "最近N天"},
    }, security_tools.export_audit_log),
    ("scan_sensitive_words", "扫描文本中的敏感词（管理员）", {
        "text": {"type": "string", "description": "要扫描的文本", "required": True},
    }, security_tools.scan_sensitive_words),
    ("manage_sensitive_word", "添加/删除敏感词（管理员）", {
        "action": {"type": "string", "description": "add 或 remove", "required": True},
        "word": {"type": "string", "description": "敏感词", "required": True},
        "level": {"type": "string", "description": "级别: low/medium/high"},
    }, security_tools.manage_sensitive_word),
    ("list_rate_limits", "查看速率限制状态（管理员）", {}, security_tools.list_rate_limits),
    ("get_security_report", "生成安全报告摘要（管理员）", {}, security_tools.get_security_report),

    # ─── 工作流/协作 (6) ───
    ("list_pending_reviews", "列出待审批内容", {
        "page": {"type": "integer", "description": "页码"},
        "limit": {"type": "integer", "description": "每页数量"},
    }, workflow.list_pending_reviews),
    ("approve_content", "审批通过内容", {
        "review_id": {"type": "integer", "description": "审批记录ID", "required": True},
        "comment": {"type": "string", "description": "审批意见"},
    }, workflow.approve_content),
    ("reject_content", "驳回内容", {
        "review_id": {"type": "integer", "description": "审批记录ID", "required": True},
        "reason": {"type": "string", "description": "驳回原因", "required": True},
    }, workflow.reject_content),
    ("get_workflow_stats", "获取工作流统计", {}, workflow.get_workflow_stats),
    ("list_collaborators", "列出协作成员（管理员）", {
        "workspace_id": {"type": "integer", "description": "工作区ID"},
    }, workflow.list_collaborators),
    ("batch_publish_articles", "批量发布文章（管理员）", {
        "article_ids": {"type": "array", "description": "文章ID列表", "items_type": "integer", "required": True},
        "schedule_time": {"type": "string", "description": "定时发布时间（可选）"},
    }, workflow.batch_publish_articles),

    # ─── 通知管理 (4) ───
    ("list_notifications", "获取当前用户的通知列表", {
        "user_id": {"type": "integer", "description": "用户ID", "required": True},
        "unread_only": {"type": "boolean", "description": "仅未读"},
        "limit": {"type": "integer", "description": "返回数量"},
    }, notifications_tools.list_notifications),
    ("mark_notification_read", "标记通知为已读", {
        "notification_id": {"type": "integer", "description": "通知ID", "required": True},
    }, notifications_tools.mark_notification_read),
    ("send_test_email", "发送测试邮件（管理员）", {
        "to_email": {"type": "string", "description": "收件人邮箱", "required": True},
    }, notifications_tools.send_test_email),
    ("send_bulk_notification", "向所有用户发送批量通知（管理员）", {
        "title": {"type": "string", "description": "通知标题", "required": True},
        "message": {"type": "string", "description": "通知内容", "required": True},
    }, notifications_tools.send_bulk_notification),

    # ─── 缓存/性能 (3) [管理员] ───
    ("clear_cache", "清理缓存（管理员）", {
        "cache_type": {"type": "string", "description": "缓存类型: all/redis/page/object"},
    }, cache_tools.clear_cache),
    ("get_cache_status", "查看缓存状态（管理员）", {}, cache_tools.get_cache_status),
    ("get_performance_metrics", "获取性能指标概览（管理员）", {}, cache_tools.get_performance_metrics),
]


def register_all(mcp_server: "MCPServer") -> None:
    """向 MCPServer 实例注册所有工具"""
    for name, desc, params, handler in _TOOL_DEFS:
        mcp_server.register_tool(name=name, description=desc, parameters=params, handler=handler)
