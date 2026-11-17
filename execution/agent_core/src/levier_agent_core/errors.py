"""Levier — agent core errors raised by adapters."""

from __future__ import annotations


class ProviderFailure(RuntimeError):
    """Adapter-normalized provider error.

    Adapters translate provider-specific exceptions into this shape so the
    core can emit a stable error event without importing provider code.
    """

    def __init__(self, code: str, message: str, *, retryable: bool = False) -> None:
        super().__init__(message)
        self.code = code
        self.retryable = retryable
