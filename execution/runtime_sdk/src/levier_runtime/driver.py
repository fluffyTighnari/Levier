"""Optional Runtime Driver contract.

The simple `run(ctx)` function remains the primary Runtime entry point. A
driver is the optional composition layer for runtimes that need to declare
capabilities or manage their own lifecycle around that entry point.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Protocol, runtime_checkable

from .context import RunContext


@dataclass(frozen=True)
class RuntimeCapabilities:
    """Capabilities a Runtime may advertise to the control plane."""

    checkpoint: bool = False
    custom_events: bool = False
    runtime_tools: bool = False
    providers: bool = False
    waiting: bool = False
    sandbox: bool = False
    longterm: bool = False

    def as_dict(self) -> dict[str, bool]:
        """Return a plain capability mapping."""
        return {
            "checkpoint": self.checkpoint,
            "custom_events": self.custom_events,
            "runtime_tools": self.runtime_tools,
            "providers": self.providers,
            "waiting": self.waiting,
            "sandbox": self.sandbox,
            "longterm": self.longterm,
        }

    def as_labels(self) -> dict[str, str]:
        """Return capability labels for the RuntimeChannel handshake."""
        return {key: str(value).lower() for key, value in self.as_dict().items()}


@runtime_checkable
class RuntimeDriver(Protocol):
    """A composable Runtime execution owner.

    A simple callable remains valid. A driver additionally exposes capability
    metadata and may provide optional `on_start` / `on_shutdown` hooks.
    """

    capabilities: RuntimeCapabilities

    async def run(self, ctx: RunContext) -> str:
        """Execute one run and return a terminal status or ``waiting``."""
        ...


def driver_capabilities(value: Any) -> RuntimeCapabilities:
    """Return declared capabilities, or the minimal capability set."""
    capabilities = getattr(value, "capabilities", None)
    if isinstance(capabilities, RuntimeCapabilities):
        return capabilities
    return RuntimeCapabilities()


def driver_run_function(value: Any) -> Any:
    """Return the awaitable callable a driver exposes."""
    run = getattr(value, "run", None)
    if callable(run):
        return run
    if callable(value):
        return value
    raise TypeError("runtime driver must be callable or expose run(ctx)")


__all__ = [
    "RuntimeCapabilities",
    "RuntimeDriver",
    "driver_capabilities",
    "driver_run_function",
]
