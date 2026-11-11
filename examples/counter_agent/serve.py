"""Middleware — Counter App runtime entrypoint (Middleware mode).

This is the App as Middleware sees it:

    verify App key
      → register local tools
      → open the App WebSocket
      → serve local_tool_call / local_tool_result until idle

The standalone baseline lives in `main.py` and is intentionally untouched.
See `docs/APP_ONBOARDING.md` for the procedure this file implements.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from credentials import CredentialsError, load_credentials
from http_client import (
    AppClient,
    HttpError,
    HttpTransportError,
    UrllibTransport,
    parse_app_view,
    parse_ws_token_response,
)
from register_tools import load_tools
from tools import Counter
from ws_client import WebSocketClient, WebSocketError, websocket_url
from ws_tool_host import default_credentials_path, serve_calls


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Serve the Counter App to Middleware")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("MW_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--tools-file", default="", help="optional JSON tool list")
    parser.add_argument(
        "--skip-register",
        action="store_true",
        help="do not POST the tool list at startup",
    )
    parser.add_argument("--initial", type=int, default=0, help="initial counter value")
    parser.add_argument(
        "--timeout",
        type=float,
        default=10.0,
        help="seconds to wait for the next message before exiting",
    )
    parser.add_argument(
        "--receive-count",
        type=int,
        default=0,
        help="exit after handling N calls; 0 keeps serving",
    )
    return parser


def _log(payload: dict) -> None:
    print(json.dumps(payload, ensure_ascii=False, sort_keys=True), flush=True)


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())

        app = parse_app_view(client.get_app())
        if app.status != "active":
            print(f"app is not active: {app.status}", file=sys.stderr)
            return 1

        tools: list[dict] = []
        if not args.skip_register:
            tools = load_tools(args.tools_file)
            parse_app_view(client.update_tools(credentials.app_name, tools))

        token = parse_ws_token_response(client.create_ws_token())
        url = websocket_url(args.mw_url, token.ws_url, credentials.app_id, token.token.reveal())
        ws = WebSocketClient.connect(url)
    except (CredentialsError, HttpError, HttpTransportError, WebSocketError, OSError, ValueError) as exc:
        print(f"serve failed: {exc}", file=sys.stderr)
        return 1

    counter = Counter(args.initial)
    _log({
        "serving": True,
        "app_id": credentials.app_id,
        "app_name": credentials.app_name,
        "registered_tools": [tool["name"] for tool in tools],
        "counter": counter.get(),
    })
    try:
        handled, reason = serve_calls(
            ws,
            counter,
            receive_count=args.receive_count,
            timeout=args.timeout,
            emit=_log,
        )
    finally:
        ws.close()
    _log({"stopped": True, "reason": reason, "handled": handled, "counter": counter.get()})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
