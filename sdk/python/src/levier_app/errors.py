"""Exceptions raised by the Levier Python adapter.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations


class LevierError(RuntimeError):
    """Base error for adapter failures."""


class HTTPError(LevierError):
    """A non-success Levier HTTP response, normalized."""

    def __init__(
        self,
        status: int,
        code: str,
        message: str,
        retryable: bool = False,
    ) -> None:
        super().__init__(f"HTTP {status}: {code}: {message}")
        self.status = status
        self.code = code
        self.message = message
        self.retryable = retryable


class TransportError(LevierError):
    """A network failure before an HTTP response was received."""


class ProtocolError(LevierError):
    """A WebSocket handshake, frame, or message violated the adapter contract."""


class AppInactiveError(LevierError):
    """The App exists but is not active; retrying cannot fix it."""


class ToolExecutionError(LevierError):
    """A local tool handler failed with a stable error code."""

    def __init__(self, code: str, message: str, retryable: bool = False) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.retryable = retryable
