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


class HttpClient:
    """Builds App/admin HTTP requests and delegates transport I/O."""

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


class RecordingTransport:
    """In-memory transport used by tests; never opens a socket."""

    def __init__(self, response: HttpResponse | None = None) -> None:
        self.requests: list[HttpRequest] = []
        self.response = response or HttpResponse(status=200, headers={}, body=b"{}")

    def send(self, request: HttpRequest) -> HttpResponse:
        self.requests.append(request)
        return self.response
