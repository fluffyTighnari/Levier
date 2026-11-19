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

from levier_runtime import RunContext, Runtime, __version__ as RUNTIME_VERSION

DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "runtime.yaml"


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
    parser.add_argument("--print-config", action="store_true", help="print redacted configuration")
    parser.add_argument("--version", action="store_true", help="print version")
    parser.add_argument("--log-level", default="", help="override log level")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.version:
        print(f"levier-echo-runtime {RUNTIME_VERSION}")
        return 0
    if args.log_level:
        os.environ["LEVIER_RUNTIME_LOG_LEVEL"] = args.log_level
    try:
        runtime = Runtime.from_config(args.config)
    except Exception as exc:
        print(f"echo runtime config error: {exc}", file=sys.stderr)
        return 1
    if args.check:
        print(f"runtime {runtime.config.runtime_name} / {runtime.config.instance_id}")
        return 0
    if args.print_config:
        print_effective_config(runtime)
        return 0
    return runtime.run(run)

def print_effective_config(runtime: Runtime) -> None:
    config = runtime.config
    print(f"runtime: {config.runtime_name}")
    print(f"instance_id: {config.instance_id}")
    print(f"runtime_type: {config.runtime_type}")
    print(f"middleware_address: {config.middleware.address}")
    print(f"middleware_tls: {str(config.middleware.tls).lower()}")
    print("middleware_auth_token: ***" if config.middleware.auth_token else "middleware_auth_token: (empty)")
    print(f"max_concurrent_runs: {config.max_concurrent_runs}")
    print(f"log_level: {config.log_level}")


if __name__ == "__main__":
    raise SystemExit(main())
