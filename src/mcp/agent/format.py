"""
统一工具结果格式化 — 将 MCP 工具执行的原始 JSON 转为结构化 Markdown。

使用 markdownify（MarkItDown 的子依赖）处理 HTML 内容，
同时保留对 JSON 结构（dict/list/primitive）的智能格式化。

被以下模块共用：
  engine.py       → execute_mcp_tool()   — 后端 ReAct 循环内
  api/v2/mcp/     → orphan tool 转换    — API 入口层
"""

import json
import logging
from typing import Any, Dict, List, Optional, Union

logger = logging.getLogger("mcp.agent.format")


def _is_html(text: str) -> bool:
    """快速检测文本是否含 HTML 标签。"""
    return bool(text.strip().startswith(("<", "&")) or "<br" in text or "<p>" in text)


def format_tool_result(
    tool_name: str,
    raw_text: str,
    success: bool = True,
    error_msg: Optional[str] = None,
) -> str:
    """将 MCP 工具原始输出格式化为结构化 Markdown。

    Args:
        tool_name: 工具名（如 create_article）
        raw_text: MCP 工具返回的原始 JSON 文本
        success: 执行是否成功
        error_msg: 失败时的错误信息

    Returns:
        结构化 Markdown 字符串（blockquote 风格）
    """
    status_icon = "✅" if success else "❌"
    lines = [f"> {status_icon} **{tool_name}** {'执行成功' if success else '执行失败'}"]

    if error_msg:
        lines.append(f">   - **错误**: {error_msg}")
        return "\n".join(lines)

    if not raw_text or raw_text == "{}":
        lines.append(">   - （空结果）")
        return "\n".join(lines)

    # ── HTML 内容 → 用 markdownify 转换为 Markdown ──
    if _is_html(raw_text):
        try:
            from markdownify import markdownify
            md = markdownify(raw_text, heading_style="ATX", strip=["style", "script"])
            lines.append(f">\n{md}")
            return "\n".join(lines)
        except Exception as e:
            logger.warning(f"markdownify failed, falling back to plain text: {e}")
            lines.append(f"> {raw_text[:500]}")
            return "\n".join(lines)

    # ── JSON 内容 → 智能格式化 ──
    try:
        data = json.loads(raw_text)
    except json.JSONDecodeError:
        # 纯文本 → 直接引用
        lines.append(f"> {raw_text[:500]}")
        return "\n".join(lines)

    _format_json(data, lines, indent=0)
    return "\n".join(lines)


def _format_json(data: Any, lines: List[str], indent: int = 0) -> None:
    """递归格式化 JSON 数据为 Markdown 列表。"""
    prefix = "  " * indent
    prefix_item = f"{'>' if indent == 0 else ''}{prefix}"

    if isinstance(data, dict):
        # 忽略顶层 success 字段
        items = {k: v for k, v in data.items() if k not in ("success",)}
        if not items:
            return
        for k, v in items.items():
            if isinstance(v, dict):
                lines.append(f"{prefix_item}   - **{k}**:")
                _format_json(v, lines, indent + 2)
            elif isinstance(v, list):
                if len(v) == 0:
                    lines.append(f"{prefix_item}   - **{k}**: （空列表）")
                elif len(v) > 0 and isinstance(v[0], dict):
                    lines.append(f"{prefix_item}   - **{k}**: 共 **{len(v)}** 条记录")
                    for i, item in enumerate(v):
                        lines.append(f"{prefix_item}       - 条目 #{i + 1}:")
                        _format_json(item, lines, indent + 3)
                else:
                    items_str = ", ".join(str(x) for x in v[:10])
                    if len(v) > 10:
                        items_str += "…"
                    lines.append(f"{prefix_item}   - **{k}**: {items_str}")
            else:
                lines.append(f"{prefix_item}   - **{k}**: {v}")

        if "message" in data:
            lines.append(f"{prefix_item}\n{prefix_item} 📝 {data['message']}")

    elif isinstance(data, list):
        lines.append(f"{prefix_item}   - 共 **{len(data)}** 条记录")
        for i, item in enumerate(data):
            if isinstance(item, dict):
                lines.append(f"{prefix_item}       - 条目 #{i + 1}:")
                _format_json(item, lines, indent + 3)
            else:
                lines.append(f"{prefix_item}       - 条目 #{i + 1}: {item}")
    else:
        lines.append(f"{prefix_item}   - 结果: {data}")
