"""Tests for the uniform local/MCP toolbox."""

from __future__ import annotations

import asyncio
import unittest
from dataclasses import dataclass

from levier_runtime.toolbox import Toolbox
from levier_runtime.types import ToolResult, ToolSpec


@dataclass
class FakeContext:
    tools: list[ToolSpec]
    mcp_bindings: list = None
    logger: object = None

    def __post_init__(self) -> None:
        if self.mcp_bindings is None:
            self.mcp_bindings = []
        if self.logger is None:
            self.logger = _Logger()

    async def call_tool(self, name: str, arguments):
        return ToolResult(ok=True, result_json='{"name":"%s"}' % name)


class _Logger:
    def warning(self, *_args, **_kwargs) -> None:
        pass


class ToolboxTest(unittest.TestCase):
    def test_local_tools_are_callable_and_have_specs(self) -> None:
        ctx = FakeContext(
            tools=[
                ToolSpec(
                    name="counter_add",
                    description="Add delta.",
                    input_schema_json='{"type":"object"}',
                )
            ]
        )

        async def run():
            toolbox = await Toolbox.open(ctx)
            self.assertEqual(len(toolbox), 1)
            self.assertEqual(toolbox.specs()[0].name, "counter_add")
            result = await toolbox.call("counter_add", {"delta": 2})
            self.assertTrue(result.ok)
            self.assertIn("counter_add", result.result_json)
            await toolbox.close()

        asyncio.run(run())

    def test_unknown_tool_is_a_structured_result(self) -> None:
        async def run():
            toolbox = await Toolbox.open(FakeContext(tools=[]))
            result = await toolbox.call("missing", {})
            self.assertFalse(result.ok)
            self.assertEqual(result.error_code, "TOOL_NOT_FOUND")

        asyncio.run(run())


if __name__ == "__main__":
    unittest.main()
