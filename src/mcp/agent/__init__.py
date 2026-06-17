"""
MCP Agent — clean, direct async AI agent with streaming support.

No langchain/langgraph dependencies. Pure asyncio + httpx.
"""
from src.mcp.agent.engine import run_agent, stream_agent, LLMConfig

__all__ = ["run_agent", "stream_agent", "LLMConfig"]
