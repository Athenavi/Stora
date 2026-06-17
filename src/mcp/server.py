"""
MCP (Model Context Protocol) Server 核心框架

提供资源/工具/提示词的注册、MCP JSON-RPC 请求路由和认证。
工具处理器已拆分到 tools/ 目录，工具定义在 tools/__init__.py。
"""
import json
import logging
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Any, List, Callable, Optional

from sqlalchemy import select

# [REMOVED] article model deleted in Stora refactor
# [REMOVED] category model deleted in Stora refactor
from shared.models.user import User
from shared.models.media import Media
from shared.models.system import SystemSettings
from src.mcp.tools import register_all as register_all_tools
from src.mcp._context import set_user_ctx, get_user_ctx, UserCtx
from src.utils.database.main import get_async_session_context

logger = logging.getLogger('mcp_server')




class MCPServer:
    """MCP Server 核心 — 注册、路由和生命周期管理"""

    def __init__(self, name: str = "fastblog-mcp", version: str = "1.0.0"):
        self.name = name
        self.version = version
        self.resources: Dict[str, Callable] = {}
        self.tools: Dict[str, Any] = {}  # {name: {description, parameters, handler_fn}}
        self.prompts: Dict[str, str] = {}
        self.is_running = False
        self.connected_clients: List[str] = []

        self._register_builtin_resources()
        register_all_tools(self)
        self._register_builtin_prompts()

    # ─── 注册 API ───

    def register_tool(self, name: str, description: str, parameters: Dict, handler: Callable):
        self.tools[name] = {"description": description, "parameters": parameters, "handler": handler}

    # ─── 资源处理器 ───

    def _register_builtin_resources(self):
        self.resources["blog://articles"] = self._get_articles_resource
        self.resources["blog://categories"] = self._get_categories_resource
        self.resources["blog://users"] = self._get_users_resource
        self.resources["blog://media"] = self._get_media_resource
        self.resources["blog://settings"] = self._get_settings_resource

    async def _get_articles_resource(self, params: Dict) -> List[Dict]:
        limit = params.get("limit", 20)
        async with get_async_session_context() as db:
            articles = (await db.execute(
                select(Article).where(Article.status == 1).order_by(Article.created_at.desc()).limit(limit)
            )).scalars().all()
            return [a.to_dict() for a in articles]

    async def _get_categories_resource(self, params: Dict) -> List[Dict]:
        async with get_async_session_context() as db:
            cats = (await db.execute(
                select(Category).order_by(Category.sort_order.asc(), Category.id.asc())
            )).scalars().all()
            return [{"id": c.id, "name": c.name, "slug": c.slug, "description": c.description} for c in cats]

    async def _get_users_resource(self, params: Dict) -> List[Dict]:
        async with get_async_session_context() as db:
            users = (await db.execute(select(User).limit(50))).scalars().all()
            return [{"id": u.id, "username": u.username, "email": u.email,
                     "role": "admin" if getattr(u, 'is_superuser', False) else "user",
                     "is_active": getattr(u, 'is_active', True)} for u in users]

    async def _get_media_resource(self, params: Dict) -> List[Dict]:
        async with get_async_session_context() as db:
            try:
                media_list = (await db.execute(
                    select(Media).order_by(Media.created_at.desc()).limit(50)
                )).scalars().all()
                return [{"id": m.id, "filename": m.filename or m.original_filename or "unknown",
                         "url": m.file_url or f"/media/{m.filename or ''}", "mime_type": getattr(m, 'mime_type', ''),
                         "size": getattr(m, 'file_size', 0), "alt_text": getattr(m, 'alt_text', '')}
                        for m in media_list]
            except Exception:
                pass
        media_dir = Path("static/uploads")
        if media_dir.exists():
            return [{"filename": f.name, "url": f"/static/uploads/{f.name}", "size": f.stat().st_size}
                    for f in list(media_dir.iterdir())[:50] if f.is_file()]
        return []

    async def _get_settings_resource(self, params: Dict) -> Dict:
        async with get_async_session_context() as db:
            try:
                settings = (await db.execute(select(SystemSettings).limit(50))).scalars().all()
                site = {s.setting_key: s.setting_value for s in settings if hasattr(s, 'setting_key')}
                return {"site_name": site.get("site_name", "FastBlog"),
                        "site_url": site.get("site_url", "http://localhost:9421"),
                        "language": site.get("default_language", "zh-CN"),
                        "description": site.get("site_description", "")}
            except Exception:
                pass
        return {"site_name": "FastBlog", "site_url": "http://localhost:9421", "language": "zh-CN"}

    # ─── 提示词模板 ───

    def _register_builtin_prompts(self):
        self.prompts["write_blog_post"] = """你是一个专业的博客作者。请根据以下主题撰写一篇博客文章：

主题: {topic}
目标读者: {audience}
文章长度: {length}
语气风格: {tone}

要求:
1. 包含引人入胜的开头
2. 使用清晰的小标题组织内容
3. 提供实用的建议和示例
4. 以有力的结论结尾
5. 字数控制在 {length} 字左右
"""
        self.prompts["seo_optimize"] = """请为以下文章优化 SEO：

标题: {title}
内容摘要: {excerpt}

请提供:
1. 优化的 Meta 描述（150-160字符）
2. 5-8个相关关键词
3. 建议的内部链接
4. 可读性改进建议
"""

    # ─── MCP 请求路由 ───

    async def handle_request(self, request: Dict[str, Any]) -> Dict[str, Any]:
        """处理 MCP JSON-RPC 请求"""
        method = request.get("method", "")
        params = request.get("params", {})
        req_id = request.get("id")

        if method == "resources/list":
            return await self._handle_resources_list(req_id)
        elif method == "resources/read":
            return await self._handle_resource_read(params, req_id)
        elif method == "tools/list":
            return await self._handle_tools_list(req_id)
        elif method == "tools/call":
            return await self._handle_tool_call(params, req_id)
        elif method == "prompts/list":
            return await self._handle_prompts_list(req_id)
        elif method == "prompts/get":
            return await self._handle_prompt_get(params, req_id)
        else:
            return self._error_response(req_id, f"Unknown method: {method}")

    async def _handle_resource_read(self, params: Dict, request_id: Any) -> Dict:
        uri = params.get("uri", "")
        handler = self.resources.get(uri)
        if not handler:
            return self._error_response(request_id, f"Resource not found: {uri}")
        try:
            result = await handler(params)
            return {"jsonrpc": "2.0", "id": request_id, "result": {"contents": [{"uri": uri, "text": json.dumps(result, ensure_ascii=False)}]}}
        except Exception as e:
            logger.exception(f"Resource read error: {uri}")
            return self._error_response(request_id, str(e))

    async def _handle_tool_call(self, params: Dict, request_id: Any) -> Dict:
        name = params.get("name", "")
        args = params.get("arguments", {})
        tool = self.tools.get(name)
        if not tool:
            return self._error_response(request_id, f"Tool not found: {name}")
        try:
            result = await tool["handler"](args)
            text = json.dumps(result if isinstance(result, dict) else {"data": result}, ensure_ascii=False)
            return {"jsonrpc": "2.0", "id": request_id, "result": {"content": [{"type": "text", "text": text}]}}
        except ValueError as e:
            return self._error_response(request_id, str(e))
        except Exception as e:
            logger.exception(f"Tool call error: {name}")
            return self._error_response(request_id, str(e))

    async def _handle_prompt_get(self, params: Dict, request_id: Any) -> Dict:
        name = params.get("name", "")
        template = self.prompts.get(name)
        if not template:
            return self._error_response(request_id, f"Prompt not found: {name}")
        try:
            args = params.get("arguments", {})
            text = template.format(**args)
            return {"jsonrpc": "2.0", "id": request_id, "result": {"description": name, "messages": [{"role": "user", "content": text}]}}
        except KeyError as e:
            return self._error_response(request_id, f"Missing prompt argument: {e}")

    async def _handle_resources_list(self, request_id: Any) -> Dict:
        resources = [{"uri": uri, "name": uri.split("://")[-1], "description": ""} for uri in self.resources]
        return {"jsonrpc": "2.0", "id": request_id, "result": {"resources": resources}}

    async def _handle_tools_list(self, request_id: Any) -> Dict:
        tools_list = [{"name": name, "description": info["description"],
                       "inputSchema": {"type": "object", "properties": self._to_json_schema(info["parameters"])}}
                      for name, info in self.tools.items()]
        return {"jsonrpc": "2.0", "id": request_id, "result": {"tools": tools_list}}

    async def _handle_prompts_list(self, request_id: Any) -> Dict:
        prompts_list = [{"name": name, "description": ""} for name in self.prompts]
        return {"jsonrpc": "2.0", "id": request_id, "result": {"prompts": prompts_list}}

    def _error_response(self, request_id: Any, message: str) -> Dict:
        return {"jsonrpc": "2.0", "id": request_id, "error": {"code": -32000, "message": message}}

    # ─── 工具格式转换 ───

    def get_server_info(self) -> Dict:
        return {"name": self.name, "version": self.version,
                "resources_count": len(self.resources), "tools_count": len(self.tools), "prompts_count": len(self.prompts)}

    def get_openai_tools(self) -> List[Dict]:
        """获取 OpenAI function-calling 格式的工具列表"""
        tools = []
        for name, info in self.tools.items():
            props = {}
            required = []
            for pname, pdef in info["parameters"].items():
                ptype = pdef.get("type", "string")
                props[pname] = {"type": ptype, "description": pdef.get("description", "")}
                if ptype == "array":
                    props[pname]["items"] = {"type": pdef.get("items_type", "string")}
                if pdef.get("enum"):
                    props[pname]["enum"] = pdef["enum"]
                if pdef.get("required", False):
                    required.append(pname)
            tools.append({"type": "function", "function": {"name": name, "description": info["description"],
                           "parameters": {"type": "object", "properties": props} | ({"required": required} if required else {})}})
        return tools

    def _to_json_schema(self, params: Dict) -> Dict:
        """将内部参数定义转为 JSON Schema"""
        props = {}
        required = []
        for pname, pdef in params.items():
            props[pname] = {"type": pdef.get("type", "string"), "description": pdef.get("description", "")}
            if pdef.get("enum"):
                props[pname]["enum"] = pdef["enum"]
            if pdef.get("required", False):
                required.append(pname)
        return {"type": "object", "properties": props, **({"required": required} if required else {})}


# 全局单例
mcp_server = MCPServer()
