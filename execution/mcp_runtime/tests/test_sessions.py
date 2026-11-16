"""Middleware — MCP session manager tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import unittest
from contextlib import AsyncExitStack
from dataclasses import dataclass, field
from typing import Any

from levier_mcp_runtime.errors import McpRuntimeError
from levier_mcp_runtime.secrets import FileSecretResolver
from levier_mcp_runtime.sessions import SessionManager


@dataclass
class FakeTool:
    name: str
    description: str = ""
    input_schema: dict[str, Any] = field(default_factory=lambda: {"type": "object"})


@dataclass
class FakeListResult:
    tools: list[FakeTool]


@dataclass
class FakeText:
    text: str
    type: str = "text"

    def model_dump(self, **kwargs: Any) -> dict[str, Any]:
        return {"type": self.type, "text": self.text}


@dataclass
class FakeCallResult:
    content: list[FakeText] = field(default_factory=list)
    structured_content: Any = None
    is_error: bool = False


class FakeSession:
    def __init__(self, tools: list[FakeTool], *, delay: float = 0.0) -> None:
        self.tools = tools
        self.delay = delay
        self.calls: list[tuple[str, dict[str, Any]]] = []
        self.closed = False

    async def list_tools(self) -> FakeListResult:
        return FakeListResult(self.tools)

    async def call_tool(self, name: str, arguments: dict[str, Any], **kwargs: Any) -> FakeCallResult:
        if self.delay:
            await asyncio.sleep(self.delay)
        self.calls.append((name, arguments))
        return FakeCallResult(content=[FakeText(f"{arguments.get('a', 0) + arguments.get('b', 0)}")])


class FakeConnector:
    def __init__(self, session: FakeSession) -> None:
        self.session = session
        self.bindings: list[dict[str, Any]] = []
        self.stacks: list[AsyncExitStack] = []

    async def connect(self, binding: dict[str, Any]):
        self.bindings.append(binding)
        stack = AsyncExitStack()
        self.stacks.append(stack)
        return self.session, stack


class SessionManagerTest(unittest.IsolatedAsyncioTestCase):
    async def asyncTearDown(self) -> None:
        if hasattr(self, "manager"):
            await self.manager.close_all()

    async def test_open_filters_allowed_tools_and_namespaces(self) -> None:
        session = FakeSession([
            FakeTool("search"),
            FakeTool("delete"),
        ])
        self.manager = SessionManager(FakeConnector(session))

        opened = await self.manager.open({
            "server_name": "github",
            "transport": "stdio",
            "command": "fake",
            "allowed_tools": ["search"],
        }, "owner-1")

        self.assertEqual(
            [tool["name"] for tool in opened["tools"]],
            ["mcp__github__search"],
        )
        self.assertEqual(
            opened["tools"][0]["input_schema_json"],
            '{"type":"object"}',
        )

    async def test_open_empty_allowlist_includes_all_tools(self) -> None:
        session = FakeSession([FakeTool("a"), FakeTool("b")])
        self.manager = SessionManager(FakeConnector(session))

        opened = await self.manager.open({
            "server_name": "srv",
            "transport": "stdio",
            "command": "fake",
            "namespace": "custom",
        }, "owner-1")

        self.assertEqual(
            [tool["name"] for tool in opened["tools"]],
            ["custom__a", "custom__b"],
        )

    async def test_open_resolves_secret_placeholders(self) -> None:
        session = FakeSession([FakeTool("add")])
        connector = FakeConnector(session)
        self.manager = SessionManager(
            connector,
            secrets=FileSecretResolver({"token": "s3"}),
        )

        await self.manager.open({
            "server_name": "srv",
            "transport": "stdio",
            "command": "cmd-${secret:token}",
            "env": {"TOKEN": "Bearer ${secret:token}"},
        }, "owner-1")

        self.assertEqual(connector.bindings[0]["command"], "cmd-s3")
        self.assertEqual(connector.bindings[0]["env"]["TOKEN"], "Bearer s3")

    async def test_open_missing_secret_fails_before_connect(self) -> None:
        connector = FakeConnector(FakeSession([]))
        self.manager = SessionManager(connector, secrets=FileSecretResolver({}))

        with self.assertRaises(McpRuntimeError) as caught:
            await self.manager.open({
                "server_name": "srv",
                "transport": "stdio",
                "command": "cmd",
                "env": {"TOKEN": "${secret:missing}"},
            }, "owner-1")

        self.assertEqual(caught.exception.code, "SECRET_MISSING")
        self.assertEqual(connector.bindings, [])

    async def test_call_maps_namespaced_tool_and_returns_text(self) -> None:
        session = FakeSession([FakeTool("add")])
        self.manager = SessionManager(FakeConnector(session))
        opened = await self.manager.open({
            "server_name": "math",
            "transport": "stdio",
            "command": "fake",
        }, "owner-1")

        result = await self.manager.call(
            opened["session_id"],
            "mcp__math__add",
            '{"a":1,"b":2}',
            1000,
        )
        self.assertTrue(result["ok"])
        self.assertEqual(result["result_json"], "3")
        self.assertEqual(session.calls, [("add", {"a": 1, "b": 2})])

    async def test_call_unknown_tool_is_denied(self) -> None:
        session = FakeSession([FakeTool("add")])
        self.manager = SessionManager(FakeConnector(session))
        opened = await self.manager.open({
            "server_name": "math",
            "transport": "stdio",
            "command": "fake",
        }, "owner-1")

        with self.assertRaises(McpRuntimeError) as caught:
            await self.manager.call(
                opened["session_id"],
                "mcp__math__delete",
                "{}",
                1000,
            )
        self.assertEqual(caught.exception.code, "TOOL_DENIED")

    async def test_call_timeout(self) -> None:
        session = FakeSession([FakeTool("slow")], delay=1.0)
        self.manager = SessionManager(FakeConnector(session))
        opened = await self.manager.open({
            "server_name": "slow",
            "transport": "stdio",
            "command": "fake",
        }, "owner-1")

        with self.assertRaises(McpRuntimeError) as caught:
            await self.manager.call(
                opened["session_id"],
                "mcp__slow__slow",
                "{}",
                10,
            )
        self.assertEqual(caught.exception.code, "TOOL_TIMEOUT")
        self.assertTrue(caught.exception.retryable)

    async def test_close_and_close_owner(self) -> None:
        session = FakeSession([FakeTool("a")])
        connector = FakeConnector(session)
        self.manager = SessionManager(connector)
        first = await self.manager.open({
            "server_name": "one",
            "transport": "stdio",
            "command": "fake",
        }, "owner-1")
        second = await self.manager.open({
            "server_name": "two",
            "transport": "stdio",
            "command": "fake",
        }, "owner-2")

        await self.manager.close(first["session_id"])
        with self.assertRaises(McpRuntimeError) as caught:
            await self.manager.call(first["session_id"], "mcp__one__a", "{}", 1000)
        self.assertEqual(caught.exception.code, "SESSION_NOT_FOUND")

        await self.manager.close_owner("owner-2")
        with self.assertRaises(McpRuntimeError) as caught:
            await self.manager.call(second["session_id"], "mcp__two__a", "{}", 1000)
        self.assertEqual(caught.exception.code, "SESSION_NOT_FOUND")

    async def test_invalid_binding_is_rejected(self) -> None:
        self.manager = SessionManager(FakeConnector(FakeSession([])))
        with self.assertRaises(McpRuntimeError) as caught:
            await self.manager.open({
                "server_name": "bad",
                "transport": "websocket",
            }, "owner-1")
        self.assertEqual(caught.exception.code, "BAD_BINDING")

    async def test_call_result_error_is_structured(self) -> None:
        class ErrorSession(FakeSession):
            async def call_tool(self, name, arguments, **kwargs):
                return FakeCallResult(content=[FakeText("boom")], is_error=True)

        session = ErrorSession([FakeTool("fail")])
        self.manager = SessionManager(FakeConnector(session))
        opened = await self.manager.open({
            "server_name": "err",
            "transport": "stdio",
            "command": "fake",
        }, "owner-1")

        result = await self.manager.call(
            opened["session_id"],
            "mcp__err__fail",
            "{}",
            1000,
        )
        self.assertFalse(result["ok"])
        self.assertEqual(result["error"]["code"], "TOOL_EXEC_ERROR")
        self.assertEqual(result["error"]["message"], "boom")


if __name__ == "__main__":
    unittest.main()
