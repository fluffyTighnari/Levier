"""Tests for the thin Counter App.

Run from the repository root with the SDK on PYTHONPATH:

    PYTHONPATH=sdk/python/src python -m unittest discover -s examples/counter_agent/tests -v
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "sdk" / "python" / "src"))
sys.path.insert(0, str(ROOT / "examples" / "counter_agent"))

from levier_app import Credentials, LevierApp, Secret  # noqa: E402

from app import register_tools  # noqa: E402
from tools import Counter  # noqa: E402


def new_app(counter: Counter | None = None) -> LevierApp:
    app = LevierApp(Credentials(
        app_id="app-counter",
        app_name="counter-agent",
        api_key=Secret("test-key"),
        server="http://127.0.0.1:8080",
    ))
    register_tools(app, counter or Counter())
    return app


class CounterToolsTest(unittest.TestCase):
    def test_tool_manifest_is_small_and_stable(self) -> None:
        app = new_app()
        self.assertEqual(
            [tool["name"] for tool in app.tools_manifest()],
            ["counter_add", "counter_get", "counter_reset"],
        )

    def test_handlers_drive_counter_through_sdk_dispatch(self) -> None:
        app = new_app(Counter(7))
        add = app._dispatch_local_tool({
            "msg_id": "msg-1",
            "invocation_id": "inv-1",
            "tool_name": "counter_add",
            "arguments_json": '{"delta": 2}',
        })
        self.assertEqual(add["ok"], True)
        self.assertEqual(add["result_json"], '{"value":9}')

        get = app._dispatch_local_tool({
            "msg_id": "msg-2",
            "invocation_id": "inv-2",
            "tool_name": "counter_get",
            "arguments_json": "{}",
        })
        self.assertEqual(get["result_json"], '{"value":9}')

        reset = app._dispatch_local_tool({
            "msg_id": "msg-3",
            "invocation_id": "inv-3",
            "tool_name": "counter_reset",
            "arguments_json": "{}",
        })
        self.assertEqual(reset["result_json"], '{"value":0}')


if __name__ == "__main__":
    unittest.main()
