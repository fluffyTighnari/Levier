"""The one-import App surface.

An App author writes ordinary functions, decorates them with ``@tool``, and
hands them to ``App``. Credentials, the server address, MCP references, the
WebSocket lifecycle, CLI flags, reconnects, and tool-call results stay inside
the SDK.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import logging
import sys
import threading
from typing import Any, Callable, Iterable, Sequence

from levier_app import LevierApp

from .tools import Tool, tool as tool_decorator


class App:
    """A small facade over ``LevierApp`` for the common case."""

    def __init__(
        self,
        tools: Iterable[Tool | Callable[..., Any]] = (),
        *,
        mcp: str | Sequence[str] | None = None,
        runtime: str | None = None,
        credentials: str | None = None,
        server: str | None = None,
        logger: logging.Logger | None = None,
        on_event: Callable[[dict[str, Any]], None] | None = None,
    ) -> None:
        refs = _mcp_refs(mcp)
        self._app = LevierApp.from_credentials(
            credentials,
            server=server,
            logger=logger,
            on_event=on_event,
            mcp_refs=refs,
        )
        self.runtime = runtime or _default_runtime()
        self.logger = self._app.logger
        self._serve_thread: threading.Thread | None = None
        self._tools: list[Tool] = []
        for item in tools:
            self.add(item)

    @property
    def app_id(self) -> str:
        return self._app.credentials.app_id

    @property
    def app_name(self) -> str:
        return self._app.credentials.app_name

    def add(self, item: Tool | Callable[..., Any]) -> Tool:
        """Add a decorated tool, or decorate a plain function on the spot."""
        value = item if isinstance(item, Tool) else tool_decorator(item)
        self._tools.append(value)
        self._app.tool(
            value.name,
            description=value.description,
            schema=dict(value.schema),
            timeout_ms=value.timeout_ms,
        )(value.invoke)
        return value

    def tool(
        self,
        function: Tool | Callable[..., Any] | str | None = None,
        *,
        name: str | None = None,
        description: str | None = None,
        schema: dict[str, Any] | None = None,
        timeout_ms: int = 10_000,
    ) -> Tool | Callable[[Callable[..., Any]], Tool]:
        """Decorator form of :meth:`add`.

        Supports ``@app.tool``, ``@app.tool("name")`` and
        ``@app.tool(name="name", description=...)``.
        """

        def decorate(function: Callable[..., Any], explicit_name: str | None = None) -> Tool:
            value = tool_decorator(
                function,
                name=explicit_name or name,
                description=description,
                schema=schema,
                timeout_ms=timeout_ms,
            )
            assert isinstance(value, Tool)
            return self.add(value)

        if callable(function):
            return decorate(function)
        if isinstance(function, str):
            return lambda fn: decorate(fn, function)
        if function is not None:
            raise TypeError("app.tool() expects a function, a name, or keyword options")
        return decorate

    def serve(self) -> int:
        """Connect and serve tool calls until signal or ``app.stop()``."""
        return self._app.run()

    def stop(self) -> None:
        """Stop a running :meth:`serve` or one-shot :meth:`run`."""
        self._app.stop()

    def run(
        self,
        prompt: str | None = None,
        *,
        runtime: str | None = None,
        runtime_type: str = "",
        session_id: str = "",
        provider: str = "",
        model: str = "",
        max_steps: int = 8,
        run_ms: int = 60_000,
        tool_ms: int = 10_000,
        timeout: float = 300.0,
        print_output: bool = True,
    ) -> int | dict[str, Any]:
        """Serve when called without a prompt; run once when given one.

        ``app.run("Add 2 and 3")`` starts the App connection in the background,
        submits the Run, waits for the terminal state, prints the final
        assistant message, and closes the connection again.
        """
        if prompt is None:
            return self.serve()

        self._start_serving()
        try:
            submitted = self._app.submit_run(
                prompt,
                runtime=runtime or self.runtime,
                runtime_type=runtime_type,
                session_id=session_id,
                provider=provider,
                model=model,
                max_steps=max_steps,
                run_ms=run_ms,
                tool_ms=tool_ms,
            )
            result = self._app.wait_run(submitted["run_id"], timeout=timeout)
            final = _final_message(self._app.get_events(submitted["run_id"]))
            if print_output:
                if final:
                    print(final)
                elif result.get("status") != "completed":
                    print(f"run {result.get('status', 'failed')}", file=sys.stderr)
            if result.get("status") == "completed":
                return result
            return 1
        finally:
            self._stop_serving()

    def list_model_providers(self) -> list[dict[str, Any]]:
        """Return the model catalog visible to this App."""
        return self._app.list_model_providers()

    def tools_manifest(self) -> list[dict[str, Any]]:
        """Return the registered tool metadata."""
        return self._app.tools_manifest()

    def submit_run(self, prompt: str, *, runtime: str | None = None, **options: Any) -> dict[str, Any]:
        """Submit a Run without blocking."""
        return self._app.submit_run(prompt, runtime=runtime or self.runtime, **options)

    def wait_run(self, run_id: str, *, timeout: float = 300.0) -> dict[str, Any]:
        return self._app.wait_run(run_id, timeout=timeout)

    def get_run(self, run_id: str) -> dict[str, Any]:
        return self._app.get_run(run_id)

    def get_events(self, run_id: str) -> list[dict[str, Any]]:
        return self._app.get_events(run_id)

    def cancel_run(self, run_id: str) -> dict[str, Any]:
        return self._app.cancel_run(run_id)

    def resume_run(self, run_id: str, resume: Any = None) -> dict[str, Any]:
        return self._app.resume_run(run_id, resume)

    def _start_serving(self) -> None:
        if self._serve_thread is not None and self._serve_thread.is_alive():
            return

        def serve() -> None:
            try:
                self._app.run(handle_signals=False)
            except Exception:  # the caller already owns the failure path
                self.logger.exception("app connection stopped")

        self._serve_thread = threading.Thread(target=serve, name="levier-app", daemon=True)
        self._serve_thread.start()
        if not self._app.wait_connected(timeout=15.0):
            self._stop_serving()
            raise RuntimeError("App could not connect to Levier; run `levier status` and retry")

    def _stop_serving(self) -> None:
        self._app.stop()
        thread = self._serve_thread
        self._serve_thread = None
        if thread is not None:
            thread.join(timeout=5.0)


def _mcp_refs(value: str | Sequence[str] | None) -> list[str] | None:
    if value is None:
        return None
    if isinstance(value, str):
        items = value.split(",")
    else:
        items = list(value)
    refs = [str(item).strip() for item in items if str(item).strip()]
    return refs or None


def _default_runtime() -> str:
    from levier_app.credentials import project_values, profile_values

    value = project_values().get("runtime", "")
    if isinstance(value, str) and value:
        return value
    return profile_values().get("runtime", "") or "react1"


def _final_message(events: Sequence[dict[str, Any]]) -> str:
    final = ""
    for event in events:
        if event.get("event_type") != "MESSAGE_COMPLETED":
            continue
        content = (event.get("data") or {}).get("content")
        if isinstance(content, str):
            final = content
    return final


__all__ = ["App", "Tool", "tool_decorator"]
