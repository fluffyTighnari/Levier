"""Shared RuntimeChannel host for independent runtime entrypoints."""

from __future__ import annotations

import asyncio
import logging
import signal
from typing import Callable

from levier_harness import __version__
from levier_harness.channel import ChannelClient, ChannelConfig
from levier_harness.config import Settings
from levier_harness.health import HealthState
from levier_harness.runtime import ManagerConfig, RunManager

RunnerFactory = Callable[[ChannelClient], tuple[object, object | None]]


async def run_runtime(
    settings: Settings,
    build_runner: RunnerFactory,
    *,
    health: HealthState | None = None,
    logger: logging.Logger | None = None,
) -> None:
    """Connect one runtime, serve TaskAssigns, and wait for shutdown."""
    client = ChannelClient(
        ChannelConfig(
            address=settings.middleware.address,
            daemon_id=settings.daemon.id,
            runtime_name=settings.daemon.runtime_name,
            auth_token=settings.daemon.auth_token.get_secret_value(),
            runtime_type=settings.daemon.runtime_type,
            max_concurrent_runs=settings.daemon.max_concurrent_runs,
            version=__version__,
            labels=dict(settings.daemon.labels),
            tls=settings.middleware.tls,
        ),
    )
    runner, close_extra = build_runner(client)
    manager = RunManager(
        sender=client,
        config=ManagerConfig(
            runtime_name=settings.daemon.runtime_name,
            max_concurrent_runs=settings.daemon.max_concurrent_runs,
            runtime_type=settings.daemon.runtime_type,
        ),
        runner=runner,  # type: ignore[arg-type]
        logger=logger,
    )
    client.on_message = manager.handle
    client.running_runs = manager.running_runs
    client.running_count = manager.running_count

    await client.start()
    try:
        await wait_for_shutdown()
    finally:
        if health is not None:
            health.set_check("middleware", client.connected)
            health.set_ready(False)
        await manager.close()
        if close_extra is not None:
            close = getattr(close_extra, "close", None)
            if callable(close):
                await close()
        await client.stop()


async def wait_for_shutdown() -> None:
    """Wait until SIGINT/SIGTERM is received."""
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()

    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, stop.set)
        except NotImplementedError:
            pass

    await stop.wait()
