"""
FastBlog MCP AI Agent API v2 — uses src.mcp.agent for clean direct execution.
Endpoints:
  POST /api/v2/mcp/chat        — Non-streaming chat
  POST /api/v2/mcp/chat/stream — SSE streaming chat
  GET  /api/v2/mcp/tools       — List MCP tools
  GET  /api/v2/mcp/info        — Server info
"""

import json
import logging
import uuid
from typing import Any, Dict, List, Optional

from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

from src.mcp.agent import LLMConfig, run_agent, stream_agent
from src.mcp.agent.format import format_tool_result
from src.mcp._context import set_user_ctx, UserCtx
from src.mcp.server import mcp_server
from src.auth.auth_deps import jwt_required_dependency as jwt_required
from src.api.v2._base import ApiResponse

logger = logging.getLogger("mcp_proxy")
router = APIRouter(prefix="/mcp", tags=["MCP AI Agent"])


# ─── Request/Response models ──────────────────────────────────────

class ChatMessage(BaseModel):
    role: str
    content: Optional[str] = None
    tool_calls: Optional[List[Dict[str, Any]]] = None
    tool_call_id: Optional[str] = None
    name: Optional[str] = None


class ChatRequest(BaseModel):
    endpoint: str
    api_key: str
    model: str
    messages: List[ChatMessage]
    conversation_id: Optional[str] = None
    max_tokens: int = 4096
    temperature: float = 0.7
    system_prompt: Optional[str] = None


# ApiResponse from src.api.v1.core.responses is used instead of ChatResponse


# ─── Helpers ──────────────────────────────────────────────────────

def _to_cfg(req: ChatRequest) -> LLMConfig:
    sp = req.system_prompt
    if not sp and req.messages and req.messages[0].role == "system":
        sp = req.messages[0].content or ""
    return LLMConfig(
        endpoint=req.endpoint,
        api_key=req.api_key,
        model=req.model,
        system_prompt=sp or "You are a helpful AI assistant.",
        max_tokens=req.max_tokens,
        temperature=req.temperature,
    )


def _to_dicts(messages: List[ChatMessage]) -> List[Dict]:
    """Convert pydantic ChatMessage list to plain dicts.
    
    Frontend-generated tool messages (from the client-side ReAct loop)
    may lack ``tool_call_id`` or may not follow an assistant ``tool_calls``
    message. Both cases are handled here to satisfy the OpenAI API contract.
    """
    result = []
    last_had_tool_calls = False

    for m in messages:
        # ── Tool message without preceding tool_calls → convert to assistant text ──
        if m.role == "tool" and not last_had_tool_calls:
            try:
                parsed = json.loads(m.content) if m.content else {}
                tool_name = m.name or parsed.get("name", "tool")
                raw_result = parsed.get("result", "")
                raw_text = json.dumps(raw_result, ensure_ascii=False) if raw_result else "{}"
                success = parsed.get("done", False)
                summary = format_tool_result(tool_name, raw_text, success=success)
            except (json.JSONDecodeError, TypeError, Exception):
                summary = m.content or "(空工具结果)"
            result.append({"role": "assistant", "content": summary})
            last_had_tool_calls = False
            continue

        d: Dict[str, Any] = {"role": m.role}
        if m.content is not None:
            d["content"] = m.content
        if m.tool_calls:
            d["tool_calls"] = m.tool_calls
            last_had_tool_calls = True
        elif m.role == "assistant":
            last_had_tool_calls = False
        if m.tool_call_id:
            d["tool_call_id"] = m.tool_call_id
        elif m.role == "tool":
            d["tool_call_id"] = f"frontend-{uuid.uuid4().hex[:8]}"
        if m.name:
            d["name"] = m.name
        result.append(d)
    return result


# ─── Non-streaming chat ──────────────────────────────────────────

@router.post("/chat")
async def chat_non_stream(req: ChatRequest, current_user=Depends(jwt_required)):
    """Non-streaming chat."""
    conv_id = req.conversation_id or uuid.uuid4().hex[:12]
    cfg = _to_cfg(req)
    messages = _to_dicts(req.messages)

    try:
        # 注入当前用户上下文以供工具处理器校验权限
        set_user_ctx(UserCtx(
            id=current_user.id,
            username=getattr(current_user, 'username', ''),
            is_superuser=getattr(current_user, 'is_superuser', False),
            role="superuser" if getattr(current_user, 'is_superuser', False) else "user",
        ))
        state = await run_agent(cfg, messages, conversation_id=conv_id)

        last = state.messages[-1] if state.messages else {}
        content = last.get("content", "") if last.get("role") == "assistant" else ""

        return ApiResponse(
            success=not state.errors,
            data={"conversation_id": conv_id, "content": content},
            error=state.errors[-1] if state.errors else None,
        )
    except Exception as e:
        logger.exception("Chat error")
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        set_user_ctx(None)


# ─── SSE streaming chat ──────────────────────────────────────────

@router.post("/chat/stream")
async def chat_stream(req: ChatRequest, current_user=Depends(jwt_required)):
    """SSE streaming chat."""
    conv_id = req.conversation_id or uuid.uuid4().hex[:12]
    cfg = _to_cfg(req)
    messages = _to_dicts(req.messages)

    async def event_generator():
        try:
            # 注入用户上下文（异步生成器内继承父级 contextvar）
            set_user_ctx(UserCtx(
                id=current_user.id,
                username=getattr(current_user, 'username', ''),
                is_superuser=getattr(current_user, 'is_superuser', False),
                role="superuser" if getattr(current_user, 'is_superuser', False) else "user",
            ))
            async for event in stream_agent(cfg, messages, conversation_id=conv_id):
                yield f"data: {json.dumps(event, ensure_ascii=False)}\n\n"
        except Exception as e:
            logger.exception("SSE error")
            yield f"data: {json.dumps({'type': 'error', 'message': str(e)}, ensure_ascii=False)}\n\n"
        finally:
            set_user_ctx(None)

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        },
    )


# ─── Info ─────────────────────────────────────────────────────────

@router.get("/tools")
async def list_mcp_tools(current_user=Depends(jwt_required)):
    return ApiResponse(success=True, data=mcp_server.get_openai_tools())


@router.get("/info")
async def mcp_proxy_info(current_user=Depends(jwt_required)):
    info = mcp_server.get_server_info()
    info["backend"] = "mcp.agent"
    return ApiResponse(success=True, data=info)
