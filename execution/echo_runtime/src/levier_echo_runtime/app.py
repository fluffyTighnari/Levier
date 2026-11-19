#!/usr/bin/env python3
"""Smallest possible Levier runtime.

This file intentionally imports only `levier_runtime`. It demonstrates the
surface a runtime author gets: configure a runtime name, write `run(ctx)`, and
call `Runtime.run()`.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from pathlib import Path

from levier_runtime import RunContext, __version__ as RUNTIME_VERSION
from levier_runtime.cli import run_cli
from levier_runtime.runtime import Runtime

DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "runtime.yaml"


async def run(ctx: RunContext) -> str:
    """Return the last input message as one assistant message."""
    content = "echo"
    if ctx.input:
        content = ctx.input[-1].content or content
    await ctx.reply(content)
    return "completed"


async def _serve(runtime: Runtime, _args) -> None:
    await runtime.serve(run)


def main(argv: list[str] | None = None) -> int:
    return run_cli(
        prog="levier-echo-runtime",
        version=RUNTIME_VERSION,
        default_config=DEFAULT_CONFIG,
        serve=_serve,
        argv=argv,
    )
