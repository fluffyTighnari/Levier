"""Echo runtime entrypoint.

This is intentionally a separate runtime entrypoint. It uses the same
channel/runtime host, but its behavior is one small run function.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Sequence

from middleware.runtime.v1 import runtime_pb2

from levier_harness import __version__
from levier_harness.config import ConfigError, load_settings
from levier_harness.health import HealthState
from levier_harness.logging import configure_logging, get_logger
from levier_harness.runtime.host import run_runtime
from levier_harness.runtime.run import RunContext

_DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "echo.example.yaml"


class EchoRuntime:
    """Return the last input message as one assistant message."""

    async def __call__(self, ctx: RunContext) -> str:
        if ctx.cancelled:
            return "cancelled"
        content = "echo"
        if ctx.assign.input:
            content = ctx.assign.input[-1].content or content
        await ctx.emit(
            runtime_pb2.EventType.MESSAGE_COMPLETED,
            {
                "message_id": "echo",
                "role": "assistant",
                "content": content,
            },
        )
        return "completed"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="levier-echo-runtime")
    parser.add_argument(
        "--config",
        default=os.environ.get("LEVIER_ECHO_CONFIG")
        or os.environ.get("LEVIER_HARNESS_CONFIG")
        or str(_DEFAULT_CONFIG),
        help="path to echo runtime YAML configuration",
    )
    parser.add_argument("--check", action="store_true", help="validate configuration")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        settings = load_settings(args.config)
    except ConfigError as exc:
        print(f"echo runtime config error: {exc}", file=sys.stderr)
        return 1

    configure_logging(settings.logging.level, settings.logging.format)
    logger = get_logger()
    health = HealthState(daemon_id=settings.daemon.id, version=__version__)
    health.set_ready(True)
    logger.info("echo runtime starting", extra=settings.summary())
    if args.check:
        print(json.dumps(health.snapshot(), ensure_ascii=False))
        return 0

    async def run() -> None:
        await run_runtime(
            settings,
            lambda _client: (EchoRuntime(), None),
            health=health,
            logger=logger,
        )

    try:
        asyncio.run(run())
    except KeyboardInterrupt:
        pass
    logger.info("echo runtime stopped")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
