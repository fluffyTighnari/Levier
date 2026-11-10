"""Middleware — connect to App WebSocket with a ws_token (P5.2).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import argparse
import json
import os
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
    parser.add_argument("--mw-url", default=os.environ.get("MW_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--hold-seconds", type=float, default=0.0)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        response = client.create_ws_token()
        token = parse_ws_token_response(response)
        url = websocket_url(args.mw_url, token.ws_url, credentials.app_id, token.token.reveal())
        socket = WebSocketClient.connect(url)
    except (CredentialsError, HttpError, HttpTransportError, WebSocketError, OSError) as exc:
        print(f"ws connect failed: {exc}", file=sys.stderr)
        return 1

    print(json.dumps({"connected": True, "app_id": credentials.app_id, "ws_url": token.ws_url}), flush=True)
    try:
        if args.hold_seconds > 0:
            time.sleep(args.hold_seconds)
    finally:
        socket.close()
    print(json.dumps({"closed": True}), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
