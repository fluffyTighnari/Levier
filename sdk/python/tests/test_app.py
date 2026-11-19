from __future__ import annotations

import json
import unittest
from pathlib import Path

from levier_app import Credentials, LevierApp, Secret, ToolExecutionError
from levier_app.app import PROTOCOL_VERSION, SDK_VERSION, hello_message

FIXTURES = Path(__file__).parent / "fixtures"


class AdapterContractTest(unittest.TestCase):
    def test_hello_fixture_and_versions(self) -> None:
        self.assertEqual(PROTOCOL_VERSION, 1)
        self.assertEqual(SDK_VERSION, "0.1.0")
        expected = json.loads((FIXTURES / "hello.json").read_text())
        self.assertEqual(hello_message(), expected)


class AppToolTest(unittest.TestCase):
    def app(self) -> LevierApp:
        return LevierApp(Credentials(
            app_id="app-1",
            app_name="demo",
            api_key=Secret("mw_secret"),
            server="http://127.0.0.1:8080",
        ))

    def test_manifest_and_successful_dispatch(self) -> None:
        app = self.app()

        @app.tool(
            "counter_add",
            description="Add a delta.",
            schema={
                "type": "object",
                "properties": {"delta": {"type": "integer"}},
                "required": ["delta"],
                "additionalProperties": False,
            },
        )
        def counter_add(arguments):
            return {"value": arguments["delta"]}

        manifest = app.tools_manifest()
        self.assertEqual(manifest[0]["name"], "counter_add")
        self.assertEqual(manifest[0]["timeout_ms"], 10000)

        call = json.loads((FIXTURES / "local_tool_call.json").read_text())
        result = app._dispatch_local_tool(call)
        expected = json.loads((FIXTURES / "local_tool_result_success.json").read_text())
        self.assertEqual(result, expected)

    def test_handler_error_maps_to_result(self) -> None:
        app = self.app()

        @app.tool("counter_add")
        def counter_add(_arguments):
            raise ToolExecutionError("TOOL_EXEC_ERROR", "delta must be an integer")

        call = json.loads((FIXTURES / "local_tool_call.json").read_text())
        result = app._dispatch_local_tool(call)
        expected = json.loads((FIXTURES / "local_tool_result_error.json").read_text())
        self.assertEqual(result, expected)

    def test_unknown_tool_and_bad_arguments(self) -> None:
        app = self.app()

        @app.tool("counter_get")
        def counter_get(_arguments):
            return 0

        call = json.loads((FIXTURES / "local_tool_call.json").read_text())
        call["tool_name"] = "missing"
        result = app._dispatch_local_tool(call)
        self.assertFalse(result["ok"])
        self.assertEqual(result["error"]["code"], "TOOL_NOT_FOUND")

        call["tool_name"] = "counter_get"
        call["arguments_json"] = "[]"
        result = app._dispatch_local_tool(call)
        self.assertFalse(result["ok"])
        self.assertEqual(result["error"]["code"], "TOOL_EXEC_ERROR")

    def test_mcp_refs_are_declared_on_connect(self) -> None:
        app = LevierApp(
            Credentials(
                app_id="app-1",
                app_name="demo",
                api_key=Secret("mw_secret"),
                server="http://127.0.0.1:8080",
            ),
            mcp_refs=["github"],
        )
        self.assertEqual(app.mcp_refs, ["github"])

    def test_duplicate_tool_is_rejected(self) -> None:
        app = self.app()

        @app.tool("counter_get")
        def first(_arguments):
            return 0

        with self.assertRaises(ValueError):
            @app.tool("counter_get")
            def second(_arguments):
                return 1

        self.assertEqual(len(app.tools_manifest()), 1)


if __name__ == "__main__":
    unittest.main()
