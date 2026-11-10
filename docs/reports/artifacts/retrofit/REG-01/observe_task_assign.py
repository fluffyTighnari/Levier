"""Middleware — one-off TaskAssign observer for registration verification.

Connects as daemon-local-1, creates an App + local tools over HTTP, creates
one echo run, prints the TaskAssign tools, then exits. This is a diagnostic
probe only; it is not part of the product runtime.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import json
import os
import pathlib
import sys
import urllib.request
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[5]
sys.path.insert(0, str(ROOT / "harness" / "src" / "gen"))
sys.path.insert(0, str(ROOT / "harness" / "src"))

from middleware.runtime.v1 import runtime_pb2  # noqa: E402
from middleware_harness.channel import ChannelClient, ChannelConfig  # noqa: E402

MW_URL = os.environ.get("MW_URL", "http://127.0.0.1:8080").rstrip("/")
ADMIN_TOKEN = os.environ.get("MW_ADMIN_TOKEN", "admin-change-me")
DAEMON_ID = os.environ.get("MW_DAEMON_ID", "daemon-local-1")
DAEMON_TOKEN = os.environ.get("MW_DAEMON_TOKEN", "change-me")
APP_NAME = f"retrofit-reg-{uuid.uuid4().hex[:10]}"

TOOLS = [
    {
        "name": "counter_add",
        "description": "Add an integer delta to the counter.",
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
        "description": "Read the current counter value.",
        "schema": {"type": "object", "properties": {}, "additionalProperties": False},
        "timeout_ms": 10000,
    },
]


def http(method: str, path: str, body: dict | None = None, token: str | None = None) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Accept": "application/json"}
    if data is not None:
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(MW_URL + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(request, timeout=15) as response:
        raw = response.read().decode()
        return json.loads(raw) if raw else {}


async def main() -> int:
    app_create = http("POST", "/admin/apps", {"name": APP_NAME, "multiagent": False}, ADMIN_TOKEN)
    app_id = app_create["app"]["app_id"]
    app_key = app_create.get("api_key") or ""
    if not app_key:
        raise RuntimeError("App already existed without a key; use a unique name")

    http(
        "POST",
        "/api/v1/apps",
        {"name": APP_NAME, "multiagent": False, "tools": TOOLS},
        app_key,
    )
    app_view = http("GET", f"/api/v1/apps/{app_id}", token=app_key)

    observed: dict = {}
    assigned = asyncio.Event()
    client: ChannelClient | None = None

    async def on_message(envelope: runtime_pb2.Envelope) -> None:
        if envelope.WhichOneof("payload") != "task_assign":
            return
        assign = envelope.task_assign
        observed.update(
            {
                "run_id": assign.run_id,
                "app_id": assign.app_id,
                "strategy": assign.strategy,
                "tool_count": len(assign.tools),
                "tools": [
                    {
                        "name": tool.name,
                        "description": tool.description,
                        "input_schema_json": tool.input_schema_json,
                        "scope": runtime_pb2.ToolScope.Name(tool.scope),
                    }
                    for tool in assign.tools
                ],
            }
        )
        ack = runtime_pb2.TaskAck(accepted=True)
        await client.send_response(envelope, ack, "task_ack")
        assigned.set()

    client = ChannelClient(
        ChannelConfig(
            address="127.0.0.1:9090",
            daemon_id=DAEMON_ID,
            auth_token=DAEMON_TOKEN,
            runtime_type="normal",
            max_concurrent_runs=4,
            strategies=["react", "echo"],
            labels={"pool": "registration-probe"},
        ),
        on_message=on_message,
    )
    await client.start()
    await client.wait_connected(5)

    created = await asyncio.to_thread(
        http,
        "POST",
        "/api/v1/runs",
        {
            "runtime": "normal",
            "strategy": {"name": "echo"},
            "input": [{"role": "user", "content": "registration probe"}],
            "model": {"provider": "anthropic", "model": "claude-test"},
            "timeouts": {"run_ms": 15000, "tool_ms": 5000},
        },
        app_key,
    )
    try:
        await asyncio.wait_for(assigned.wait(), timeout=5)
    finally:
        await client.stop()

    run_id = created.get("run_id", "")
    if run_id:
        try:
            await asyncio.to_thread(
                http,
                "POST",
                f"/api/v1/runs/{run_id}/cancel",
                {"reason": "registration probe cleanup"},
                app_key,
            )
        except Exception:
            pass

    output = {
        "app_id": app_id,
        "app_name": APP_NAME,
        "app_view": app_view,
        "registered_tools": TOOLS,
        "run_id": created.get("run_id"),
        "task_assign": observed,
    }
    print(json.dumps(output, indent=2, ensure_ascii=False))
    return 0 if observed.get("tool_count") == len(TOOLS) else 1


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
