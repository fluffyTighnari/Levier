"""One import for Levier Apps and Runtimes.

Apps::

    from levier import App, tool

    @tool
    def counter_add(delta: int) -> dict:
        return {"value": delta}

    app = App(tools=[counter_add], mcp=["github"])
    app.run()

Runtimes::

    from levier import Context, Runtime

    async def run(ctx: Context) -> str:
        tools = await ctx.toolbox()
        reply = await ctx.chat(ctx.input, tools=tools)
        await ctx.invoke_all(reply, toolbox=tools)
        return "completed"

    Runtime.start(run)

The heavy runtime SDK is imported lazily so an App-only installation stays
small and has no gRPC dependency.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from typing import Any

from levier_app import ToolExecutionError

from .app import App
from .tools import Tool, schema_from_function, tool

__version__ = "0.1.1"

__all__ = [
    "App",
    "Tool",
    "ToolExecutionError",
    "schema_from_function",
    "tool",
    "Context",
    "Runtime",
    "RunContext",
    "Toolbox",
    "__version__",
]


def __getattr__(name: str) -> Any:
    if name in {"Context", "RunContext", "Runtime", "Toolbox"}:
        from levier_runtime import Context, RunContext, Runtime
        from levier_runtime.toolbox import Toolbox

        exports = {
            "Context": Context,
            "RunContext": RunContext,
            "Runtime": Runtime,
            "Toolbox": Toolbox,
        }
        return exports[name]
    raise AttributeError(f"module 'levier' has no attribute {name!r}")
