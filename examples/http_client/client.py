"""Levier — zero-dependency HTTP client smoke test.

Uses only the Python standard library and no generated Levier code. It
verifies the App/admin REST contract from a non-Go process.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from typing import Any

DEFAULT_URL = "http://127.0.0.1:8080"
DEFAULT_ADMIN_TOKEN = "admin-change-me"
DEFAULT_APP_NAME = "python-http-client"
REQUEST_TIMEOUT_SECONDS = 15


def request(
    method: str,
    url: str,
    *,
    token: str | None = None,
    body: dict[str, Any] | None = None,
    expected: int | tuple[int, ...] = 200,
) -> tuple[int, dict[str, Any], dict[str, str]]:
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=REQUEST_TIMEOUT_SECONDS) as response:
            status = response.status
            raw = response.read().decode("utf-8")
            payload = json.loads(raw) if raw else {}
            response_headers = dict(response.headers.items())
    except urllib.error.HTTPError as exc:
        status = exc.code
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {"raw": raw}
        response_headers = dict(exc.headers.items())
    expected_statuses = (expected,) if isinstance(expected, int) else expected
    if status not in expected_statuses:
        raise RuntimeError(
            f"{method} {url} -> {status}, want {expected_statuses}: "
            f"{json.dumps(payload, ensure_ascii=False)[:800]}"
        )
    return status, payload, response_headers


def header(headers: dict[str, str], name: str) -> str | None:
    lower = name.lower()
    for key, value in headers.items():
        if key.lower() == lower:
            return value
    return None


def bootstrap_app(base_url: str, admin_token: str, app_name: str) -> tuple[str, str]:
    _, payload, _ = request(
        "POST",
        base_url + "/admin/apps",
        token=admin_token,
        body={"name": app_name},
        expected=(200, 201),
    )
    app_id = payload["app"]["app_id"]
    api_key = payload.get("api_key") or ""
    if not api_key:
        _, rotated, _ = request(
            "POST",
            base_url + f"/admin/apps/{app_id}/rotate-key",
            token=admin_token,
            expected=200,
        )
        api_key = rotated["api_key"]
    if not api_key:
        raise RuntimeError("admin bootstrap did not return an App API key")
    return app_id, api_key


def assert_error_envelope(payload: dict[str, Any], code: str) -> None:
    error = payload.get("error") or {}
    if error.get("code") != code:
        raise RuntimeError(f"error code = {error.get('code')!r}, want {code!r}")
    if not error.get("request_id"):
        raise RuntimeError("error envelope is missing request_id")


def main() -> int:
    base_url = (os.environ.get("LEVIER_URL") or os.environ.get("MW_URL") or DEFAULT_URL).rstrip("/")
    admin_token = os.environ.get("LEVIER_ADMIN_TOKEN") or os.environ.get("MW_ADMIN_TOKEN") or DEFAULT_ADMIN_TOKEN
    app_name = os.environ.get("LEVIER_APP_NAME") or os.environ.get("MW_APP_NAME") or DEFAULT_APP_NAME

    _, health, health_headers = request("GET", base_url + "/healthz", expected=200)
    print(f"health: {health.get('status')} request_id={header(health_headers, 'X-Request-ID')}")

    app_id, api_key = bootstrap_app(base_url, admin_token, app_name)
    print(f"app: {app_id}")

    _, ws_token, _ = request(
        "POST",
        base_url + "/api/v1/ws-token",
        token=api_key,
        expected=200,
    )
    print(f"ws_token: url={ws_token.get('ws_url')} expires_in={ws_token.get('expires_in')}")

    _, created, created_headers = request(
        "POST",
        base_url + "/api/v1/runs",
        token=api_key,
        body={
            "runtime": "normal",
            "strategy": {"name": "echo"},
            "input": [{"role": "user", "content": "python stdlib http client"}],
            "model": {"provider": "anthropic", "model": "claude-test"},
            "timeouts": {"run_ms": 30000, "tool_ms": 5000},
        },
        expected=202,
    )
    run_id = created["run_id"]
    print(
        f"run: {run_id} status={created.get('status')} "
        f"request_id={header(created_headers, 'X-Request-ID')}"
    )

    _, run, _ = request(
        "GET",
        base_url + f"/api/v1/runs/{run_id}",
        token=api_key,
        expected=200,
    )
    print(f"run_status: {run.get('status')} attempt={run.get('attempt')}")

    _, events, _ = request(
        "GET",
        base_url + f"/api/v1/runs/{run_id}/events?limit=100",
        token=api_key,
        expected=200,
    )
    print(f"events: count={len(events.get('events', []))} caught_up={events.get('caught_up')}")

    _, missing, _ = request(
        "GET",
        base_url + "/api/v1/sessions/does-not-exist",
        token=api_key,
        expected=404,
    )
    assert_error_envelope(missing, "SESSION_NOT_FOUND")
    print("error_envelope: ok")

    request(
        "POST",
        base_url + f"/api/v1/runs/{run_id}/cancel",
        token=api_key,
        body={"reason": "python client smoke"},
        expected=200,
    )
    print("cancel: ok")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 - CLI must surface one clear error.
        print(f"smoke failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
