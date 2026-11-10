"""Middleware — Counter App local tool dispatcher (P1.2).

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
    if arguments_json is None or arguments_json.strip() == "":
        return {}
    parsed = json.loads(arguments_json)
    if not isinstance(parsed, dict):
        raise TypeError("arguments_json must be a JSON object")
    return parsed


def dispatch_counter_tool(
    counter: Counter,
    tool_name: str,
    arguments_json: str,
) -> dict[str, Any]:
    """Dispatch one local counter tool call.

    P1.2 intentionally implements only `counter_add`. Later P1 steps add
    `counter_get`, `counter_reset`, and error cases.
    """
    if tool_name != "counter_add":
        return _error("TOOL_NOT_FOUND", f"unknown tool {tool_name}")

    try:
        arguments = _parse_arguments(arguments_json)
    except (json.JSONDecodeError, TypeError) as exc:
        return _error("TOOL_EXEC_ERROR", f"invalid arguments_json: {exc}")

    delta = arguments.get("delta")
    if not isinstance(delta, int) or isinstance(delta, bool):
        return _error("TOOL_EXEC_ERROR", "counter_add.delta must be an integer")

    try:
        value = counter.add(delta)
    except TypeError as exc:
        return _error("TOOL_EXEC_ERROR", str(exc))

    return {"ok": True, "result_json": json.dumps({"value": value})}
