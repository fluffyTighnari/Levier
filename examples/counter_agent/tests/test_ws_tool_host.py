"""Middleware — App WebSocket tool host tests (P6.4).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import unittest

from tools import Counter
from ws_tool_host import build_local_tool_result


def call(msg_id: str, invocation_id: str, tool_name: str, arguments_json: str) -> dict:
    return {
        "type": "local_tool_call",
        "msg_id": msg_id,
        "invocation_id": invocation_id,
        "tool_name": tool_name,
        "arguments_json": arguments_json,
        "timeout_ms": 10000,
    }


class LocalToolResultTest(unittest.TestCase):
    def test_p6_4_add_returns_correlated_result(self) -> None:
        counter = Counter()
        result = build_local_tool_result(counter, call("msg-1", "inv-1", "counter_add", '{"delta": 2}'))

        self.assertEqual(result["type"], "local_tool_result")
        self.assertEqual(result["correlation_id"], "msg-1")
        self.assertEqual(result["invocation_id"], "inv-1")
        self.assertTrue(result["ok"])
        self.assertEqual(json.loads(result["result_json"]), {"value": 2})
        self.assertEqual(counter.get(), 2)

    def test_p6_4_get_and_reset(self) -> None:
        counter = Counter()
        build_local_tool_result(counter, call("m", "i", "counter_add", '{"delta": 5}'))

        got = build_local_tool_result(counter, call("m2", "i2", "counter_get", "{}"))
        self.assertEqual(json.loads(got["result_json"]), {"value": 5})

        reset = build_local_tool_result(counter, call("m3", "i3", "counter_reset", ""))
        self.assertEqual(json.loads(reset["result_json"]), {"value": 0})
        self.assertEqual(counter.get(), 0)

    def test_p6_4_state_persists_across_calls(self) -> None:
        counter = Counter()
        build_local_tool_result(counter, call("a", "1", "counter_add", '{"delta": 2}'))
        build_local_tool_result(counter, call("b", "2", "counter_add", '{"delta": 3}'))
        final = build_local_tool_result(counter, call("c", "3", "counter_get", "{}"))

        self.assertEqual(json.loads(final["result_json"]), {"value": 5})

    def test_p6_4_error_is_structured(self) -> None:
        counter = Counter()
        result = build_local_tool_result(counter, call("msg-1", "inv-1", "counter_add", '{"delta": "two"}'))

        self.assertFalse(result["ok"])
        self.assertNotIn("result_json", result)
        self.assertEqual(result["error"]["code"], "TOOL_EXEC_ERROR")
        self.assertEqual(counter.get(), 0)

    def test_p6_4_unknown_tool(self) -> None:
        result = build_local_tool_result(Counter(), call("m", "i", "counter_mul", "{}"))

        self.assertFalse(result["ok"])
        self.assertEqual(result["error"]["code"], "TOOL_NOT_FOUND")


if __name__ == "__main__":
    unittest.main()
