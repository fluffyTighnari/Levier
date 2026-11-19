#!/usr/bin/env python3
"""Levier — Docker Compose smoke check.

Waits for Levier readiness, Runtime registration, App WebSocket, MCP
runtime health, then submits one echo run and verifies it completes.

Uses only the Python standard library.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import socket
import sys
import time
import urllib.error
import urllib.request

MW_URL = (os.environ.get("LEVIER_URL") or os.environ.get("MW_URL") or "http://levier:8080").rstrip("/")
MCP_SOCKET = os.environ.get("MCP_SOCKET", "/run/levier/mcp.sock")
CREDENTIALS = os.environ.get("CREDENTIALS", "/data/credentials.json")
DAEMON_ID = os.environ.get("DAEMON_ID", "echo-local-1")
RUNTIME_NAME = os.environ.get("RUNTIME_NAME", "echo1")


def log(message: str) -> None:
    print(f"docker-smoke: {message}", flush=True)


def wait_for(description: str, predicate, timeout: float = 120.0) -> None:
    deadline = time.time() + timeout
    last = ""
    while time.time() < deadline:
        try:
            ok, last = predicate()
            if ok:
                log(f"{description}: ok")
                return
        except OSError as exc:
            last = str(exc)
        time.sleep(0.5)
    raise SystemExit(f"docker-smoke: {description} timed out ({last})")


def http(method: str, path: str, body: dict | None = None, token: str = "") -> tuple[int, dict]:
    data = None if body is None else json.dumps(body).encode()
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(MW_URL + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            raw = response.read()
            return response.status, json.loads(raw.decode()) if raw else {}
    except urllib.error.HTTPError as exc:
        return exc.code, {}


def metrics() -> str:
    with urllib.request.urlopen(MW_URL + "/metrics", timeout=5) as response:
        return response.read().decode("utf-8", "replace")


def read_credentials() -> dict:
    with open(CREDENTIALS, encoding="utf-8") as handle:
        return json.load(handle)


def mcp_health() -> dict:
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
        client.settimeout(5)
        client.connect(MCP_SOCKET)
        client.sendall(b'{"jsonrpc":"2.0","id":1,"method":"mcp.health","params":{}}\n')
        line = client.makefile("rb").readline()
    payload = json.loads(line.decode())
    if "error" in payload:
        raise RuntimeError(payload["error"])
    return payload["result"]


def main() -> int:
    wait_for("middleware /healthz", lambda: (http("GET", "/healthz")[0] == 200, ""))
    wait_for("middleware /readyz", lambda: (http("GET", "/readyz")[0] == 200, ""))

    def meta_ok():
        status, data = http("GET", "/meta")
        return (
            status == 200
            and data.get("service") == "levier"
            and data.get("meta_version") == 1
            and data.get("api", {}).get("http_version") == "v1",
            data,
        )

    wait_for("service /meta", meta_ok)

    def status_ok():
        status, data = http("GET", "/status")
        return (
            status == 200
            and data.get("status_version") == 1
            and data.get("service") == "levier"
            and data.get("health", {}).get("liveness") == "ok",
            data,
        )

    wait_for("service /status", status_ok)
    wait_for(
        "runtime registered",
        lambda: ('daemon="' + DAEMON_ID + '"' in metrics(), ""),
    )

    def credentials_ready():
        try:
            data = read_credentials()
        except (OSError, json.JSONDecodeError) as exc:
            return False, str(exc)
        if not data.get("app_id") or not data.get("api_key"):
            return False, "missing app credentials"
        return True, data

    wait_for("agent credentials", credentials_ready)
    credentials = read_credentials()
    app_id = credentials["app_id"]
    app_key = credentials["api_key"]
    wait_for(
        "agent websocket",
        lambda: (f'levier_ws_connections{{app="{app_id}"}} 1' in metrics(), ""),
    )

    wait_for(
        "mcp runtime socket",
        lambda: (os.path.exists(MCP_SOCKET), MCP_SOCKET),
    )
    def mcp_ok():
        try:
            result = mcp_health()
        except (OSError, ValueError, RuntimeError) as exc:
            return False, str(exc)
        return result.get("status") == "ok", result

    wait_for("mcp runtime health", mcp_ok, timeout=30)

    status, created = http("POST", "/api/v1/runs", {
        "runtime_name": RUNTIME_NAME,
        "params": {},
        "input": [{"role": "user", "content": "docker compose smoke"}],
        "timeouts": {"run_ms": 30000, "tool_ms": 5000},
    }, token=app_key)
    if status != 202:
        raise SystemExit(f"docker-smoke: create run HTTP {status} {created!r}")
    run_id = created["run_id"]
    log(f"run submitted: {run_id}")

    def terminal():
        code, run = http("GET", f"/api/v1/runs/{run_id}", token=app_key)
        if code != 200:
            return False, f"HTTP {code}"
        return run.get("status") in {"completed", "failed", "cancelled", "timeout"}, run.get("status")

    wait_for("run terminal", terminal, timeout=60)
    _, run = http("GET", f"/api/v1/runs/{run_id}", token=app_key)
    if run.get("status") != "completed":
        raise SystemExit(f"docker-smoke: run ended as {run.get('status')}")

    log("run completed: ok")
    log("PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
