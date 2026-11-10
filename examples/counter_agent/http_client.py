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
from dataclasses import dataclass
from typing import Protocol

from config import AppConfig


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
