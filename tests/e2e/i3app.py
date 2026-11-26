#!/usr/bin/env python3
"""Test App for the I3 multi-replica matrix.

It deliberately uses the low-level Python adapter so the test can observe the
WebSocket event stream and local tool invocations. Normal Apps use the smaller
``levier.App`` facade and do not need this file.
"""

from __future__ import annotations

import argparse
import json
import sys
import threading
import time
from pathlib import Path
from typing import Any

from levier_app import LevierApp


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--credentials", required=True)
    parser.add_argument("--server", required=True)
    parser.add_argument("--log", required=True)
    parser.add_argument("--subscribe-file", default="")
    parser.add_argument("--ping-interval", type=float, default=1.0)
    args = parser.parse_args()
    log_path = Path(args.log)

    def log(record: dict[str, Any]) -> None:
        with log_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(record, separators=(",", ":")) + "\n")

    def on_event(message: dict[str, Any]) -> None:
        log({"kind": "ws", "message": message})

    app = LevierApp.from_credentials(
        args.credentials,
        server=args.server,
        on_event=on_event,
    )
    original_hello = app._handle_hello_ack

    def handle_hello(message: dict[str, Any]) -> None:
        original_hello(message)
        log({
            "kind": "hello_ack",
            "connection_id": message.get("connection_id", ""),
            "protocol": message.get("protocol", 0),
        })

    app._handle_hello_ack = handle_hello  # type: ignore[method-assign]
    counter = {"value": 0}

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
    def counter_add(arguments: dict[str, Any]) -> dict[str, int]:
        delta = int(arguments.get("delta", 0))
        counter["value"] += delta
        log({"kind": "tool", "tool": "counter_add", "delta": delta, "value": counter["value"]})
        return {"value": counter["value"]}

    @app.tool(
        "counter_get",
        description="Read the current counter value.",
        schema={"type": "object", "properties": {}, "additionalProperties": False},
    )
    def counter_get(arguments: dict[str, Any]) -> dict[str, int]:
        log({"kind": "tool", "tool": "counter_get", "value": counter["value"]})
        return {"value": counter["value"]}

    if args.subscribe_file:
        subscribe_path = Path(args.subscribe_file)
        seen: set[str] = set()

        def watch_subscriptions() -> None:
            while True:
                try:
                    lines = subscribe_path.read_text(encoding="utf-8").splitlines()
                except OSError:
                    lines = []
                for run_id in lines:
                    run_id = run_id.strip()
                    if run_id and run_id not in seen:
                        seen.add(run_id)
                        app.subscribe_run(run_id)
                        log({"kind": "subscribe", "run_id": run_id})
                time.sleep(0.05)

        threading.Thread(target=watch_subscriptions, name="i3-subscribe", daemon=True).start()

    log({"kind": "start", "app_id": app.credentials.app_id})
    return app.run(
        handle_signals=False,
        ping_interval=args.ping_interval,
        reconnect_initial=0.2,
        reconnect_max=2.0,
    )


if __name__ == "__main__":
    raise SystemExit(main())
