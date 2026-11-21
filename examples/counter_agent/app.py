"""Counter App built with the small Levier authoring surface.

This file is intentionally small. The App owns its counter state and three
business handlers; the schema, credentials, HTTP, WebSocket, tool registration,
reconnects, and local_tool_call framing are handled by the SDK.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from levier import tool
from levier_app import LevierApp

from tools import Counter


def register_tools(app: LevierApp, counter: Counter) -> None:
    """Register the Counter business tools on one adapter instance."""

    @tool
    def counter_add(delta: int) -> dict[str, int]:
        """Add an integer delta to the counter."""
        return {"value": counter.add(delta)}

    @tool
    def counter_get() -> dict[str, int]:
        """Read the current counter value."""
        return {"value": counter.get()}

    @tool
    def counter_reset() -> dict[str, int]:
        """Reset the counter to zero."""
        return {"value": counter.reset()}

    app.register(counter_add)
    app.register(counter_get)
    app.register(counter_reset)


def main() -> int:
    # The SDK owns credentials, argparse, logging, reconnect, and serving.
    return LevierApp.run_cli(lambda app: register_tools(app, Counter()))


if __name__ == "__main__":
    raise SystemExit(main())
