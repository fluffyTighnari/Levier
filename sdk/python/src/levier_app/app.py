"""The reusable Levier App adapter.

The App owns tool handlers and business state. This adapter owns credentials,
HTTP calls, tool registration, WebSocket lifecycle, reconnects, and the
local_tool_call/local_tool_result contract.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import logging
import os
import signal
import socket
import threading
import time
from dataclasses import dataclass, field
from typing import Any, Callable, Mapping, Sequence

from .credentials import Credentials, load_credentials, resolve_server
from .errors import (
    AppInactiveError,
    HTTPError,
    ProtocolError,
    ToolExecutionError,
    TransportError,
)
from .http import LevierClient, Transport
from .websocket import WebSocketClient, websocket_url

SDK_VERSION = "0.1.0"
PROTOCOL_VERSION = 1

Handler = Callable[[dict[str, Any]], Any]
EventCallback = Callable[[dict[str, Any]], None]


def hello_message() -> dict[str, Any]:
    """Return the v1 adapter handshake sent immediately after connect."""
    return {
        "type": "hello",
        "protocol": PROTOCOL_VERSION,
        "client": "levier-app-python",
        "version": SDK_VERSION,
    }


@dataclass(frozen=True)
class ToolSpec:
    """One App-local tool and its JSON schema."""

    name: str
    handler: Handler
    description: str = ""
    schema: dict[str, Any] = field(
        default_factory=lambda: {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        }
    )
    timeout_ms: int = 10000

    def manifest(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "description": self.description,
            "schema": self.schema,
            "timeout_ms": self.timeout_ms,
        }


class LevierApp:
    """One App connected to Levier through the Python adapter."""

    def __init__(
        self,
        credentials: Credentials,
        *,
        server: str | None = None,
        transport: Transport | None = None,
        logger: logging.Logger | None = None,
        on_event: EventCallback | None = None,
        mcp_refs: Sequence[str] | None = None,
    ) -> None:
        self.credentials = credentials
        self.server = resolve_server(credentials, server)
        self.client = LevierClient(
            self.server,
            credentials.app_id,
            credentials.api_key.reveal(),
            transport,
        )
        self.logger = logger or logging.getLogger("levier_app")
        self.on_event = on_event
        self.mcp_refs = _parse_mcp_refs(mcp_refs)
        self._tools: dict[str, ToolSpec] = {}
        self._stop_requested = False
        self._active_ws: WebSocketClient | None = None

    @classmethod
    def from_credentials(
        cls,
        path: str | None = None,
        *,
        server: str | None = None,
        transport: Transport | None = None,
        logger: logging.Logger | None = None,
        on_event: EventCallback | None = None,
        mcp_refs: Sequence[str] | None = None,
    ) -> "LevierApp":
        return cls(
            load_credentials(path),
            server=server,
            transport=transport,
            logger=logger,
            on_event=on_event,
            mcp_refs=mcp_refs,
        )

    def tool(
        self,
        name: str,
        *,
        description: str = "",
        schema: Mapping[str, Any] | None = None,
        timeout_ms: int = 10000,
    ) -> Callable[[Handler], Handler]:
        """Register a handler invoked as handler(arguments: dict) -> JSON value."""
        normalized = name.strip()
        if not normalized:
            raise ValueError("tool name is required")
        if normalized in self._tools:
            raise ValueError(f"tool already registered: {normalized}")

        tool_schema = dict(schema) if schema is not None else {
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        }
        if timeout_ms <= 0:
            raise ValueError("tool timeout_ms must be positive")

        def decorator(handler: Handler) -> Handler:
            self._tools[normalized] = ToolSpec(
                name=normalized,
                handler=handler,
                description=description,
                schema=tool_schema,
                timeout_ms=timeout_ms,
            )
            return handler

        return decorator

    def tools_manifest(self) -> list[dict[str, Any]]:
        return [spec.manifest() for spec in self._tools.values()]

    def stop(self) -> None:
        """Request a graceful stop; safe to call from a signal handler."""
        self._stop_requested = True
        active = self._active_ws
        if active is not None:
            active.close()

    @classmethod
    def run_cli(
        cls,
        register: "Callable[[LevierApp], None] | None" = None,
        *,
        argv: list[str] | None = None,
        prog: str = "levier-app",
    ) -> int:
        """Run the standard App CLI; the only user hook is tool registration."""
        from .cli import run_app

        return run_app(register, argv=argv, prog=prog)

    def run(
        self,
        *,
        max_calls: int = 0,
        idle_timeout: float = 0.0,
        ping_interval: float = 20.0,
        reconnect_initial: float = 0.5,
        reconnect_max: float = 30.0,
        handle_signals: bool = True,
    ) -> int:
        """Serve local tool calls until stopped or max_calls is reached."""
        if not self._tools:
            self.logger.warning("no local tools registered")

        old_handlers: list[tuple[int, Any]] = []
        if handle_signals and threading.current_thread() is threading.main_thread():
            for signum in (signal.SIGINT, signal.SIGTERM):
                old_handlers.append((signum, signal.getsignal(signum)))
                signal.signal(signum, lambda _signum, _frame: self.stop())

        ws: WebSocketClient | None = None
        calls = 0
        last_activity = time.monotonic()
        backoff = reconnect_initial
        try:
            while not self._stop_requested:
                if ws is None:
                    try:
                        ws = self._connect()
                        self._active_ws = ws
                        last_activity = time.monotonic()
                        backoff = reconnect_initial
                    except HTTPError as exc:
                        if exc.status in (400, 401, 403, 404, 409):
                            self.logger.error("app setup rejected: %s", exc)
                            return 1
                        self.logger.warning("connect failed: %s", exc)
                        if self._sleep_backoff(backoff):
                            return 0
                        backoff = min(backoff * 2, reconnect_max)
                        continue
                    except AppInactiveError as exc:
                        self.logger.error("app is not active: %s", exc)
                        return 1
                    except (TransportError, ProtocolError, OSError) as exc:
                        self.logger.warning("connect failed: %s", exc)
                        if self._sleep_backoff(backoff):
                            return 0
                        backoff = min(backoff * 2, reconnect_max)
                        continue

                timeout = ping_interval if ping_interval > 0 else None
                if idle_timeout > 0:
                    timeout = min(timeout or idle_timeout, idle_timeout)
                try:
                    message = ws.recv_json(timeout=timeout)
                except socket.timeout:
                    if idle_timeout > 0 and time.monotonic() - last_activity >= idle_timeout:
                        return 0
                    if ping_interval > 0:
                        try:
                            ws.send_json({"type": "ping"})
                        except OSError:
                            ws.close()
                            ws = None
                            self._active_ws = None
                    continue
                except (OSError, ProtocolError) as exc:
                    self.logger.warning("websocket lost: %s", exc)
                    ws.close()
                    ws = None
                    self._active_ws = None
                    if not self._stop_requested and self._sleep_backoff(backoff):
                        return 0
                    backoff = min(backoff * 2, reconnect_max)
                    continue

                last_activity = time.monotonic()
                try:
                    handled = self._handle_message(ws, message)
                except (OSError, ProtocolError) as exc:
                    self.logger.warning("websocket write failed: %s", exc)
                    ws.close()
                    ws = None
                    self._active_ws = None
                    if not self._stop_requested and self._sleep_backoff(backoff):
                        return 0
                    backoff = min(backoff * 2, reconnect_max)
                    continue
                if handled:
                    calls += 1
                    if max_calls > 0 and calls >= max_calls:
                        return 0
            return 0
        finally:
            if ws is not None:
                ws.close()
            self._active_ws = None
            for signum, previous in old_handlers:
                signal.signal(signum, previous)

    def _connect(self) -> WebSocketClient:
        app = self.client.get_app()
        status = str(app.get("status") or "")
        if status != "active":
            raise AppInactiveError(f"app status is {status!r}, want active")
        self.client.update_tools(
            self.credentials.app_name,
            self.tools_manifest(),
            mcp_refs=self.mcp_refs,
        )
        token = self.client.create_ws_token()
        url = websocket_url(
            self.server,
            token.ws_url,
            self.credentials.app_id,
            token.token.reveal(),
        )
        ws = WebSocketClient.connect(url)
        try:
            ws.send_json(hello_message())
        except Exception:
            ws.close()
            raise
        self.logger.info("connected", extra={"app_id": self.credentials.app_id})
        return ws

    def _handle_message(self, ws: WebSocketClient, message: dict[str, Any]) -> bool:
        kind = message.get("type")
        if kind == "local_tool_call":
            result = self._dispatch_local_tool(message)
            if result is not None:
                ws.send_json(result)
            return result is not None
        if kind == "ping":
            ws.send_json({"type": "pong"})
            return False
        if kind == "pong":
            return False
        if self.on_event is not None:
            try:
                self.on_event(message)
            except Exception:
                self.logger.exception("event callback failed")
        return False

    def _dispatch_local_tool(self, message: dict[str, Any]) -> dict[str, Any] | None:
        correlation_id = str(message.get("msg_id") or "")
        invocation_id = str(message.get("invocation_id") or "")
        if not correlation_id or not invocation_id:
            self.logger.warning("ignored local_tool_call without correlation ids")
            return None
        name = str(message.get("tool_name") or "")
        arguments_json = message.get("arguments_json")
        try:
            arguments = _parse_arguments(arguments_json)
        except (TypeError, ValueError) as exc:
            return _tool_error(correlation_id, invocation_id, "TOOL_EXEC_ERROR", str(exc))

        spec = self._tools.get(name)
        if spec is None:
            return _tool_error(
                correlation_id, invocation_id, "TOOL_NOT_FOUND", f"unknown tool {name}"
            )
        try:
            result = spec.handler(arguments)
            result_json = json.dumps(result, separators=(",", ":"), allow_nan=False)
        except ToolExecutionError as exc:
            return _tool_error(correlation_id, invocation_id, exc.code, exc.message, exc.retryable)
        except Exception as exc:  # Handler failures must become tool results.
            self.logger.error(
                "local tool failed",
                extra={
                    "tool": name,
                    "error_code": "TOOL_EXEC_ERROR",
                    "error_type": type(exc).__name__,
                    "correlation_id": correlation_id,
                    "invocation_id": invocation_id,
                },
            )
            return _tool_error(
                correlation_id, invocation_id, "TOOL_EXEC_ERROR", str(exc)
            )
        return {
            "type": "local_tool_result",
            "correlation_id": correlation_id,
            "invocation_id": invocation_id,
            "ok": True,
            "result_json": result_json,
        }

    def _sleep_backoff(self, seconds: float) -> bool:
        deadline = time.monotonic() + seconds
        while not self._stop_requested and time.monotonic() < deadline:
            time.sleep(min(0.1, deadline - time.monotonic()))
        return self._stop_requested


def _parse_arguments(value: Any) -> dict[str, Any]:
    if value is None or value == "":
        return {}
    if not isinstance(value, str):
        raise TypeError("arguments_json must be a string")
    parsed = json.loads(value)
    if not isinstance(parsed, dict):
        raise TypeError("arguments_json must be a JSON object")
    return parsed


def _tool_error(
    correlation_id: str,
    invocation_id: str,
    code: str,
    message: str,
    retryable: bool = False,
) -> dict[str, Any]:
    return {
        "type": "local_tool_result",
        "correlation_id": correlation_id,
        "invocation_id": invocation_id,
        "ok": False,
        "error": {
            "code": code,
            "message": message,
            "retryable": retryable,
        },
    }


def _parse_mcp_refs(value: Sequence[str] | None) -> list[str] | None:
    if value is None:
        raw = os.environ.get("LEVIER_APP_MCP_REFS", "")
        if not raw.strip():
            return None
        value = raw.split(",")
    refs: list[str] = []
    for item in value:
        name = str(item).strip()
        if name and name not in refs:
            refs.append(name)
    return refs
