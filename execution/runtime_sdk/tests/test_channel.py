"""Levier — RuntimeChannel client tests.

Uses an in-process grpc.aio server to verify Hello, outbound messages,
inbound dispatch, heartbeat, and reconnect behavior.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import asyncio
import unittest
from typing import AsyncIterator

import grpc
from middleware.runtime.v1 import runtime_pb2, runtime_pb2_grpc

from levier_runtime.channel import ChannelClient, ChannelConfig
from levier_runtime.channel.envelope import new_envelope


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
                runtime_name="react1",
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
        self.assertEqual(hello.hello.runtime_name, "react1")
        self.assertEqual(hello.hello.auth_token, "token-test")
        self.assertEqual(hello.hello.version, "test")

        task = runtime_pb2.TaskAck(accepted=True)
        await self.client.send(new_envelope(task, "task_ack", run_id="run-1", attempt=1, lease_epoch=1))
        ack = await wait_for(
            self.service.received,
            lambda envelope: envelope.WhichOneof("payload") == "task_ack",
        )
        self.assertTrue(ack.task_ack.accepted)

        assign = runtime_pb2.TaskAssign(run_id="run-1", app_id="app-1")
        await self.service.outgoing.put(
            new_envelope(assign, "task_assign", run_id="run-1", attempt=1, lease_epoch=1)
        )
        inbound = await wait_for(
            self.incoming,
            lambda envelope: envelope.WhichOneof("payload") == "task_assign",
        )
        self.assertEqual(inbound.task_assign.run_id, "run-1")

    async def test_request_resolves_correlated_tool_result(self) -> None:
        await self.client.start()
        await self.client.wait_connected(2.0)

        request = runtime_pb2.ToolCallRequest(
            invocation_id="inv-1",
            tool_name="add",
            arguments_json='{"a":1}',
            timeout_ms=500,
        )
        pending = asyncio.create_task(self.client.request(
            request,
            "tool_call_request",
            run_id="run-1",
            attempt=1,
            lease_epoch=1,
            timeout=1.0,
        ))
        sent = await wait_for(
            self.service.received,
            lambda envelope: envelope.WhichOneof("payload") == "tool_call_request",
        )
        self.assertEqual(sent.tool_call_request.tool_name, "add")

        result = runtime_pb2.ToolCallResult(
            invocation_id="inv-1",
            ok=True,
            result_json="2",
        )
        await self.service.outgoing.put(new_envelope(
            result,
            "tool_call_result",
            correlation_id=sent.msg_id,
            run_id="run-1",
            attempt=1,
            lease_epoch=1,
        ))
        response = await pending
        self.assertEqual(response.tool_call_result.result_json, "2")
        self.assertTrue(self.incoming.empty())

    async def test_request_timeout(self) -> None:
        await self.client.start()
        await self.client.wait_connected(2.0)
        with self.assertRaises(asyncio.TimeoutError):
            await self.client.request(
                runtime_pb2.ToolCallRequest(invocation_id="inv-1", tool_name="add"),
                "tool_call_request",
                run_id="run-1",
                attempt=1,
                lease_epoch=1,
                timeout=0.01,
            )

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


class ReconnectingClient(ChannelClient):
    """A client whose sessions succeed without opening a transport."""

    sessions = 0

    async def _session(self) -> bool:
        type(self).sessions += 1
        return True


class ChannelBackpressureTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        self.client = ChannelClient(
            ChannelConfig(
                address="127.0.0.1:1",
                daemon_id="daemon-test",
                runtime_name="react1",
                auth_token="token-test",
                reconnect_initial=0.01,
                reconnect_max=0.05,
            )
        )

    async def asyncTearDown(self) -> None:
        await self.client.stop()

    async def test_heartbeat_send_does_not_block_on_full_events(self) -> None:
        for _ in range(self.client._outbound.maxsize):
            self.client._outbound.put_nowait(new_envelope(
                runtime_pb2.Event(seq=1, type=runtime_pb2.EventType.MESSAGE_COMPLETED),
                "event",
                run_id="run-1",
                attempt=1,
                lease_epoch=1,
            ))

        heartbeat = new_envelope(runtime_pb2.Heartbeat(running_count=0), "heartbeat")
        await asyncio.wait_for(self.client.send(heartbeat), timeout=0.1)
        self.assertEqual(self.client._control.qsize(), 1)

    async def test_writer_prefers_control_messages(self) -> None:
        heartbeat = new_envelope(runtime_pb2.Heartbeat(running_count=0), "heartbeat")
        event = new_envelope(
            runtime_pb2.Event(seq=1, type=runtime_pb2.EventType.MESSAGE_COMPLETED),
            "event",
            run_id="run-1",
            attempt=1,
            lease_epoch=1,
        )
        self.client._control.put_nowait(heartbeat)
        self.client._outbound.put_nowait(event)
        got = self.client._take_next()
        self.assertEqual(got.WhichOneof("payload"), "heartbeat")


class ChannelBackoffTest(unittest.IsolatedAsyncioTestCase):
    async def test_successful_session_resets_backoff(self) -> None:
        ReconnectingClient.sessions = 0
        client = ReconnectingClient(
            ChannelConfig(
                address="127.0.0.1:1",
                daemon_id="daemon-test",
                runtime_name="react1",
                auth_token="token-test",
                reconnect_initial=0.01,
                reconnect_max=0.05,
                reconnect_stable_after=0.0,
            )
        )
        await client.start()
        try:
            for _ in range(100):
                if ReconnectingClient.sessions >= 3:
                    break
                await asyncio.sleep(0.01)
            self.assertGreaterEqual(ReconnectingClient.sessions, 3)
            self.assertLessEqual(client._last_reconnect_delay, 0.012)
        finally:
            await client.stop()
