"""Levier — MCP runtime errors.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations


class McpRuntimeError(RuntimeError):
    """A protocol-visible MCP runtime error."""

    def __init__(
        self,
        code: str,
        message: str,
        *,
        retryable: bool = False,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.retryable = retryable

    def as_dict(self) -> dict[str, object]:
        """Return the JSON-RPC error object."""
        return {
            "code": self.code,
            "message": str(self),
            "data": {"retryable": self.retryable},
        }
