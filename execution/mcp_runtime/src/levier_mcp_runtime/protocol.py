"""Middleware — JSON-RPC framing for the MCP runtime.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
from typing import Any


def decode_request(line: bytes) -> dict[str, Any]:
    """Parse one JSON-RPC request line."""
    try:
        request = json.loads(line.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ValueError("invalid JSON-RPC request") from exc
    if not isinstance(request, dict):
        raise ValueError("JSON-RPC request must be an object")
    if request.get("jsonrpc") != "2.0":
        raise ValueError("jsonrpc must be 2.0")
    if not isinstance(request.get("method"), str):
        raise ValueError("method is required")
    return request


def encode_response(request_id: Any, result: Any) -> bytes:
    """Encode one successful JSON-RPC response."""
    payload = {"jsonrpc": "2.0", "id": request_id, "result": result}
    return (json.dumps(payload, ensure_ascii=False) + "\n").encode("utf-8")


def encode_error(request_id: Any, error: dict[str, Any]) -> bytes:
    """Encode one JSON-RPC error response."""
    payload = {"jsonrpc": "2.0", "id": request_id, "error": error}
    return (json.dumps(payload, ensure_ascii=False) + "\n").encode("utf-8")
