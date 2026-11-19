"""Counter App built with the Levier Python adapter.

This file is intentionally small. The App owns its counter state and three
business handlers; credentials, HTTP, WebSocket, tool registration, reconnects,
and local_tool_call framing are handled by levier_app.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from typing import Any

from levier_app import LevierApp

from tools import Counter


def register_tools(app: LevierApp, counter: Counter) -> None:
    """Register the Counter business tools on one adapter instance."""

    @app.tool(
        "counter_add",
        description="Add an integer delta to the counter.",
        schema={
            "type": "object",
            "properties": {"delta": {"type": "integer"}},
            "required": ["delta"],
            "additionalProperties": False,
        },
    )
    def counter_add(arguments: dict[str, Any]) -> dict[str, int]:
        return {"value": counter.add(arguments["delta"])}

    @app.tool(
        "counter_get",
        description="Read the current counter value.",
        schema={"type": "object", "properties": {}, "additionalProperties": False},
    )
    def counter_get(_arguments: dict[str, Any]) -> dict[str, int]:
        return {"value": counter.get()}

    @app.tool(
        "counter_reset",
        description="Reset the counter to zero.",
        schema={"type": "object", "properties": {}, "additionalProperties": False},
    )
    def counter_reset(_arguments: dict[str, Any]) -> dict[str, int]:
        return {"value": counter.reset()}


def main() -> int:
    # The SDK owns credentials, argparse, logging, reconnect, and serving.
    # LEVIER_APP_MCP_REFS=github,e2e-echo is also read by the SDK.
    return LevierApp.run_cli(lambda app: register_tools(app, Counter()))


if __name__ == "__main__":
    raise SystemExit(main())
