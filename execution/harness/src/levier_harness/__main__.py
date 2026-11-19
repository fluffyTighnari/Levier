"""Levier — harness command entry point.

P3-23 provides configuration, logging, health state, and a process
lifecycle skeleton. RuntimeChannel/ReAct start in later steps.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import signal
import sys
from pathlib import Path
from typing import Sequence

from levier_harness import __version__
from levier_harness.channel import ChannelClient
from levier_harness.config import ConfigError, Settings, load_settings
from levier_harness.health import HealthState
from levier_harness.llm import ProviderRouter
from levier_harness.logging import configure_logging, get_logger
from levier_harness.mcp import McpRuntimeClient
from levier_harness.runtime.host import run_runtime
from levier_harness.runtime.react import ReactRuntime
from levier_harness.tools.bridge import ToolBridge
from levier_harness.tools.router import ToolRouter

_DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "levier.example.yaml"


def build_parser() -> argparse.ArgumentParser:
    """Build the command-line parser."""
    parser = argparse.ArgumentParser(prog="levier-harness")
    parser.add_argument(
        "--config",
        default=os.environ.get("LEVIER_HARNESS_CONFIG")
        or os.environ.get("HARNESS_CONFIG")
        or str(_DEFAULT_CONFIG),
        help="path to harness YAML configuration",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="validate configuration and exit",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """Run the harness skeleton."""
    args = build_parser().parse_args(argv)

    try:
        settings = load_settings(args.config)
    except ConfigError as exc:
        print(f"harness config error: {exc}", file=sys.stderr)
        return 1

    configure_logging(settings.logging.level, settings.logging.format)
    logger = get_logger()
    health = HealthState(daemon_id=settings.daemon.id, version=__version__)
    health.set_ready(True)

    logger.info("harness starting", extra=settings.summary())
    if args.check:
        print(json.dumps(health.snapshot(), ensure_ascii=False))
        return 0

    try:
        asyncio.run(_run(settings, health))
    except KeyboardInterrupt:
        pass
    except ConfigError as exc:
        print(f"harness config error: {exc}", file=sys.stderr)
        return 1
    logger.info("harness stopped")
    return 0


async def _run(settings: Settings, health: HealthState) -> None:
    """Start the React runtime and wait for shutdown."""

    def build_runner(channel: ChannelClient) -> tuple[object, object | None]:
        """Build the React runner and optional MCP client."""
        mcp_client = (
            McpRuntimeClient(settings.mcp_runtime.socket)
            if settings.mcp_runtime.socket
            else None
        )
        tools = ToolRouter(bridge=ToolBridge(channel), mcp=mcp_client)
        return ReactRuntime(ProviderRouter(), tools), mcp_client

    await run_runtime(settings, build_runner, health=health, logger=get_logger())


if __name__ == "__main__":
    raise SystemExit(main())
