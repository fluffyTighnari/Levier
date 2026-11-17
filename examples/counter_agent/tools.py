"""Levier — Counter App local functions and registration metadata.

This is the first small retrofit step: the existing counter state stays local,
but its functions and registration shape become explicit.

Design references:
- docs/DESIGN.md §5.3: local tools are executed by the App; Levier only routes.
- docs/DESIGN.md §4.2: App HTTP registration body uses tools[].schema.
- proto/middleware/runtime/v1/runtime.proto ToolManifest:
  name / description / input_schema_json.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
from typing import Any


class Counter:
    """The App-owned counter state."""

    def __init__(self, initial: int = 0) -> None:
        if not isinstance(initial, int) or isinstance(initial, bool):
            raise TypeError("initial counter value must be an integer")
        self.value = initial

    def add(self, delta: int) -> int:
        if not isinstance(delta, int) or isinstance(delta, bool):
            raise TypeError("delta must be an integer")
        self.value += delta
        return self.value

    def get(self) -> int:
        return self.value

    def reset(self) -> int:
        self.value = 0
        return self.value


# Shape: POST /api/v1/apps { "tools": [ ... ] }
# Levier converts schema -> TaskAssign ToolManifest.input_schema_json.
TOOL_DEFINITIONS: list[dict[str, Any]] = [
    {
        "name": "counter_add",
        "description": "Add an integer delta to the counter. Use this for every arithmetic change.",
        "schema": {
            "type": "object",
            "properties": {"delta": {"type": "integer"}},
            "required": ["delta"],
            "additionalProperties": False,
        },
        "timeout_ms": 10000,
    },
    {
        "name": "counter_get",
        "description": "Read the current counter value. Use this before reporting state.",
        "schema": {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        },
        "timeout_ms": 10000,
    },
    {
        "name": "counter_reset",
        "description": "Reset the counter to zero.",
        "schema": {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        },
        "timeout_ms": 10000,
    },
]


def tool_manifests() -> list[dict[str, str]]:
    """Return the TaskAssign ToolManifest shape.

    Levier stores registration `schema` as `schema_json`, then emits it
    to Harness as `ToolManifest.input_schema_json`.
    """
    return [
        {
            "name": tool["name"],
            "description": tool["description"],
            "input_schema_json": json.dumps(tool["schema"]),
        }
        for tool in TOOL_DEFINITIONS
    ]
