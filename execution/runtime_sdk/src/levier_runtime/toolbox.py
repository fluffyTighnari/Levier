"""One object that makes App, runtime, and MCP tools look the same.

Runtime authors do not need to know whether a capability is an App tool, a
Runtime-local tool, or an MCP tool. ``await ctx.toolbox()`` returns a sequence
of callable tools, model-facing specs, and one dispatch method.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import time
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable, Iterator, Mapping, Sequence

from .ids import new_ulid
from .mcp import McpRuntimeClient, McpRuntimeError
from .types import McpBinding, ToolResult, ToolSpec


_Caller = Callable[[str, str], Awaitable[ToolResult]]


@dataclass(frozen=True)
class Tool:
    """One tool with a uniform async call interface."""

    name: str
    description: str
    schema_json: str
    scope: str
    _caller: _Caller = field(repr=False, compare=False)

    async def call(
        self,
        arguments: Mapping[str, Any] | str | None = None,
        *,
        invocation_id: str = "",
    ) -> ToolResult:
        """Call this tool with a mapping or a JSON object string."""
        if isinstance(arguments, Mapping):
            payload = json.dumps(arguments, ensure_ascii=False, separators=(",", ":"))
        elif arguments is None:
            payload = "{}"
        elif isinstance(arguments, str):
            payload = arguments
        else:
            raise TypeError("tool arguments must be a mapping or JSON string")
        return await self._caller(payload, invocation_id)


class Toolbox(Sequence[Tool]):
    """All tools visible to one Run, with one dispatcher."""

    def __init__(
        self,
        tools: Sequence[Tool],
        *,
        mcp: McpRuntimeClient | None = None,
        sessions: Sequence[str] = (),
        logger: Any = None,
    ) -> None:
        self._tools = list(tools)
        self._by_name = {tool.name: tool for tool in self._tools}
        self._mcp = mcp
        self._sessions = list(sessions)
        self._logger = logger

    @classmethod
    async def open(cls, ctx: "Any") -> "Toolbox":
        """Build the toolbox for one ``RunContext``."""
        tools: list[Tool] = []
        seen: set[str] = set()
        for spec in ctx.tools:
            if spec.name in seen:
                continue
            seen.add(spec.name)
            tools.append(_local_tool(ctx, spec))

        mcp_client: McpRuntimeClient | None = None
        sessions: list[str] = []
        if ctx.mcp_bindings:
            socket_path = _mcp_socket()
            if not socket_path:
                ctx.logger.warning(
                    "MCP bindings were provided but no MCP runtime socket is configured"
                )
            else:
                mcp_client = McpRuntimeClient(socket_path)
                for binding in ctx.mcp_bindings:
                    session_id, mcp_tools = await _open_mcp(ctx, mcp_client, binding)
                    if session_id:
                        sessions.append(session_id)
                    for tool in mcp_tools:
                        if tool.name in seen:
                            ctx.logger.warning(
                                "MCP tool name %r conflicts with a visible tool; skipping",
                                tool.name,
                            )
                            continue
                        seen.add(tool.name)
                        tools.append(tool)
        return cls(tools, mcp=mcp_client, sessions=sessions, logger=ctx.logger)

    def __len__(self) -> int:
        return len(self._tools)

    def __iter__(self) -> Iterator[Tool]:
        return iter(self._tools)

    def __getitem__(self, index: int) -> Tool:
        return self._tools[index]

    def find(self, name: str) -> Tool | None:
        return self._by_name.get(name)

    def local(self) -> list[Tool]:
        """Return tools executed through the Levier tool bridge or runtime."""
        return [tool for tool in self._tools if tool.scope != "mcp"]

    def mcp(self) -> list[Tool]:
        """Return only tools backed by an MCP server."""
        return [tool for tool in self._tools if tool.scope == "mcp"]

    def specs(self) -> list[Any]:
        """Return model-facing tool specs for ``ctx.chat``."""
        from .llm.base import ToolSpec as LLMToolSpec

        return [
            LLMToolSpec(
                name=tool.name,
                description=tool.description,
                input_schema_json=tool.schema_json,
            )
            for tool in self._tools
        ]

    async def call(
        self,
        name: str,
        arguments: Mapping[str, Any] | str | None = None,
        *,
        invocation_id: str = "",
    ) -> ToolResult:
        """Dispatch one tool by its model-facing name."""
        value = self._by_name.get(name)
        if value is None:
            return ToolResult(
                ok=False,
                error_code="TOOL_NOT_FOUND",
                error_message=f"unknown tool {name!r}",
            )
        return await value.call(arguments, invocation_id=invocation_id)

    async def close(self) -> None:
        """Close every MCP session opened by this toolbox."""
        if self._mcp is None:
            return
        for session_id in self._sessions:
            try:
                await self._mcp.close_session(session_id)
            except Exception as exc:  # closing must not mask the run result
                if self._logger is not None:
                    self._logger.warning("close MCP session failed: %s", exc)
        self._sessions.clear()
        await self._mcp.close()
        self._mcp = None


def _local_tool(ctx: Any, spec: ToolSpec) -> Tool:
    async def call(payload: str, invocation_id: str) -> ToolResult:
        return await ctx.call_tool(
            spec.name,
            payload,
            invocation_id=invocation_id,
        )

    return Tool(
        name=spec.name,
        description=spec.description,
        schema_json=spec.input_schema_json or "{}",
        scope=spec.scope,
        _caller=call,
    )


async def _open_mcp(
    ctx: Any,
    client: McpRuntimeClient,
    binding: McpBinding,
) -> tuple[str, list[Tool]]:
    try:
        session = await client.open(binding)
    except McpRuntimeError as exc:
        await ctx.error(exc.code, str(exc), retryable=exc.retryable)
        return "", []
    except Exception as exc:  # runtime boundary
        await ctx.error("MCP_RUNTIME_ERROR", str(exc), retryable=True)
        return "", []
    if not session.session_id:
        await ctx.error(
            "MCP_RUNTIME_ERROR",
            f"MCP server {binding.server_name!r} returned no session",
            retryable=True,
        )
        return "", []

    tools: list[Tool] = []
    for spec in session.tools:

        async def call(
            payload: str,
            invocation_id: str,
            *,
            _name: str = spec.name,
            _session: str = session.session_id,
        ) -> ToolResult:
            invocation_id = invocation_id or new_ulid()
            started = time.monotonic()
            await ctx._emit_safe(
                "TOOL_CALL_BEGIN",
                {
                    "invocation_id": invocation_id,
                    "tool_name": _name,
                    "arguments_json": payload,
                },
            )
            try:
                result = await client.call(_session, _name, payload, _tool_timeout(ctx))
            except McpRuntimeError as exc:
                result = ToolResult(
                    ok=False,
                    error_code=exc.code,
                    error_message=str(exc),
                    retryable=exc.retryable,
                )
            except Exception as exc:  # runtime boundary
                result = ToolResult(
                    ok=False,
                    error_code="MCP_RUNTIME_ERROR",
                    error_message=str(exc),
                    retryable=True,
                )
            data: dict[str, Any] = {
                "invocation_id": invocation_id,
                "tool_name": _name,
                "ok": result.ok,
                "duration_ms": int((time.monotonic() - started) * 1000),
            }
            if result.ok:
                data["result_json"] = result.result_json
            else:
                data["error_code"] = result.error_code
                data["error_message"] = result.error_message
                data["retryable"] = result.retryable
            await ctx._emit_safe("TOOL_CALL_END", data)
            return result

        tools.append(
            Tool(
                name=spec.name,
                description=spec.description,
                schema_json=spec.input_schema_json or "{}",
                scope="mcp",
                _caller=call,
            )
        )
    return session.session_id, tools


def _mcp_socket() -> str:
    import os

    value = os.environ.get("LEVIER_MCP_RUNTIME_SOCKET", "").strip()
    if value:
        return value
    from .profile import default_mcp_socket

    return default_mcp_socket()


def _tool_timeout(ctx: Any) -> int:
    try:
        return int(ctx.timeouts.tool_ms or 60_000)
    except Exception:
        return 60_000


__all__ = ["Tool", "Toolbox"]
