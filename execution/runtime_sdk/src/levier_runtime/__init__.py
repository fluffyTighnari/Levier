"""Levier Runtime SDK.

A runtime author defines one `run(ctx)` function and gives it to `Runtime`:

    from levier_runtime import Runtime, RunContext

    async def run(ctx: RunContext) -> str:
        await ctx.reply("hello")
        return "completed"

    Runtime.from_config("runtime.yaml").run(run)

The SDK owns the RuntimeChannel and protocol. Model, tools, MCP, and checkpoint
are optional capabilities exposed through `RunContext`.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

__version__ = "0.1.0"

from .config import ConfigError, MiddlewareConfig, RuntimeConfig, load_runtime_config
from .context import RunContext, RuntimeUsageError
from .runtime import Runtime, RunFunction
from .types import (
    McpBinding,
    Message,
    ModelConfig,
    Timeouts,
    ToolResult,
    ToolSpec,
)

__all__ = [
    "ConfigError",
    "McpBinding",
    "Message",
    "MiddlewareConfig",
    "ModelConfig",
    "RunContext",
    "RunFunction",
    "Runtime",
    "RuntimeConfig",
    "RuntimeUsageError",
    "Timeouts",
    "ToolResult",
    "ToolSpec",
    "load_runtime_config",
]
