"""Levier — harness health state.

The health object is intentionally independent of gRPC/HTTP so the
runtime can report readiness without owning transport state.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any


@dataclass
class HealthState:
    """Mutable readiness state for the harness."""

    daemon_id: str
    version: str
    ready: bool = False
    checks: dict[str, bool] = field(default_factory=dict)

    def set_ready(self, ready: bool) -> None:
        """Set overall readiness."""
        self.ready = ready

    def set_check(self, name: str, ok: bool) -> None:
        """Set one named check."""
        self.checks[name] = ok

    def snapshot(self) -> dict[str, Any]:
        """Return a JSON-friendly health snapshot."""
        return {
            "ready": self.ready,
            "daemon_id": self.daemon_id,
            "version": self.version,
            "checks": dict(self.checks),
            "time": datetime.now(tz=timezone.utc).isoformat(),
        }
