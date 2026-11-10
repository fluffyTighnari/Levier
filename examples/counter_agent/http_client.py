"""Middleware — Counter App HTTP request construction (P2.2).

Builds the `/admin/apps` request and sends it through an injected transport.
No real network client is used in this step.

Design references:
- docs/DESIGN.md §4.2: Admin App bootstrap.
- docs/DESIGN.md §13.1: Admin token authentication.
- internal/httpapi/server.go: POST /admin/apps.
- internal/httpapi/contracts.go: AppUpsertRequest.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Protocol

from config import AppConfig, Secret


@dataclass(frozen=True)
class HttpRequest:
    """One HTTP request, independent of the transport."""

    method: str
    url: str
    headers: dict[str, str]
    body: bytes

    def json_body(self) -> dict:
        """Decode the request body for assertions and debugging."""
        return json.loads(self.body.decode("utf-8"))


@dataclass(frozen=True)
class HttpResponse:
    """One HTTP response, independent of the transport."""

    status: int
    headers: dict[str, str]
    body: bytes


class Transport(Protocol):
    """Injected HTTP transport."""

    def send(self, request: HttpRequest) -> HttpResponse:
        """Send one request and return one response."""


class HttpError(RuntimeError):
    """One non-success HTTP response, normalized."""

    def __init__(
        self,
        status: int,
        code: str,
        message: str,
        retryable: bool = False,
    ) -> None:
        super().__init__(f"HTTP {status}: {code}: {message}")
        self.status = status
        self.code = code
        self.message = message
        self.retryable = retryable


@dataclass(frozen=True)
class RunAccepted:
    """Parsed POST /api/v1/runs response."""

    run_id: str
    status: str
    ws_url: str
    ws_token: Secret


@dataclass(frozen=True)
class AppView:
    """Parsed GET /api/v1/apps/{app_id} response."""

    app_id: str
    name: str
    status: str


@dataclass(frozen=True)
class AppRegistration:
    """Parsed POST /admin/apps response."""

    app_id: str
    name: str
    api_key: Secret

    @property
    def has_api_key(self) -> bool:
        """Whether this response carried a one-time API key."""
        return bool(self.api_key.reveal())


def _error_from_response(response: HttpResponse) -> HttpError:
    try:
        payload = json.loads(response.body.decode("utf-8"))
        error = payload.get("error") or {}
        return HttpError(
            status=response.status,
            code=str(error.get("code") or "HTTP_ERROR"),
            message=str(error.get("message") or "request failed"),
            retryable=bool(error.get("retryable")),
        )
    except (json.JSONDecodeError, UnicodeDecodeError, AttributeError):
        return HttpError(response.status, "HTTP_ERROR", "request failed")


def parse_create_app_response(response: HttpResponse) -> AppRegistration:
    """Parse a 200/201 POST /admin/apps response."""
    if response.status not in (200, 201):
        raise _error_from_response(response)
    try:
        payload = json.loads(response.body.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise HttpError(response.status, "BAD_RESPONSE", f"invalid JSON: {exc}") from exc
    app = payload.get("app")
    if not isinstance(app, dict):
        raise HttpError(response.status, "BAD_RESPONSE", "app is required")
    app_id = app.get("app_id")
    if not isinstance(app_id, str) or not app_id:
        raise HttpError(response.status, "BAD_RESPONSE", "app.app_id is required")
    api_key = payload.get("api_key", "")
    if not isinstance(api_key, str):
        raise HttpError(response.status, "BAD_RESPONSE", "api_key must be a string")
    name = app.get("name", "")
    if not isinstance(name, str):
        raise HttpError(response.status, "BAD_RESPONSE", "app.name must be a string")
    return AppRegistration(app_id=app_id, name=name, api_key=Secret(api_key))


class HttpTransportError(RuntimeError):
    """Network/transport failure before an HTTP response was received."""


class UrllibTransport:
    """Stdlib HTTP transport for real Middleware calls."""

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
            raise HttpTransportError(str(exc.reason)) from exc


def parse_rotate_key_response(response: HttpResponse) -> Secret:
    """Parse POST /admin/apps/{id}/rotate-key."""
    if response.status != 200:
        raise _error_from_response(response)
    try:
        payload = json.loads(response.body.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise HttpError(response.status, "BAD_RESPONSE", f"invalid JSON: {exc}") from exc
    api_key = payload.get("api_key")
    if not isinstance(api_key, str) or not api_key:
        raise HttpError(response.status, "BAD_RESPONSE", "api_key is required")
    return Secret(api_key)


def parse_app_view(response: HttpResponse) -> AppView:
    """Parse GET /api/v1/apps/{app_id}."""
    if response.status != 200:
        raise _error_from_response(response)
    try:
        payload = json.loads(response.body.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise HttpError(response.status, "BAD_RESPONSE", f"invalid JSON: {exc}") from exc
    app_id = payload.get("app_id")
    name = payload.get("name")
    status = payload.get("status")
    if not isinstance(app_id, str) or not app_id:
        raise HttpError(response.status, "BAD_RESPONSE", "app_id is required")
    if not isinstance(name, str) or not name:
        raise HttpError(response.status, "BAD_RESPONSE", "name is required")
    if not isinstance(status, str) or not status:
        raise HttpError(response.status, "BAD_RESPONSE", "status is required")
    return AppView(app_id=app_id, name=name, status=status)


class HttpClient:
    """Builds Admin HTTP requests and delegates transport I/O."""

    def __init__(self, config: AppConfig, transport: Transport) -> None:
        self.config = config
        self.transport = transport

    def create_app(self) -> HttpResponse:
        """POST /admin/apps for the configured App name."""
        payload = {
            "name": self.config.app_name,
            "multiagent": False,
        }
        request = HttpRequest(
            method="POST",
            url=self.config.mw_url + "/admin/apps",
            headers={
                "Authorization": "Bearer " + self.config.admin_token.reveal(),
                "Content-Type": "application/json",
                "Accept": "application/json",
            },
            body=json.dumps(payload, separators=(",", ":")).encode("utf-8"),
        )
        return self.transport.send(request)

    def rotate_app_key(self, app_id: str) -> HttpResponse:
        """POST /admin/apps/{app_id}/rotate-key."""
        request = HttpRequest(
            method="POST",
            url=self.config.mw_url + f"/admin/apps/{app_id}/rotate-key",
            headers={
                "Authorization": "Bearer " + self.config.admin_token.reveal(),
                "Accept": "application/json",
            },
            body=b"",
        )
        return self.transport.send(request)


def parse_run_response(response: HttpResponse) -> RunAccepted:
    """Parse a 202 POST /api/v1/runs response."""
    if response.status != 202:
        raise _error_from_response(response)
    try:
        payload = json.loads(response.body.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise HttpError(response.status, "BAD_RESPONSE", f"invalid JSON: {exc}") from exc
    run_id = payload.get("run_id")
    status = payload.get("status")
    ws_url = payload.get("ws_url", "")
    ws_token = payload.get("ws_token", "")
    if not isinstance(run_id, str) or not run_id:
        raise HttpError(response.status, "BAD_RESPONSE", "run_id is required")
    if not isinstance(status, str) or not status:
        raise HttpError(response.status, "BAD_RESPONSE", "status is required")
    if not isinstance(ws_url, str) or not isinstance(ws_token, str):
        raise HttpError(response.status, "BAD_RESPONSE", "ws_url/ws_token must be strings")
    return RunAccepted(run_id=run_id, status=status, ws_url=ws_url, ws_token=Secret(ws_token))


class AppClient:
    """Builds App-scoped HTTP requests using an App API key."""

    def __init__(self, mw_url: str, app_id: str, api_key: Secret, transport: Transport) -> None:
        self.mw_url = mw_url.rstrip("/")
        self.app_id = app_id
        self.api_key = api_key
        self.transport = transport

    def get_app(self) -> HttpResponse:
        """GET /api/v1/apps/{app_id} with the App key."""
        request = HttpRequest(
            method="GET",
            url=self.mw_url + f"/api/v1/apps/{self.app_id}",
            headers={
                "Authorization": "Bearer " + self.api_key.reveal(),
                "Accept": "application/json",
            },
            body=b"",
        )
        return self.transport.send(request)

    def create_run(self, payload: dict) -> HttpResponse:
        """POST /api/v1/runs with the App key."""
        request = HttpRequest(
            method="POST",
            url=self.mw_url + "/api/v1/runs",
            headers={
                "Authorization": "Bearer " + self.api_key.reveal(),
                "Content-Type": "application/json",
                "Accept": "application/json",
            },
            body=json.dumps(payload, separators=(",", ":")).encode("utf-8"),
        )
        return self.transport.send(request)

    def update_tools(self, app_name: str, tools: list[dict]) -> HttpResponse:
        """POST /api/v1/apps with the full local tool list."""
        payload = {
            "name": app_name,
            "multiagent": False,
            "tools": tools,
        }
        request = HttpRequest(
            method="POST",
            url=self.mw_url + "/api/v1/apps",
            headers={
                "Authorization": "Bearer " + self.api_key.reveal(),
                "Content-Type": "application/json",
                "Accept": "application/json",
            },
            body=json.dumps(payload, separators=(",", ":")).encode("utf-8"),
        )
        return self.transport.send(request)


class RecordingTransport:
    """In-memory transport used by tests; never opens a socket."""

    def __init__(self, response: HttpResponse | None = None) -> None:
        self.requests: list[HttpRequest] = []
        self.response = response or HttpResponse(status=200, headers={}, body=b"{}")

    def send(self, request: HttpRequest) -> HttpResponse:
        self.requests.append(request)
        return self.response
