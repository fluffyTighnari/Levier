"""Middleware — RuntimeChannel gRPC client.

Maintains one bidirectional stream, sends Hello/heartbeats, dispatches
inbound envelopes, and reconnects with exponential backoff.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import inspect
import logging
import random
from dataclasses import dataclass, field
from typing import Awaitable, Callable, Sequence

import grpc
from middleware.runtime.v1 import runtime_pb2, runtime_pb2_grpc

from middleware_harness.channel.envelope import new_envelope, response_envelope
from middleware_harness.ids import new_ulid

MessageHandler = Callable[[runtime_pb2.Envelope], Awaitable[None] | None]
RunningRunsProvider = Callable[[], Sequence[tuple[str, int]]]
RunningCountProvider = Callable[[], int]


@dataclass
class ChannelConfig:
    """Connection and heartbeat settings."""

    address: str
    daemon_id: str
    auth_token: str
    runtime_type: str = "normal"
    max_concurrent_runs: int = 4
    strategies: list[str] = field(default_factory=lambda: ["react", "echo"])
    version: str = "0.1.0"
    labels: dict[str, str] = field(default_factory=dict)
    tls: bool = False
    heartbeat_interval: float = 10.0
    reconnect_initial: float = 1.0
    reconnect_max: float = 30.0
    hello_timeout: float = 3.0


class ChannelClient:
    """Owns one RuntimeChannel stream and its reconnect loop."""

    def __init__(
        self,
        config: ChannelConfig,
        on_message: MessageHandler | None = None,
        running_runs: RunningRunsProvider | None = None,
        running_count: RunningCountProvider | None = None,
        logger: logging.Logger | None = None,
    ) -> None:
        self.config = config
        self.on_message = on_message
        self.running_runs = running_runs or (lambda: [])
        self.running_count = running_count or (lambda: 0)
        self.logger = logger or logging.getLogger("middleware_harness.channel")

        self._outbound: asyncio.Queue[runtime_pb2.Envelope] = asyncio.Queue(maxsize=256)
        self._connected = asyncio.Event()
        self._stopped = asyncio.Event()
        self._runner: asyncio.Task[None] | None = None
        self._call: Any = None
        self._pending: dict[str, asyncio.Future[runtime_pb2.Envelope]] = {}

    @property
    def connected(self) -> bool:
        """Whether the current stream has completed Hello."""
        return self._connected.is_set()

    async def start(self) -> None:
        """Start the reconnect/read loop."""
        if self._runner is not None:
            return
        self._runner = asyncio.create_task(self._run(), name="runtime-channel")

    async def wait_connected(self, timeout: float | None = None) -> None:
        """Wait until the stream is connected."""
        await asyncio.wait_for(self._connected.wait(), timeout)

    async def send(self, envelope: runtime_pb2.Envelope) -> None:
        """Queue an outbound envelope for the current stream."""
        if self._stopped.is_set():
            raise RuntimeError("channel client is stopped")
        await self._outbound.put(envelope)

    async def send_response(self, request: runtime_pb2.Envelope, payload: Any, field: str) -> None:
        """Queue a response correlated to request.msg_id."""
        await self.send(response_envelope(request, payload, field))

    async def request(
        self,
        payload: Any,
        field: str,
        *,
        run_id: str = "",
        attempt: int = 0,
        lease_epoch: int = 0,
        timeout: float = 30.0,
    ) -> runtime_pb2.Envelope:
        """Send one request and wait for its correlated response envelope."""
        envelope = new_envelope(
            payload,
            field,
            run_id=run_id,
            attempt=attempt,
            lease_epoch=lease_epoch,
        )
        loop = asyncio.get_running_loop()
        future: asyncio.Future[runtime_pb2.Envelope] = loop.create_future()
        self._pending[envelope.msg_id] = future
        try:
            await self.send(envelope)
            return await asyncio.wait_for(future, timeout)
        finally:
            self._pending.pop(envelope.msg_id, None)

    async def stop(self) -> None:
        """Stop reconnecting and close the stream."""
        self._stopped.set()
        if self._call is not None:
            self._call.cancel()
        if self._runner is not None:
            self._runner.cancel()
            try:
                await self._runner
            except asyncio.CancelledError:
                pass
            self._runner = None
        self._connected.clear()

    async def _run(self) -> None:
        delay = self.config.reconnect_initial
        while not self._stopped.is_set():
            try:
                await self._session()
            except asyncio.CancelledError:
                raise
            except Exception as exc:  # keep the daemon alive across transport errors
                self.logger.warning("runtime channel session ended", extra={
                    "daemon_id": self.config.daemon_id,
                    "error": str(exc),
                })
            if self._stopped.is_set():
                return
            sleep_for = min(delay, self.config.reconnect_max)
            sleep_for *= random.uniform(0.8, 1.2)
            self.logger.info("runtime channel reconnecting", extra={
                "daemon_id": self.config.daemon_id,
                "delay_seconds": round(sleep_for, 3),
            })
            await asyncio.sleep(sleep_for)
            delay = min(delay * 2, self.config.reconnect_max)

    async def _session(self) -> None:
        channel = self._make_channel()
        try:
            await asyncio.wait_for(channel.channel_ready(), self.config.hello_timeout)
            stub = runtime_pb2_grpc.RuntimeServiceStub(channel)
            call = stub.Channel()
            self._call = call
            await call.write(self._hello())
            self._connected.set()

            writer = asyncio.create_task(self._writer(call), name="runtime-writer")
            heartbeat = asyncio.create_task(self._heartbeat(), name="runtime-heartbeat")
            try:
                async for envelope in call:
                    await self._dispatch(envelope)
            finally:
                writer.cancel()
                heartbeat.cancel()
                await asyncio.gather(writer, heartbeat, return_exceptions=True)
                self._connected.clear()
                self._call = None
                self._fail_pending(ConnectionError("runtime channel disconnected"))
        finally:
            await channel.close()
            self._connected.clear()

    def _make_channel(self) -> grpc.aio.Channel:
        if self.config.tls:
            return grpc.aio.secure_channel(
                self.config.address,
                grpc.ssl_channel_credentials(),
            )
        return grpc.aio.insecure_channel(self.config.address)

    def _hello(self) -> runtime_pb2.Envelope:
        running = [
            runtime_pb2.RunningRun(run_id=run_id, lease_epoch=lease_epoch)
            for run_id, lease_epoch in self.running_runs()
        ]
        hello = runtime_pb2.Hello(
            daemon_id=self.config.daemon_id,
            auth_token=self.config.auth_token,
            runtime_type=_runtime_type(self.config.runtime_type),
            max_concurrent_runs=self.config.max_concurrent_runs,
            supported_strategies=self.config.strategies,
            version=self.config.version,
            labels=self.config.labels,
            running_runs=running,
        )
        return new_envelope(hello, "hello")

    async def _writer(self, call: Any) -> None:
        while True:
            envelope = await self._outbound.get()
            await call.write(envelope)

    async def _heartbeat(self) -> None:
        while True:
            await asyncio.sleep(self.config.heartbeat_interval)
            heartbeat = runtime_pb2.Heartbeat(
                running_count=self.running_count(),
                cpu_load=0.0,
                mem_mb=0.0,
            )
            await self.send(new_envelope(heartbeat, "heartbeat"))

    async def _dispatch(self, envelope: runtime_pb2.Envelope) -> None:
        if envelope.WhichOneof("payload") == "tool_call_result":
            future = self._pending.get(envelope.correlation_id)
            if future is not None and not future.done():
                future.set_result(envelope)
                return
        if self.on_message is None:
            return
        try:
            result = self.on_message(envelope)
            if inspect.isawaitable(result):
                await result
        except Exception as exc:
            self.logger.error("runtime channel handler failed", extra={
                "daemon_id": self.config.daemon_id,
                "error": str(exc),
            })


    def _fail_pending(self, exc: BaseException) -> None:
        pending = list(self._pending.values())
        self._pending.clear()
        for future in pending:
            if not future.done():
                future.set_exception(exc)


def _runtime_type(value: str) -> int:
    mapping = {
        "normal": runtime_pb2.RuntimeType.NORMAL,
        "sandbox": runtime_pb2.RuntimeType.SANDBOX,
        "longterm": runtime_pb2.RuntimeType.LONGTERM,
    }
    return mapping.get(value, runtime_pb2.RuntimeType.NORMAL)
