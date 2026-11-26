#!/usr/bin/env python3
"""Minimal v1 App WebSocket client for the I3 compatibility matrix.

v1 does not send a hello. It obtains a stateless WS token over HTTP, connects,
sends an explicit subscribe, and records every message. The same token is
validated by a different app-gateway process than the control that issued it.
"""

from __future__ import annotations

import argparse
import json
import socket
import sys
import time
from pathlib import Path
from typing import Any

from levier_app.credentials import load_credentials
from levier_app.http import LevierClient
from levier_app.websocket import WebSocketClient, websocket_url


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--credentials", required=True)
    parser.add_argument("--server", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--log", required=True)
    parser.add_argument("--timeout", type=float, default=30.0)
    args = parser.parse_args()
    credentials = load_credentials(args.credentials)
    client = LevierClient(
        args.server,
        credentials.app_id,
        credentials.api_key.reveal(),
    )
    token = client.create_ws_token()
    url = websocket_url(args.server, token.ws_url, credentials.app_id, token.token.reveal())
    ws = WebSocketClient.connect(url)
    log_path = Path(args.log)

    def log(record: dict[str, Any]) -> None:
        with log_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(record, separators=(",", ":")) + "\n")

    log({"kind": "connected", "protocol": 1})
    ws.send_json({
        "type": "subscribe",
        "run_id": args.run_id,
        "last_attempt": 0,
        "last_seq": 0,
    })
    deadline = time.monotonic() + args.timeout
    try:
        while time.monotonic() < deadline:
            try:
                message = ws.recv_json(timeout=1.0)
            except socket.timeout:
                continue
            log({"kind": "ws", "message": message})
            if message.get("type") == "run_completed":
                return 0
    finally:
        ws.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
