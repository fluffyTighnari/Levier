"""Middleware — harness structured logging.

Provides JSON and text logging without exposing the daemon token.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import logging
import sys
from datetime import datetime, timezone
from typing import Any

_LOG_FIELDS = (
    "daemon_id",
    "run_id",
    "attempt",
    "lease_epoch",
    "strategy",
    "step",
    "invocation_id",
    "app_id",
    "trace_context",
    "tool_count",
    "tools",
    "mcp_binding_count",
    "mcp_bindings",
    "session_id",
)


class JsonFormatter(logging.Formatter):
    """Format log records as one JSON object per line."""

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "time": datetime.fromtimestamp(record.created, tz=timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        for field in _LOG_FIELDS:
            value = getattr(record, field, None)
            if value is not None:
                payload[field] = value
        if record.exc_info:
            payload["exception"] = self.formatException(record.exc_info)
        return json.dumps(payload, ensure_ascii=False, separators=(",", ":"))


def configure_logging(level: str, log_format: str, stream: Any = None) -> None:
    """Configure the root logger for the harness process."""
    output = sys.stdout if stream is None else stream
    handler = logging.StreamHandler(output)

    if log_format == "json":
        handler.setFormatter(JsonFormatter())
    else:
        handler.setFormatter(
            logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s")
        )

    root = logging.getLogger()
    root.handlers.clear()
    root.setLevel(level.upper())
    root.addHandler(handler)


def get_logger(name: str = "middleware_harness") -> logging.Logger:
    """Return a named harness logger."""
    return logging.getLogger(name)
