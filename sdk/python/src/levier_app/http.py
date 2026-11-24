"""Small standard-library HTTP client for the Levier App API.

The adapter owns auth headers, JSON envelopes, and response validation so an
App only deals with its own tools and business functions.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Protocol, Sequence

from .credentials import Secret
from .errors import HTTPError, TransportError


@dataclass(frozen=True)
class HttpRequest:
    method: str
    url: str
    headers: dict[str, str]
    body: bytes = b""


@dataclass(frozen=True)
class HttpResponse:
    status: int
    headers: dict[str, str]
    body: bytes


class Transport(Protocol):
    """Injected HTTP transport; useful for deterministic tests."""

    def send(self, request: HttpRequest) -> HttpResponse:
        """Send one request and return one response."""


class UrllibTransport:
    """Standard-library HTTP transport."""

    def __init__(self, timeout: float = 15.0) -> None:
        self.timeout = timeout

    def send(self, request: HttpRequest) -> HttpResponse:
        outgoing = urllib.request.Request(
            request.url,
            data=request.body or None,
            headers=request.headers,
            method=request.method,
        )
        try:
            with urllib.request.urlopen(outgoing, timeout=self.timeout) as response:
                return HttpResponse(
                    status=response.status,
                    headers=dict(response.headers.items()),
                    body=response.read(),
                )
        except urllib.error.HTTPError as exc:
            return HttpResponse(
                status=exc.code,
                headers=dict(exc.headers.items()),
                body=exc.read(),
            )
        except urllib.error.URLError as exc:
            raise TransportError(str(exc.reason)) from exc


@dataclass(frozen=True)
class WsToken:
    ws_url: str
    token: Secret = field(repr=False)
    expires_in: int = 0


def _decode_json(response: HttpResponse) -> dict[str, Any]:
    try:
        payload = json.loads(response.body.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise HTTPError(response.status, "BAD_RESPONSE", f"invalid JSON: {exc}") from exc
    if not isinstance(payload, dict):
        raise HTTPError(response.status, "BAD_RESPONSE", "JSON body must be an object")
    return payload


def _error_from_response(response: HttpResponse) -> HTTPError:
    try:
        payload = json.loads(response.body.decode("utf-8"))
        error = payload.get("error") or {}
        return HTTPError(
            status=response.status,
            code=str(error.get("code") or "HTTP_ERROR"),
            message=str(error.get("message") or "request failed"),
            retryable=bool(error.get("retryable")),
        )
    except (json.JSONDecodeError, UnicodeDecodeError, AttributeError):
        return HTTPError(response.status, "HTTP_ERROR", "request failed")


def _require_ok(response: HttpResponse, expected: int | tuple[int, ...]) -> dict[str, Any]:
    statuses = (expected,) if isinstance(expected, int) else expected
    if response.status not in statuses:
        raise _error_from_response(response)
    return _decode_json(response)


class LevierClient:
    """App-scoped HTTP API client."""

    def __init__(
        self,
        server: str,
        app_id: str,
        api_key: str,
        transport: Transport | None = None,
        timeout: float = 15.0,
    ) -> None:
        self.server = server.rstrip("/")
        self.app_id = app_id
        self._api_key = api_key
        self.transport = transport or UrllibTransport(timeout)

    def _request(
        self,
        method: str,
        path: str,
        *,
        body: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> HttpResponse:
        request_headers = {
            "Authorization": "Bearer " + self._api_key,
            "Accept": "application/json",
        }
        if body is not None:
            request_headers["Content-Type"] = "application/json"
        if headers:
            request_headers.update(headers)
        payload = b"" if body is None else json.dumps(body, separators=(",", ":")).encode("utf-8")
        return self.transport.send(HttpRequest(
            method=method,
            url=self.server + path,
            headers=request_headers,
            body=payload,
        ))

    def get_app(self) -> dict[str, Any]:
        response = self._request("GET", f"/api/v1/apps/{urllib.parse.quote(self.app_id, safe='')}")
        return _require_ok(response, 200)

    @classmethod
    def from_credentials(
        cls,
        path: str | None = None,
        *,
        server: str | None = None,
        transport: Transport | None = None,
        timeout: float = 15.0,
    ) -> "LevierClient":
        from .credentials import load_credentials, resolve_server

        credentials = load_credentials(path)
        return cls(
            resolve_server(credentials, server),
            credentials.app_id,
            credentials.api_key.reveal(),
            transport,
            timeout,
        )

    def update_tools(
        self,
        app_name: str,
        tools: list[dict[str, Any]],
        *,
        mcp_refs: Sequence[str] | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {
            "name": app_name,
            "multiagent": False,
            "tools": tools,
        }
        if mcp_refs is not None:
            body["mcp_refs"] = list(mcp_refs)
        response = self._request("POST", "/api/v1/apps", body=body)
        return _require_ok(response, 200)

    def list_model_providers(self) -> list[dict[str, Any]]:
        response = self._request("GET", "/api/v1/model-providers")
        payload = _require_ok(response, 200)
        providers = payload.get("providers")
        if not isinstance(providers, list):
            raise HTTPError(response.status, "BAD_RESPONSE", "providers must be a list")
        return [provider for provider in providers if isinstance(provider, dict)]

    def create_ws_token(self) -> WsToken:
        response = self._request("POST", "/api/v1/ws-token")
        payload = _require_ok(response, 200)
        ws_url = payload.get("ws_url")
        token = payload.get("ws_token")
        expires_in = payload.get("expires_in")
        if not isinstance(ws_url, str) or not ws_url:
            raise HTTPError(response.status, "BAD_RESPONSE", "ws_url is required")
        if not isinstance(token, str) or not token:
            raise HTTPError(response.status, "BAD_RESPONSE", "ws_token is required")
        if not isinstance(expires_in, int) or expires_in <= 0:
            raise HTTPError(response.status, "BAD_RESPONSE", "expires_in must be positive")
        return WsToken(ws_url=ws_url, token=Secret(token), expires_in=expires_in)

    def create_run(
        self,
        payload: dict[str, Any],
        *,
        idempotency_key: str = "",
    ) -> dict[str, Any]:
        headers = {"Idempotency-Key": idempotency_key} if idempotency_key else None
        response = self._request("POST", "/api/v1/runs", body=payload, headers=headers)
        return _require_ok(response, 202)

    def submit_run(
        self,
        prompt: str,
        *,
        runtime: str,
        runtime_type: str = "",
        session_id: str = "",
        provider: str = "",
        model: str = "",
        max_steps: int = 8,
        run_ms: int = 60000,
        tool_ms: int = 10000,
        idempotency_key: str = "",
    ) -> dict[str, Any]:
        """Submit one user prompt and return the queued Run view."""
        if not prompt.strip():
            raise ValueError("prompt is required")
        if not runtime.strip():
            raise ValueError("runtime is required")
        payload: dict[str, Any] = {
            "runtime_name": runtime,
            "params": {"max_steps": max_steps},
            "input": [{"role": "user", "content": prompt}],
            "timeouts": {"run_ms": run_ms, "tool_ms": tool_ms},
        }
        if runtime_type:
            payload["runtime"] = runtime_type
        if session_id:
            payload["session_id"] = session_id
        if provider or model:
            payload["model"] = {"provider": provider, "model": model}
        return self.create_run(payload, idempotency_key=idempotency_key)

    def wait_run(
        self,
        run_id: str,
        *,
        timeout: float = 300.0,
        interval: float = 0.5,
    ) -> dict[str, Any]:
        """Poll until a Run reaches a terminal status and return its view."""
        if timeout <= 0:
            raise ValueError("timeout must be positive")
        if interval <= 0:
            interval = 0.5
        deadline = time.monotonic() + timeout
        while True:
            run = self.get_run(run_id)
            status = str(run.get("status") or "")
            if status in {"completed", "failed", "cancelled", "timeout"}:
                return run
            if time.monotonic() >= deadline:
                raise TimeoutError(f"run {run_id} did not finish within {timeout}s")
            time.sleep(interval)

    def get_run(self, run_id: str) -> dict[str, Any]:
        response = self._request("GET", f"/api/v1/runs/{urllib.parse.quote(run_id, safe='')}")
        return _require_ok(response, 200)

    def get_events(
        self,
        run_id: str,
        *,
        after_attempt: int = 0,
        after_seq: int = 0,
        limit: int = 1000,
    ) -> list[dict[str, Any]]:
        payload = self.list_events(
            run_id,
            after_attempt=after_attempt,
            after_seq=after_seq,
            limit=limit,
        )
        events = payload.get("events")
        return [event for event in events if isinstance(event, dict)] if isinstance(events, list) else []

    def list_events(
        self,
        run_id: str,
        *,
        after_attempt: int = 0,
        after_seq: int = 0,
        limit: int = 1000,
    ) -> dict[str, Any]:
        query = urllib.parse.urlencode({
            "after_attempt": after_attempt,
            "after_seq": after_seq,
            "limit": limit,
        })
        response = self._request(
            "GET",
            f"/api/v1/runs/{urllib.parse.quote(run_id, safe='')}/events?{query}",
        )
        return _require_ok(response, 200)

    def cancel_run(self, run_id: str) -> dict[str, Any]:
        response = self._request(
            "POST",
            f"/api/v1/runs/{urllib.parse.quote(run_id, safe='')}/cancel",
        )
        return _require_ok(response, 200)
