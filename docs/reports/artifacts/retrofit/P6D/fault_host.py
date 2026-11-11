"""Middleware — fault-injection App for P6D.

Connects as a real App, executes local tools, and then deliberately corrupts
the response path in one of these ways:

    duplicate   send the same local_tool_result twice
    mismatch    send a wrong correlation_id, wait for the error, then the real one
    unknown     send a fabricated invocation_id, then the real one
    foreign     send the result through a second App's connection, then the real one
    disconnect  close the connection after the first call, sending nothing
    slow        sleep past the tool timeout, then send the result

Usage:
    PYTHONPATH=examples/counter_agent python3 fault_host.py \
        --mode duplicate --credentials /tmp/app.json
"""

from __future__ import annotations

import argparse
import json
import os
import socket
import sys
import time
from pathlib import Path
from typing import Any

from credentials import load_credentials
from http_client import AppClient, UrllibTransport, parse_ws_token_response
from ws_client import WebSocketClient, websocket_url
from ws_tool_host import build_local_tool_result, default_credentials_path
from tools import Counter


def log(payload: dict[str, Any]) -> None:
    print(json.dumps(payload, ensure_ascii=False, sort_keys=True), flush=True)


def connect(credentials_path: str, mw_url: str) -> tuple[WebSocketClient, str]:
    credentials = load_credentials(Path(credentials_path).expanduser())
    client = AppClient(mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
    token = parse_ws_token_response(client.create_ws_token())
    url = websocket_url(mw_url, token.ws_url, credentials.app_id, token.token.reveal())
    return WebSocketClient.connect(url), credentials.app_id


def read_error(ws: WebSocketClient, seconds: float) -> dict[str, Any] | None:
    """Read until an error message arrives or the window closes."""
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            message = ws.recv_json(timeout=max(0.05, deadline - time.monotonic()))
        except socket.timeout:
            return None
        if message.get("type") == "error":
            return message
    return None


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Fault-injecting local tool host")
    parser.add_argument("--mode", required=True,
                        choices=["normal", "duplicate", "mismatch", "unknown", "foreign",
                                 "disconnect", "slow"])
    parser.add_argument("--credentials", default="")
    parser.add_argument("--foreign-credentials", default="",
                        help="second App used by --mode foreign")
    parser.add_argument("--mw-url", default=os.environ.get("MW_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--receive-count", type=int, default=1)
    parser.add_argument("--timeout", type=float, default=30.0)
    parser.add_argument("--delay", type=float, default=3.0, help="seconds for --mode slow")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = args.credentials or str(default_credentials_path())
    ws, app_id = connect(path, args.mw_url)
    foreign = None
    foreign_app_id = ""
    if args.mode == "foreign":
        if not args.foreign_credentials:
            print("--mode foreign requires --foreign-credentials", file=sys.stderr)
            return 2
        foreign, foreign_app_id = connect(args.foreign_credentials, args.mw_url)

    counter = Counter(0)
    log({"connected": True, "mode": args.mode, "app_id": app_id,
         "foreign_app_id": foreign_app_id, "counter": 0})
    handled = 0
    try:
        while handled < args.receive_count:
            try:
                message = ws.recv_json(timeout=args.timeout)
            except socket.timeout:
                log({"idle": True, "handled": handled})
                break
            if message.get("type") != "local_tool_call":
                if message.get("type") == "error":
                    log({"error": message})
                continue

            msg_id = message.get("msg_id")
            invocation_id = message.get("invocation_id")
            result = build_local_tool_result(counter, message)
            handled += 1

            if args.mode == "disconnect":
                log({"closing_before_result": True, "invocation_id": invocation_id})
                ws.close()
                return 0

            if args.mode == "duplicate":
                ws.send_json(result)
                log({"sent": result})
                ws.send_json(result)
                log({"sent_duplicate": True, "invocation_id": invocation_id})

            elif args.mode == "mismatch":
                bad = dict(result)
                bad["correlation_id"] = "bogus-" + str(msg_id)
                ws.send_json(bad)
                log({"sent_bad_correlation": bad["correlation_id"]})
                log({"server_error": read_error(ws, 2.0)})
                ws.send_json(result)
                log({"sent_correct": result})

            elif args.mode == "unknown":
                fake = {
                    "type": "local_tool_result",
                    "correlation_id": "unknown-" + str(msg_id),
                    "invocation_id": "unknown-" + str(invocation_id),
                    "ok": True,
                    "result_json": "{\"value\":-1}",
                }
                ws.send_json(fake)
                log({"sent_unknown_invocation": fake["invocation_id"]})
                log({"server_error": read_error(ws, 2.0)})
                ws.send_json(result)
                log({"sent_correct": result})

            elif args.mode == "foreign":
                assert foreign is not None
                forged = dict(result)
                foreign.send_json(forged)
                log({"sent_via_foreign_app": foreign_app_id,
                     "forged_invocation_id": invocation_id})
                log({"server_error": read_error(foreign, 2.0)})
                ws.send_json(result)
                log({"sent_correct": result})

            elif args.mode == "slow":
                log({"sleeping": args.delay, "invocation_id": invocation_id})
                time.sleep(args.delay)
                ws.send_json(result)
                log({"sent_late": True, "invocation_id": invocation_id})
                log({"server_error": read_error(ws, 2.0)})

            else:
                ws.send_json(result)
                log({"sent": result})
    finally:
        ws.close()
        if foreign is not None:
            foreign.close()
    log({"closed": True, "mode": args.mode, "handled": handled, "counter": counter.get()})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
