"""Middleware — minimal streaming HTTP transport for LLM providers.

The harness has no third-party HTTP client dependency. This transport uses
the standard library in a helper thread and exposes an async byte stream so
the provider can parse server-sent events incrementally.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import http.client
import threading
from dataclasses import dataclass, field
from typing import AsyncIterator
from urllib.parse import urlsplit


@dataclass(frozen=True)
class HTTPRequest:
    """One streaming POST request."""

    url: str
    headers: dict[str, str] = field(default_factory=dict)
    body: bytes = b""
    timeout_s: float = 120.0


@dataclass(frozen=True)
class HTTPResponse:
    """Status, headers, and a streaming response body."""

    status: int
    headers: dict[str, str]
    body: AsyncIterator[bytes]


class HTTPTransportError(RuntimeError):
    """Raised when the transport cannot complete a request."""


class HTTPTransport:
    """Transport contract used by providers and tests."""

    async def open(self, request: HTTPRequest) -> HTTPResponse:
        """Open a request and return the response stream."""
        raise NotImplementedError


class UrllibTransport(HTTPTransport):
    """stdlib HTTP/HTTPS transport backed by a helper thread."""

    async def open(self, request: HTTPRequest) -> HTTPResponse:
        parts = urlsplit(request.url)
        if parts.scheme not in ("http", "https") or not parts.hostname:
            raise HTTPTransportError(f"unsupported URL {request.url!r}")

        loop = asyncio.get_running_loop()
        queue: asyncio.Queue[tuple[str, object]] = asyncio.Queue()
        stop = threading.Event()

        def put(kind: str, value: object = None) -> None:
            loop.call_soon_threadsafe(queue.put_nowait, (kind, value))

        def worker() -> None:
            connection = None
            try:
                cls = (
                    http.client.HTTPSConnection
                    if parts.scheme == "https"
                    else http.client.HTTPConnection
                )
                connection = cls(parts.hostname, parts.port, timeout=request.timeout_s)
                path = parts.path or "/"
                if parts.query:
                    path = f"{path}?{parts.query}"
                connection.request("POST", path, body=request.body, headers=request.headers)
                response = connection.getresponse()
                headers = {key.lower(): value for key, value in response.getheaders()}
                put("status", (response.status, headers))
                while not stop.is_set():
                    chunk = response.read1(65536)
                    if not chunk:
                        break
                    put("chunk", chunk)
                put("done")
            except Exception as exc:  # transport failures are normalized by the provider
                put("error", exc)
            finally:
                if connection is not None:
                    connection.close()

        thread = threading.Thread(target=worker, name="llm-http", daemon=True)
        thread.start()

        kind, value = await queue.get()
        if kind == "error":
            raise HTTPTransportError(str(value))
        if kind != "status":
            raise HTTPTransportError("transport did not return a response status")
        status, headers = value
        return HTTPResponse(
            status=status,
            headers=headers,
            body=self._body(queue, stop),
        )

    async def _body(
        self,
        queue: asyncio.Queue[tuple[str, object]],
        stop: threading.Event,
    ) -> AsyncIterator[bytes]:
        try:
            while True:
                kind, value = await queue.get()
                if kind == "chunk":
                    yield value  # type: ignore[misc]
                elif kind == "error":
                    raise HTTPTransportError(str(value))
                elif kind == "done":
                    break
        finally:
            stop.set()
