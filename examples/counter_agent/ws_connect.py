"""Levier — connect to App WebSocket with a ws_token (P5.2).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import socket
import sys
import time
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
    parser = argparse.ArgumentParser(description="Connect to the App WebSocket")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("LEVIER_URL") or os.environ.get("MW_URL") or "http://127.0.0.1:8080")
    parser.add_argument("--hold-seconds", type=float, default=0.0)
    parser.add_argument("--ping", action="store_true", help="send one ping and require pong")
    parser.add_argument("--subscribe-run", default="", help="subscribe to one run and wait for caught_up")
    parser.add_argument("--last-attempt", type=int, default=0)
    parser.add_argument("--last-seq", type=int, default=0)
    parser.add_argument("--listen-seconds", type=float, default=0.0)
    parser.add_argument("--delay-seconds", type=float, default=0.0, help="wait after token issuance")
    parser.add_argument("--app-id-override", default="", help="test-only app_id override")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        response = client.create_ws_token()
        token = parse_ws_token_response(response)
        if args.delay_seconds > 0:
            time.sleep(args.delay_seconds)
        connect_app_id = args.app_id_override or credentials.app_id
        url = websocket_url(args.mw_url, token.ws_url, connect_app_id, token.token.reveal())
        ws = WebSocketClient.connect(url)
    except (CredentialsError, HttpError, HttpTransportError, WebSocketError, OSError) as exc:
        print(f"ws connect failed: {exc}", file=sys.stderr)
        return 1

    print(json.dumps({"connected": True, "app_id": connect_app_id, "ws_url": token.ws_url}), flush=True)
    pong = False
    try:
        if args.ping:
            ws.send_json({"type": "ping"})
            reply = ws.recv_json(timeout=5.0)
            pong = reply.get("type") == "pong"
            print(json.dumps({"pong": pong, "reply": reply}), flush=True)
            if not pong:
                return 1
        if args.subscribe_run:
            ws.send_json(
                {
                    "type": "subscribe",
                    "run_id": args.subscribe_run,
                    "last_attempt": args.last_attempt,
                    "last_seq": args.last_seq,
                }
            )
            received: list[dict] = []
            while True:
                reply = ws.recv_json(timeout=5.0)
                if reply.get("type") == "caught_up":
                    print(
                        json.dumps({"caught_up": reply, "events_before": len(received)}),
                        flush=True,
                    )
                    break
                if reply.get("type") == "error":
                    print(json.dumps({"error": reply}), flush=True)
                    return 1
                received.append(reply)
            if args.listen_seconds > 0:
                deadline = time.monotonic() + args.listen_seconds
                while time.monotonic() < deadline:
                    remaining = deadline - time.monotonic()
                    try:
                        reply = ws.recv_json(timeout=min(0.5, max(0.1, remaining)))
                    except socket.timeout:
                        continue
                    if reply.get("type") == "error":
                        print(json.dumps({"error": reply}), flush=True)
                        return 1
                    received.append(reply)
            summaries = [
                {
                    "type": item.get("type"),
                    "event_type": item.get("event_type"),
                    "attempt": item.get("attempt"),
                    "seq": item.get("seq"),
                    "data_status": (item.get("data") or {}).get("status"),
                }
                for item in received
            ]
            print(json.dumps({"received": summaries}), flush=True)
        if args.hold_seconds > 0:
            time.sleep(args.hold_seconds)
    finally:
        ws.close()
    print(json.dumps({"closed": True}), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
