"""Middleware — harness command entry point.

P3-23 provides configuration, logging, health state, and a process
lifecycle skeleton. RuntimeChannel/ReAct start in later steps.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import signal
import sys
from typing import Sequence

from middleware_harness import __version__
from middleware_harness.config import ConfigError, load_settings
from middleware_harness.health import HealthState
from middleware_harness.logging import configure_logging, get_logger


def build_parser() -> argparse.ArgumentParser:
    """Build the command-line parser."""
    parser = argparse.ArgumentParser(prog="middleware-harness")
    parser.add_argument(
        "--config",
        default=os.environ.get("HARNESS_CONFIG", "harness/config.example.yaml"),
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
        asyncio.run(_wait_for_shutdown())
    except KeyboardInterrupt:
        pass
    logger.info("harness stopped")
    return 0


async def _wait_for_shutdown() -> None:
    """Wait until SIGINT/SIGTERM is received."""
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()

    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, stop.set)
        except NotImplementedError:
            pass

    await stop.wait()


if __name__ == "__main__":
    raise SystemExit(main())
