"""Small CLI shell shared by built-in Levier runtimes.

It handles configuration loading, validation, logging, and the common runtime
flags. Runtime-specific entry points only provide their async serve function
and any extra options.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import asyncio
import logging
import os
import sys
from pathlib import Path
from typing import Awaitable, Callable, Mapping, Sequence

from .runtime import Runtime

ServeFunction = Callable[[Runtime, argparse.Namespace], Awaitable[None]]
ExtraArguments = Callable[[argparse.ArgumentParser], None]
PrintConfigExtra = Callable[[Runtime, argparse.Namespace], Sequence[str]]


def run_cli(
    *,
    prog: str,
    version: str,
    default_config: str | Path,
    serve: ServeFunction,
    config_env: Sequence[str] = (),
    add_arguments: ExtraArguments | None = None,
    env_aliases: Mapping[str, str] | None = None,
    print_config_extra: PrintConfigExtra | None = None,
    argv: Sequence[str] | None = None,
) -> int:
    """Parse common runtime flags and serve until shutdown."""
    parser = argparse.ArgumentParser(prog=prog)
    default = _first_env(config_env) or os.environ.get("LEVIER_RUNTIME_CONFIG") or str(default_config)
    parser.add_argument("--config", default=default, help="path to runtime YAML configuration")
    parser.add_argument("--check", action="store_true", help="validate configuration")
    parser.add_argument("--print-config", action="store_true", help="print redacted configuration")
    parser.add_argument("--version", action="store_true", help="print version")
    parser.add_argument("--log-level", default="", help="override log level")
    if add_arguments is not None:
        add_arguments(parser)
    args = parser.parse_args(argv)

    if args.version:
        print(f"{prog} {version}")
        return 0

    _apply_env_aliases(env_aliases)
    if getattr(args, "log_level", ""):
        os.environ["LEVIER_RUNTIME_LOG_LEVEL"] = args.log_level
    try:
        runtime = Runtime.from_config(args.config)
    except Exception as exc:  # configuration must produce a clean CLI error
        print(f"{prog} config error: {exc}", file=sys.stderr)
        return 1

    level = getattr(logging, runtime.config.log_level.upper(), logging.INFO)
    logging.basicConfig(level=level)
    if args.check:
        print(f"runtime {runtime.config.runtime_name} / {runtime.config.instance_id}")
        return 0
    if args.print_config:
        _print_config(runtime, args, print_config_extra)
        return 0

    try:
        asyncio.run(serve(runtime, args))
    except KeyboardInterrupt:
        pass
    return 0


def _print_config(
    runtime: Runtime,
    args: argparse.Namespace,
    extra: PrintConfigExtra | None,
) -> None:
    config = runtime.config
    print(f"runtime: {config.runtime_name}")
    print(f"instance_id: {config.instance_id}")
    print(f"runtime_type: {config.runtime_type}")
    print(f"middleware_address: {config.middleware.address}")
    print(f"middleware_tls: {str(config.middleware.tls).lower()}")
    print(
        "middleware_auth_token: ***"
        if config.middleware.auth_token
        else "middleware_auth_token: (empty)"
    )
    print(f"max_concurrent_runs: {config.max_concurrent_runs}")
    print(f"log_level: {config.log_level}")
    if extra is not None:
        for line in extra(runtime, args):
            print(line)


def _apply_env_aliases(aliases: Mapping[str, str] | None) -> None:
    if not aliases:
        return
    for old, new in aliases.items():
        value = os.environ.get(old)
        if value is not None and new not in os.environ:
            os.environ[new] = value


def _first_env(names: Sequence[str]) -> str:
    for name in names:
        value = os.environ.get(name)
        if value:
            return value
    return ""
