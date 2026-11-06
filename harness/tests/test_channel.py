"""Middleware — RuntimeChannel client tests.

Uses an in-process grpc.aio server to verify Hello, outbound messages,
inbound dispatch, heartbeat, and reconnect behavior.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import asyncio
import unittest
from typing import AsyncIterator

import grpc
from middleware.runtime.v1 import runtime_pb2, runtime_pb2_grpc

from middleware_harness.channel import ChannelClient, ChannelConfig
from middleware_harness.channel.envelope import new_envelope


class FakeRuntimeService(runtime_pb2_grpc.RuntimeServiceServicer):
    """Minimal in-process RuntimeService implementation."""

    def __init__(self) -> None:
        self.received: asyncio.Queue[runtime_pb2.Envelope] = asyncio.Queue()
        self.outgoing: asyncio.Queue[runtime_pb2.Envelope | None] = asyncio.Queue()
        self.connection_count = 0

    async def Channel(  # type: ignore[override]
        self,
        request_iterator: AsyncIterator[runtime_pb2.Envelope],
        context: grpc.aio.ServicerContext,
    ) -> AsyncIterator[runtime_pb2.Envelope]:
        self.connection_count += 1

        async def consume() -> None:
            async for envelope in request_iterator:
                await self.received.put(envelope)

        consumer = asyncio.create_task(consume())
        try:
            while True:
                envelope = await self.outgoing.get()
                if envelope is None:
                    return
                yield envelope
        finally:
            consumer.cancel()
            await asyncio.gather(consumer, return_exceptions=True)


async def wait_for(
    queue: asyncio.Queue[runtime_pb2.Envelope],
    predicate,
    timeout: float = 2.0,
) -> runtime_pb2.Envelope:
    async def inner() -> runtime_pb2.Envelope:
        while True:
            envelope = await queue.get()
            if predicate(envelope):
                return envelope

    return await asyncio.wait_for(inner(), timeout)


class ChannelClientTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.service = FakeRuntimeService()
        self.server = grpc.aio.server()
        runtime_pb2_grpc.add_RuntimeServiceServicer_to_server(
            self.service,
            self.server,
        )
        port = self.server.add_insecure_port("127.0.0.1:0")
        await self.server.start()
        self.address = f"127.0.0.1:{port}"
        self.client = ChannelClient(
            ChannelConfig(
                address=self.address,
                daemon_id="daemon-test",
                auth_token="token-test",
                version="test",
                heartbeat_interval=0.05,
                reconnect_initial=0.01,
                reconnect_max=0.05,
                hello_timeout=1.0,
            ),
            on_message=self._on_message,
        )
        self.incoming: asyncio.Queue[runtime_pb2.Envelope] = asyncio.Queue()

    async def asyncTearDown(self) -> None:
        await self.client.stop()
        await self.server.stop(0)

    async def _on_message(self, envelope: runtime_pb2.Envelope) -> None:
        await self.incoming.put(envelope)

    async def test_hello_send_and_dispatch(self) -> None:
        await self.client.start()
        await self.client.wait_connected(2.0)

        hello = await wait_for(
            self.service.received,
            lambda envelope: envelope.WhichOneof("payload") == "hello",
        )
        self.assertEqual(hello.hello.daemon_id, "daemon-test")
        self.assertEqual(hello.hello.auth_token, "token-test")
        self.assertEqual(hello.hello.version, "test")

        task = runtime_pb2.TaskAck(accepted=True)
        await self.client.send(new_envelope(task, "task_ack", run_id="run-1", attempt=1, lease_epoch=1))
        ack = await wait_for(
            self.service.received,
            lambda envelope: envelope.WhichOneof("payload") == "task_ack",
        )
        self.assertTrue(ack.task_ack.accepted)

        assign = runtime_pb2.TaskAssign(run_id="run-1", app_id="app-1", strategy="echo")
        await self.service.outgoing.put(
            new_envelope(assign, "task_assign", run_id="run-1", attempt=1, lease_epoch=1)
        )
        inbound = await wait_for(
            self.incoming,
            lambda envelope: envelope.WhichOneof("payload") == "task_assign",
        )
        self.assertEqual(inbound.task_assign.run_id, "run-1")

    async def test_heartbeat(self) -> None:
        await self.client.start()
        await self.client.wait_connected(2.0)
        heartbeat = await wait_for(
            self.service.received,
            lambda envelope: envelope.WhichOneof("payload") == "heartbeat",
        )
        self.assertEqual(heartbeat.heartbeat.running_count, 0)

    async def test_reconnect(self) -> None:
        await self.client.start()
        await self.client.wait_connected(2.0)
        first_count = self.service.connection_count

        # End the current server stream; the client should reconnect.
        await self.service.outgoing.put(None)
        for _ in range(100):
            if self.service.connection_count > first_count:
                break
            await asyncio.sleep(0.01)
        self.assertGreater(self.service.connection_count, first_count)

        hello = await wait_for(
            self.service.received,
            lambda envelope: envelope.WhichOneof("payload") == "hello",
        )
        self.assertEqual(hello.hello.daemon_id, "daemon-test")


if __name__ == "__main__":
    unittest.main()
