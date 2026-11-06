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
from middleware_harness.channel import ChannelClient, ChannelConfig
from middleware_harness.config import ConfigError, Settings, load_settings
from middleware_harness.health import HealthState
from middleware_harness.llm import AnthropicProvider
from middleware_harness.logging import configure_logging, get_logger
from middleware_harness.runtime import ManagerConfig, RunManager
from middleware_harness.strategies import EchoStrategy, ReactStrategy, StrategyRouter


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
        asyncio.run(_run(settings, health))
    except KeyboardInterrupt:
        pass
    except ConfigError as exc:
        print(f"harness config error: {exc}", file=sys.stderr)
        return 1
    logger.info("harness stopped")
    return 0


async def _run(settings: Settings, health: HealthState) -> None:
    """Start the RuntimeChannel client and wait for shutdown."""
    client = ChannelClient(
        ChannelConfig(
            address=settings.middleware.address,
            daemon_id=settings.daemon.id,
            auth_token=settings.daemon.auth_token.get_secret_value(),
            runtime_type=settings.daemon.runtime_type,
            max_concurrent_runs=settings.daemon.max_concurrent_runs,
            strategies=list(settings.daemon.strategies),
            version=__version__,
            labels=dict(settings.daemon.labels),
            tls=settings.middleware.tls,
        ),
    )
    runner, supported = _build_runner(settings)
    manager = RunManager(
        sender=client,
        config=ManagerConfig(
            max_concurrent_runs=settings.daemon.max_concurrent_runs,
            supported_strategies=supported,
        ),
        runner=runner,
    )
    client.on_message = manager.handle
    client.running_runs = manager.running_runs
    client.running_count = manager.running_count

    await client.start()
    try:
        await _wait_for_shutdown()
    finally:
        health.set_check("middleware", client.connected)
        await manager.close()
        await client.stop()
        health.set_ready(False)


def _build_runner(settings: Settings) -> tuple[StrategyRouter, tuple[str, ...]]:
    """Build the strategy registry from daemon configuration."""
    available = {
        "echo": EchoStrategy(),
        "react": ReactStrategy(AnthropicProvider()),
    }
    unknown = [name for name in settings.daemon.strategies if name not in available]
    if unknown:
        raise ConfigError(f"unsupported strategies: {', '.join(unknown)}")
    names = tuple(dict.fromkeys(settings.daemon.strategies))
    if not names:
        raise ConfigError("daemon.strategies must not be empty")
    return StrategyRouter({name: available[name] for name in names}), names


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
