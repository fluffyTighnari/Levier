"""Middleware — App WebSocket local tool host (P6.3).

Connects, records incoming local_tool_call messages, and exits after the
requested count. It does not return a local_tool_result yet.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import socket
import sys
from pathlib import Path

from credentials import CredentialsError, load_credentials
from http_client import (
    AppClient,
    HttpError,
    HttpTransportError,
    UrllibTransport,
    parse_ws_token_response,
)
from ws_client import WebSocketClient, WebSocketError, websocket_url


def default_credentials_path() -> Path:
    value = os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "middleware" / "counter-app.json"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Record App local tool calls")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("MW_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--receive-count", type=int, default=1)
    parser.add_argument("--timeout", type=float, default=60.0)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        token = parse_ws_token_response(client.create_ws_token())
        url = websocket_url(args.mw_url, token.ws_url, credentials.app_id, token.token.reveal())
        ws = WebSocketClient.connect(url)
    except (CredentialsError, HttpError, HttpTransportError, WebSocketError, OSError) as exc:
        print(f"tool host failed: {exc}", file=sys.stderr)
        return 1

    print(json.dumps({"connected": True, "app_id": credentials.app_id}), flush=True)
    received = 0
    try:
        while received < args.receive_count:
            try:
                message = ws.recv_json(timeout=args.timeout)
            except socket.timeout:
                print("tool host timed out waiting for local_tool_call", file=sys.stderr)
                return 1
            if message.get("type") == "local_tool_call":
                record = {
                    "msg_id": message.get("msg_id"),
                    "invocation_id": message.get("invocation_id"),
                    "tool_name": message.get("tool_name"),
                    "arguments_json": message.get("arguments_json"),
                    "timeout_ms": message.get("timeout_ms"),
                }
                print(json.dumps({"local_tool_call": record}), flush=True)
                received += 1
            elif message.get("type") == "error":
                print(json.dumps({"error": message}), flush=True)
    finally:
        ws.close()
    print(json.dumps({"closed": True}), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
