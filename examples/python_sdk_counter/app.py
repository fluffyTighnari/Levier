"""Minimal Counter App built with the Levier Python adapter.

The business state and three handlers are the only App-specific code here.
Credentials, registration, WebSocket lifecycle, reconnects, and tool result
framing are handled by levier_app.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from levier_app import LevierApp

app = LevierApp.from_credentials()
state = {"value": 0}


@app.tool(
    "counter_add",
    description="Add an integer delta to the counter.",
    schema={
        "type": "object",
        "properties": {"delta": {"type": "integer"}},
        "required": ["delta"],
        "additionalProperties": False,
    },
)
def counter_add(arguments):
    state["value"] += arguments["delta"]
    return {"value": state["value"]}


@app.tool(
    "counter_get",
    description="Read the current counter value.",
    schema={"type": "object", "properties": {}, "additionalProperties": False},
)
def counter_get(_arguments):
    return {"value": state["value"]}


@app.tool(
    "counter_reset",
    description="Reset the counter to zero.",
    schema={"type": "object", "properties": {}, "additionalProperties": False},
)
def counter_reset(_arguments):
    state["value"] = 0
    return {"value": state["value"]}


if __name__ == "__main__":
    raise SystemExit(app.run())
