"""Levier - React Runtime command entry point.

The React runtime uses the independent `levier_runtime` package for
RuntimeChannel, lifecycle, sequencing, and `RunContext`.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Sequence

from levier_runtime import MiddlewareConfig, Runtime, RuntimeConfig

from levier_react_runtime import __version__
from levier_react_runtime.config import ConfigError, Settings, load_settings
from levier_react_runtime.health import HealthState
from levier_react_runtime.llm import ProviderRouter
from levier_react_runtime.logging import configure_logging, get_logger
from levier_runtime.mcp import McpRuntimeClient
from levier_react_runtime.runtime.react import ReactRuntime
from levier_react_runtime.tools.router import ToolRouter

_DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "react.example.yaml"


def build_parser() -> argparse.ArgumentParser:
    """Build the command-line parser."""
    parser = argparse.ArgumentParser(prog="levier-react-runtime")
    parser.add_argument(
        "--config",
        default=os.environ.get("LEVIER_REACT_RUNTIME_CONFIG")
        or os.environ.get("LEVIER_HARNESS_CONFIG")
        or os.environ.get("HARNESS_CONFIG")
        or str(_DEFAULT_CONFIG),
        help="path to React Runtime YAML configuration",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="validate configuration and exit",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """Run the React runtime."""
    args = build_parser().parse_args(argv)

    try:
        settings = load_settings(args.config)
    except ConfigError as exc:
        print(f"react runtime config error: {exc}", file=sys.stderr)
        return 1

    configure_logging(settings.logging.level, settings.logging.format)
    logger = get_logger()
    health = HealthState(daemon_id=settings.daemon.id, version=__version__)
    health.set_ready(True)

    logger.info("react runtime starting", extra=settings.summary())
    if args.check:
        print(json.dumps(health.snapshot(), ensure_ascii=False))
        return 0

    try:
        asyncio.run(_run(settings, health))
    except KeyboardInterrupt:
        pass
    except ConfigError as exc:
        print(f"react runtime config error: {exc}", file=sys.stderr)
        return 1
    logger.info("react runtime stopped")
    return 0


async def _run(settings: Settings, health: HealthState) -> None:
    """Start the React runtime and wait for shutdown."""
    mcp_client = (
        McpRuntimeClient(settings.mcp_runtime.socket)
        if settings.mcp_runtime.socket
        else None
    )
    tools = ToolRouter(mcp=mcp_client)
    runtime = Runtime(
        RuntimeConfig(
            runtime_name=settings.daemon.runtime_name,
            instance_id=settings.daemon.id,
            middleware=MiddlewareConfig(
                address=settings.middleware.address,
                auth_token=settings.daemon.auth_token.get_secret_value(),
                auth_token_file=settings.daemon.auth_token_file,
                tls=settings.middleware.tls,
            ),
            runtime_type=settings.daemon.runtime_type,
            max_concurrent_runs=settings.daemon.max_concurrent_runs,
            labels=dict(settings.daemon.labels),
            log_level=settings.logging.level,
        ),
        logger=get_logger(),
    )
    try:
        await runtime.serve(ReactRuntime(ProviderRouter(), tools))
    finally:
        health.set_ready(False)
        if mcp_client is not None:
            await mcp_client.close()


if __name__ == "__main__":
    raise SystemExit(main())
