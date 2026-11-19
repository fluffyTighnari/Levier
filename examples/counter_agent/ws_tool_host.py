"""Levier — App WebSocket local tool host (P6.3–P6.4).

Hosts the App's local tools over the Levier WebSocket: receive
`local_tool_call`, execute the App function, return `local_tool_result`
correlated by `msg_id`.

This module is also the shared serving loop used by `serve.py`, the App
runtime entrypoint. It is not the standalone baseline (`main.py`).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import socket
import sys
from pathlib import Path
from typing import Any, Callable

from credentials import CredentialsError, load_credentials, resolve_server
from dispatch import dispatch_counter_tool
from http_client import (
    AppClient,
    HttpError,
    HttpTransportError,
    UrllibTransport,
    parse_ws_token_response,
)
from tools import Counter
from ws_client import WebSocketClient, WebSocketError, websocket_url


def default_credentials_path() -> Path:
    value = os.environ.get("LEVIER_APP_CREDENTIALS") or os.environ.get("LEVIER_CREDENTIALS") or os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "levier" / "counter-app.json"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Serve App local tool calls over WebSocket")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default="")
    parser.add_argument(
        "--receive-count",
        type=int,
        default=0,
        help="exit after handling N calls; 0 keeps serving",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=10.0,
        help="seconds to wait for the next message before exiting",
    )
    parser.add_argument("--initial", type=int, default=0, help="initial counter value")
    return parser


def build_local_tool_result(counter: Counter, message: dict[str, Any]) -> dict[str, Any]:
    """Execute one local_tool_call and build the correlated result message."""
    tool_name = str(message.get("tool_name") or "")
    arguments_json = message.get("arguments_json")
    if arguments_json is None:
        arguments_json = ""
    outcome = dispatch_counter_tool(counter, tool_name, arguments_json)
    result: dict[str, Any] = {
        "type": "local_tool_result",
        "correlation_id": message.get("msg_id"),
        "invocation_id": message.get("invocation_id"),
    }
    if outcome.get("ok"):
        result["ok"] = True
        result["result_json"] = outcome["result_json"]
    else:
        result["ok"] = False
        result["error"] = outcome["error"]
    return result


def serve_calls(
    ws: Any,
    counter: Counter,
    *,
    receive_count: int = 0,
    timeout: float = 10.0,
    emit: Callable[[dict[str, Any]], None] | None = None,
) -> tuple[int, str]:
    """Serve local tool calls until a count, idle timeout, or run completion.

    Returns (handled, reason) where reason is one of:
    "count", "idle", "completed".
    """
    log = emit or (lambda payload: None)
    handled = 0
    while receive_count <= 0 or handled < receive_count:
        try:
            message = ws.recv_json(timeout=timeout)
        except socket.timeout:
            return handled, "idle"

        kind = message.get("type")
        if kind == "local_tool_call":
            if not message.get("msg_id") or not message.get("invocation_id"):
                log({"ignored": "local_tool_call without correlation ids"})
                continue
            result = build_local_tool_result(counter, message)
            ws.send_json(result)
            handled += 1
            log({
                "local_tool_call": {
                    "msg_id": message.get("msg_id"),
                    "invocation_id": message.get("invocation_id"),
                    "tool_name": message.get("tool_name"),
                    "arguments_json": message.get("arguments_json"),
                },
                "local_tool_result": result,
                "counter": counter.get(),
            })
        elif kind == "run_completed":
            log({"run_completed": message, "handled": handled})
            return handled, "completed"
        elif kind == "error":
            log({"error": message})
    return handled, "count"


def _log(payload: dict[str, Any]) -> None:
    print(json.dumps(payload, ensure_ascii=False, sort_keys=True), flush=True)


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        base_url = resolve_server(args.mw_url, credentials)
        client = AppClient(base_url, credentials.app_id, credentials.api_key, UrllibTransport())
        token = parse_ws_token_response(client.create_ws_token())
        url = websocket_url(base_url, token.ws_url, credentials.app_id, token.token.reveal())
        ws = WebSocketClient.connect(url)
    except (CredentialsError, HttpError, HttpTransportError, WebSocketError, OSError) as exc:
        print(f"tool host failed: {exc}", file=sys.stderr)
        return 1

    counter = Counter(args.initial)
    _log({"connected": True, "app_id": credentials.app_id, "counter": counter.get()})
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
    if reason == "idle" and handled == 0:
        print("tool host timed out waiting for local_tool_call", file=sys.stderr)
        return 1
    if reason == "idle":
        _log({"idle_timeout": True, "handled": handled})
    _log({"closed": True, "handled": handled, "counter": counter.get()})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
