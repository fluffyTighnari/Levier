#!/usr/bin/env python3
"""Smallest possible Levier runtime.

This file intentionally imports only `levier_runtime`. It demonstrates the
surface a runtime author gets: configure a runtime name, write `run(ctx)`, and
call `Runtime.run()`.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path
from typing import Sequence

from levier_runtime import RunContext, Runtime, load_runtime_config

DEFAULT_CONFIG = Path(__file__).with_name("runtime.yaml")


async def run(ctx: RunContext) -> str:
    """Return the last input message as one assistant message."""
    content = "echo"
    if ctx.input:
        content = ctx.input[-1].content or content
    await ctx.reply(content)
    return "completed"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="levier-echo-runtime")
    parser.add_argument(
        "--config",
        default=os.environ.get("LEVIER_RUNTIME_CONFIG") or str(DEFAULT_CONFIG),
        help="path to runtime YAML configuration",
    )
    parser.add_argument("--check", action="store_true", help="validate configuration")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        runtime = Runtime.from_config(args.config)
    except Exception as exc:
        print(f"echo runtime config error: {exc}", file=sys.stderr)
        return 1
    if args.check:
        print(f"runtime {runtime.config.runtime_name} / {runtime.config.instance_id}")
        return 0
    return runtime.run(run)


if __name__ == "__main__":
    raise SystemExit(main())
