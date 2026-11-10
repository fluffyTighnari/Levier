"""Middleware — Counter App local tool dispatcher (P1.2–P1.6).

The dispatcher accepts the two fields Middleware sends to a local tool:
`tool_name` and `arguments_json`. It returns the success/error shape used by
WebSocket `local_tool_result`; the WS layer adds correlation/invocation IDs.

Design references:
- docs/DESIGN.md §5.3: local tools are executed by the App.
- docs/DESIGN.md §4.5: tool error codes.
- proto/middleware/runtime/v1/runtime.proto ToolCallResult / ErrorInfo.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
from typing import Any

from tools import Counter


def _error(code: str, message: str, retryable: bool = False) -> dict[str, Any]:
    return {
        "ok": False,
        "error": {
            "code": code,
            "message": message,
            "retryable": retryable,
        },
    }


def _parse_arguments(arguments_json: str) -> dict[str, Any]:
    if arguments_json is None:
        return {}
    if not isinstance(arguments_json, str):
        raise TypeError("arguments_json must be a string")
    if arguments_json.strip() == "":
        return {}
    parsed = json.loads(arguments_json)
    if not isinstance(parsed, dict):
        raise TypeError("arguments_json must be a JSON object")
    return parsed


def _success(value: int) -> dict[str, Any]:
    return {"ok": True, "result_json": json.dumps({"value": value})}


def dispatch_counter_tool(
    counter: Counter,
    tool_name: str,
    arguments_json: str,
) -> dict[str, Any]:
    """Dispatch one local counter tool call.

    P1.2–P1.4 cover the three success paths; P1.5/P1.6 add the remaining
    error cases. The WS layer later adds correlation/invocation IDs.
    """
    if tool_name not in {"counter_add", "counter_get", "counter_reset"}:
        return _error("TOOL_NOT_FOUND", f"unknown tool {tool_name}")

    try:
        arguments = _parse_arguments(arguments_json)
    except (json.JSONDecodeError, TypeError) as exc:
        return _error("TOOL_EXEC_ERROR", f"invalid arguments_json: {exc}")

    if tool_name == "counter_get":
        return _success(counter.get())
    if tool_name == "counter_reset":
        return _success(counter.reset())

    if "delta" not in arguments:
        return _error("TOOL_EXEC_ERROR", "counter_add.delta is required")
    delta = arguments["delta"]
    if not isinstance(delta, int) or isinstance(delta, bool):
        return _error("TOOL_EXEC_ERROR", "counter_add.delta must be an integer")

    try:
        value = counter.add(delta)
    except TypeError as exc:
        return _error("TOOL_EXEC_ERROR", str(exc))

    return _success(value)
