"""Runtime process lifecycle.

The public entry point is `Runtime.from_config(...).run(your_run_function)`.
The SDK owns channel setup, reconnects, heartbeats, acknowledgements, event
sequencing, cancellation, and graceful shutdown.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import inspect
import logging
import signal
from pathlib import Path
from typing import Any, Awaitable, Callable

from . import __version__
from .channel.client import ChannelClient, ChannelConfig
from .config import RuntimeConfig, load_runtime_config
from .context import RunContext
from .driver import RuntimeDriver, driver_capabilities, driver_run_function
from .manager import ManagerConfig, RunManager

RunFunction = Callable[[RunContext], Awaitable[str]]


class Runtime:
    """One runtime process serving one `runtime_name`."""

    def __init__(self, config: RuntimeConfig | None = None, *, logger: logging.Logger | None = None) -> None:
        self.config = config or load_runtime_config()
        self.logger = logger or logging.getLogger("levier_runtime")
        self._stop_event: asyncio.Event | None = None

    @classmethod
    def from_config(
        cls,
        path: str | Path | None = None,
        *,
        environ: dict[str, str] | None = None,
        logger: logging.Logger | None = None,
    ) -> "Runtime":
        """Build a runtime from `runtime.yaml`, the user profile, or defaults."""
        return cls(load_runtime_config(path, environ=environ), logger=logger)

    @classmethod
    def start(
        cls,
        run: RunFunction,
        *,
        config: RuntimeConfig | str | Path | None = None,
        environ: dict[str, str] | None = None,
        handle_signals: bool = True,
    ) -> int:
        """Load defaults (no YAML required) and serve one `run(ctx)`."""
        if isinstance(config, RuntimeConfig):
            runtime = cls(config)
        else:
            runtime = cls.from_config(config, environ=environ)
        return runtime.run(run, handle_signals=handle_signals)

    @classmethod
    def start_driver(
        cls,
        driver: RuntimeDriver | RunFunction,
        *,
        config: RuntimeConfig | str | Path | None = None,
        environ: dict[str, str] | None = None,
        handle_signals: bool = True,
    ) -> int:
        """Load defaults and serve a driver or a simple run function."""
        if isinstance(config, RuntimeConfig):
            runtime = cls(config)
        else:
            runtime = cls.from_config(config, environ=environ)
        return runtime.run_driver(driver, handle_signals=handle_signals)

    def run(self, run: RunFunction, *, handle_signals: bool = True) -> int:
        """Run until SIGINT/SIGTERM or `stop()` is called."""
        _configure_logging(self.config.log_level)
        try:
            asyncio.run(self.serve(run, handle_signals=handle_signals))
        except KeyboardInterrupt:
            pass
        return 0

    def run_driver(
        self,
        driver: RuntimeDriver | RunFunction,
        *,
        handle_signals: bool = True,
    ) -> int:
        """Run until shutdown using an optional Runtime Driver."""
        _configure_logging(self.config.log_level)
        try:
            asyncio.run(self.serve_driver(driver, handle_signals=handle_signals))
        except KeyboardInterrupt:
            pass
        return 0

    async def serve(
        self,
        run: RunFunction,
        *,
        handle_signals: bool = True,
        capabilities: dict[str, str] | None = None,
    ) -> None:
        """Async variant used by tests and embedding callers."""
        channel = ChannelClient(
            ChannelConfig(
                address=self.config.middleware.address,
                daemon_id=self.config.instance_id,
                runtime_name=self.config.runtime_name,
                auth_token=self.config.middleware.auth_token,
                runtime_type=self.config.runtime_type,
                max_concurrent_runs=self.config.max_concurrent_runs,
                version=__version__,
                labels=dict(self.config.labels),
                capabilities=dict(capabilities or {}),
                tls=self.config.middleware.tls,
            ),
            logger=logging.getLogger("levier_runtime.channel"),
        )
        manager = RunManager(
            channel,
            ManagerConfig(
                runtime_name=self.config.runtime_name,
                max_concurrent_runs=self.config.max_concurrent_runs,
            ),
            run,
            logger=logging.getLogger("levier_runtime.manager"),
        )
        channel.on_message = manager.handle
        channel.running_runs = manager.running_runs
        channel.running_count = manager.running_count

        self._stop_event = asyncio.Event()
        removers = _install_signal_handlers(self._stop_event) if handle_signals else []
        self.logger.info(
            "runtime starting name=%s instance=%s",
            self.config.runtime_name,
            self.config.instance_id,
        )
        await channel.start()
        try:
            await self._stop_event.wait()
        finally:
            for remove in removers:
                remove()
            await manager.close()
            await channel.stop()
            self.logger.info("runtime stopped name=%s", self.config.runtime_name)

    async def serve_driver(
        self,
        driver: RuntimeDriver | RunFunction,
        *,
        handle_signals: bool = True,
    ) -> None:
        """Serve a driver while keeping the simple callable path compatible."""
        await _call_driver_hook(driver, "on_start", self)
        capabilities = driver_capabilities(driver).as_labels()
        try:
            await self.serve(
                driver_run_function(driver),
                handle_signals=handle_signals,
                capabilities=capabilities,
            )
        finally:
            await _call_driver_hook(driver, "on_shutdown")

    def stop(self) -> None:
        """Request a graceful stop."""
        if self._stop_event is not None:
            self._stop_event.set()


async def _call_driver_hook(driver: RuntimeDriver | RunFunction, name: str, *args: Any) -> None:
    hook = getattr(driver, name, None)
    if not callable(hook):
        return
    result = hook(*args)
    if inspect.isawaitable(result):
        await result


def _install_signal_handlers(stop: asyncio.Event) -> list[Callable[[], None]]:
    loop = asyncio.get_running_loop()
    removers: list[Callable[[], None]] = []
    for signum in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(signum, stop.set)
        except (NotImplementedError, RuntimeError):
            continue
        removers.append(lambda signum=signum: loop.remove_signal_handler(signum))
    return removers


def _configure_logging(level: str) -> None:
    logging.basicConfig(
        level=level.upper(),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
