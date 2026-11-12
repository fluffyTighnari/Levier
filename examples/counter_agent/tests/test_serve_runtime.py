"""Middleware — App runtime serving loop tests.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import socket
import unittest

from tools import Counter
from ws_tool_host import serve_calls


class FakeWS:
    def __init__(self, messages: list[dict]) -> None:
        self.messages = list(messages)
        self.sent: list[dict] = []
        self.closed = False

    def recv_json(self, timeout: float = 5.0) -> dict:
        if not self.messages:
            raise socket.timeout()
        return self.messages.pop(0)

    def send_json(self, payload: dict) -> None:
        self.sent.append(payload)

    def close(self) -> None:
        self.closed = True


def call(msg_id: str, invocation_id: str, tool_name: str, arguments_json: str) -> dict:
    return {
        "type": "local_tool_call",
        "msg_id": msg_id,
        "invocation_id": invocation_id,
        "tool_name": tool_name,
        "arguments_json": arguments_json,
    }


class ServeCallsTest(unittest.TestCase):
    def test_p6_4_serves_until_idle(self) -> None:
        counter = Counter()
        ws = FakeWS([
            call("m1", "i1", "counter_add", '{"delta": 2}'),
            call("m2", "i2", "counter_get", "{}"),
        ])
        logs: list[dict] = []

        handled, reason = serve_calls(ws, counter, timeout=0.01, emit=logs.append)

        self.assertEqual((handled, reason), (2, "idle"))
        self.assertEqual([item["correlation_id"] for item in ws.sent], ["m1", "m2"])
        self.assertEqual(json.loads(ws.sent[1]["result_json"]), {"value": 2})
        self.assertEqual(counter.get(), 2)
        self.assertEqual(len([entry for entry in logs if "local_tool_result" in entry]), 2)

    def test_stops_at_receive_count(self) -> None:
        ws = FakeWS([
            call("m1", "i1", "counter_add", '{"delta": 1}'),
            call("m2", "i2", "counter_add", '{"delta": 1}'),
        ])

        handled, reason = serve_calls(ws, Counter(), receive_count=1, timeout=0.01)

        self.assertEqual((handled, reason), (1, "count"))
        self.assertEqual(len(ws.sent), 1)
        self.assertEqual(len(ws.messages), 1)

    def test_run_completed_stops_serving(self) -> None:
        ws = FakeWS([{"type": "run_completed", "run_id": "r1"}])

        handled, reason = serve_calls(ws, Counter(), timeout=0.01)

        self.assertEqual((handled, reason), (0, "completed"))

    def test_ignores_call_without_correlation_ids(self) -> None:
        ws = FakeWS([
            {"type": "local_tool_call", "tool_name": "counter_add", "arguments_json": "{}"},
            call("m2", "i2", "counter_get", "{}"),
        ])

        handled, reason = serve_calls(ws, Counter(), timeout=0.01)

        self.assertEqual((handled, reason), (1, "idle"))
        self.assertEqual(ws.sent[0]["correlation_id"], "m2")


if __name__ == "__main__":
    unittest.main()
