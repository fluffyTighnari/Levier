#!/usr/bin/env python3
"""Middleware — App onboarding conformance check (P8R.5).

Two modes:

  protocol (default)
    health, app key, optional tool registration, ws-token, WS handshake,
    ping/pong, unknown-invocation error, malformed-message error,
    optional subscribe/caught_up.

  --run-tool
    submits one Run, waits for a tool call, and verifies the App actually
    served it. Requires the App under test to be running.

Usage:
  python3 scripts/app_check.py --credentials /path/app.json
  python3 scripts/app_check.py --credentials /path/app.json \
      --run-tool --tool-name counter_add --arguments '{"delta":2}'
"""

from __future__ import annotations

import argparse
import json
import socket
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "examples" / "counter_agent"))

from ws_client import WebSocketClient, WebSocketError, websocket_url  # noqa: E402


def request(
    method: str,
    url: str,
    *,
    token: str = "",
    body: dict[str, Any] | None = None,
    expected: tuple[int, ...] = (200,),
) -> tuple[int, dict[str, Any]]:
    headers = {"Accept": "application/json"}
    data = None
    if body is not None:
        data = json.dumps(body, separators=(",", ":")).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            status = response.status
            payload = json.loads(response.read().decode() or "{}")
    except urllib.error.HTTPError as exc:
        status = exc.code
        payload = json.loads(exc.read().decode() or "{}")
    if status not in expected:
        raise RuntimeError(f"{method} {url} -> {status}: {payload}")
    return status, payload


def wait_for(ws: WebSocketClient, kind: str, timeout: float) -> dict[str, Any]:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            message = ws.recv_json(timeout=min(1.0, deadline - time.monotonic()))
        except socket.timeout:
            continue
        if message.get("type") == kind:
            return message
    raise RuntimeError(f"timed out waiting for {kind}")


def check_protocol(args: argparse.Namespace, credentials: dict[str, str]) -> dict[str, Any]:
    base = args.mw_url.rstrip("/")
    app_id = credentials["app_id"]
    key = credentials["api_key"]
    result: dict[str, Any] = {"mode": "protocol", "checks": {}}

    status, health = request("GET", base + "/healthz", expected=(200,))
    result["checks"]["health"] = health.get("status", "")

    request("GET", f"{base}/api/v1/apps/{app_id}", token=key, expected=(200,))
    request("GET", f"{base}/api/v1/apps/{app_id}", token="invalid-key", expected=(401,))
    result["checks"]["app_key"] = "ok"

    if args.tools_file:
        tools = json.loads(Path(args.tools_file).read_text())
        if isinstance(tools, dict):
            tools = tools.get("tools")
        request(
            "POST",
            base + "/api/v1/apps",
            token=key,
            body={"name": credentials["app_name"], "tools": tools},
            expected=(200,),
        )
        result["checks"]["tools"] = f"registered {len(tools)}"

    _, token_payload = request("POST", base + "/api/v1/ws-token", token=key, expected=(200,))
    ws_token = token_payload["ws_token"]
    url = websocket_url(base, token_payload["ws_url"], app_id, ws_token)
    ws = WebSocketClient.connect(url)
    try:
        ws.send_json({"type": "hello", "protocol": 1, "client": "app-check"})
        ws.send_json({"type": "ping"})
        wait_for(ws, "pong", 5.0)
        result["checks"]["ws"] = "connected/pong"

        ws.send_json({
            "type": "local_tool_result",
            "correlation_id": "app-check",
            "invocation_id": "app-check-unknown",
            "ok": True,
            "result_json": "{}",
        })
        error = wait_for(ws, "error", 5.0)
        code = (error.get("error") or {}).get("code")
        if code != "LOCAL_TOOL_UNKNOWN":
            raise RuntimeError(f"unknown invocation returned {code!r}")
        result["checks"]["result_validation"] = code

        ws.send_json({"type": "not-a-message"})
        error = wait_for(ws, "error", 5.0)
        code = (error.get("error") or {}).get("code")
        if code != "BAD_MESSAGE":
            raise RuntimeError(f"malformed message returned {code!r}")
        result["checks"]["message_validation"] = code

        if args.subscribe_run:
            ws.send_json({
                "type": "subscribe",
                "run_id": args.subscribe_run,
                "last_attempt": 0,
                "last_seq": 0,
            })
            caught_up = wait_for(ws, "caught_up", 10.0)
            result["checks"]["subscribe"] = f"attempt={caught_up.get('attempt')} seq={caught_up.get('seq')}"
    finally:
        ws.close()

    try:
        WebSocketClient.connect(
            websocket_url(base, token_payload["ws_url"], app_id, "invalid-token")
        ).close()
        raise RuntimeError("invalid ws token was accepted")
    except WebSocketError:
        result["checks"]["ws_token_rejected"] = "ok"

    return result


def check_tool(args: argparse.Namespace, credentials: dict[str, str]) -> dict[str, Any]:
    base = args.mw_url.rstrip("/")
    key = credentials["api_key"]
    prompt = args.prompt or (
        f"Call the tool named {args.tool_name} exactly once with these arguments: "
        f"{args.arguments}. Then report the exact result it returned."
    )
    _, created = request(
        "POST",
        base + "/api/v1/runs",
        token=key,
        body={
            "runtime": "normal",
            "strategy": {"name": "react", "params": {"max_steps": 4}},
            "input": [{"role": "user", "content": prompt}],
            "model": {"provider": args.provider, "model": args.model},
            "timeouts": {"run_ms": args.run_ms, "tool_ms": args.tool_ms},
        },
        expected=(202,),
    )
    run_id = created["run_id"]
    deadline = time.monotonic() + args.run_ms / 1000 + 10
    status = "queued"
    while time.monotonic() < deadline:
        _, view = request("GET", f"{base}/api/v1/runs/{run_id}", token=key, expected=(200,))
        status = view.get("status", "")
        if status in {"completed", "failed", "cancelled", "timeout"}:
            break
        time.sleep(0.5)
    if status not in {"completed", "failed"}:
        raise RuntimeError(f"run ended as {status}")

    _, events = request(
        "GET",
        f"{base}/api/v1/runs/{run_id}/events?limit=1000",
        token=key,
        expected=(200,),
    )
    calls = [
        event for event in events.get("events", [])
        if event.get("event_type") == "TOOL_CALL_END"
        and (event.get("data") or {}).get("tool_name") == args.tool_name
    ]
    if not calls:
        raise RuntimeError(f"no TOOL_CALL_END for {args.tool_name!r}")
    last = calls[-1].get("data") or {}
    if args.expect_error:
        if last.get("ok") is not False or last.get("error_code") != args.expect_error:
            raise RuntimeError(f"expected {args.expect_error}, got {last}")
    else:
        if last.get("ok") is not True:
            raise RuntimeError(f"tool call failed: {last}")
        if args.expect_substring and args.expect_substring not in str(last.get("result_json", "")):
            raise RuntimeError(f"result missing {args.expect_substring!r}: {last}")
    return {
        "mode": "run-tool",
        "run_id": run_id,
        "status": status,
        "tool_name": args.tool_name,
        "tool_ok": last.get("ok"),
        "result_json": last.get("result_json", ""),
        "calls": len(calls),
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="App onboarding conformance check")
    parser.add_argument("--credentials", required=True, help="0600 JSON with app_id/app_name/api_key")
    parser.add_argument("--mw-url", default="http://127.0.0.1:8080")
    parser.add_argument("--tools-file", default="", help="optional JSON tool list to register")
    parser.add_argument("--subscribe-run", default="", help="optional run_id for caught_up check")
    parser.add_argument("--run-tool", action="store_true", help="verify a real tool call")
    parser.add_argument("--tool-name", default="")
    parser.add_argument("--arguments", default="{}")
    parser.add_argument("--prompt", default="")
    parser.add_argument("--provider", default="deepseek")
    parser.add_argument("--model", default="deepseek-flash")
    parser.add_argument("--run-ms", type=int, default=120000)
    parser.add_argument("--tool-ms", type=int, default=10000)
    parser.add_argument("--expect-error", default="", help="expected tool error code")
    parser.add_argument("--expect-substring", default="")
    parser.add_argument("--json", action="store_true")
    return parser


def main() -> int:
    args = build_parser().parse_args()
    credentials = json.loads(Path(args.credentials).read_text())
    for field in ("app_id", "app_name", "api_key"):
        if not credentials.get(field):
            raise SystemExit(f"credentials missing {field}")
    try:
        result = check_protocol(args, credentials)
        if args.run_tool:
            if not args.tool_name:
                raise SystemExit("--run-tool requires --tool-name")
            result = {"protocol": result, "tool": check_tool(args, credentials)}
    except Exception as exc:  # noqa: BLE001 - CLI must surface one clear error.
        if args.json:
            print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
        else:
            print(f"app-check: FAIL: {exc}", file=sys.stderr)
        return 1

    if args.json:
        print(json.dumps({"ok": True, "result": result}, ensure_ascii=False))
    else:
        print(json.dumps(result, ensure_ascii=False, indent=2))
        print("app-check: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
