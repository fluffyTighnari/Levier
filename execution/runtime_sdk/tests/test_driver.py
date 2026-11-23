"""Runtime driver and capability contract tests."""

from __future__ import annotations

import unittest
from unittest.mock import patch

from levier_runtime import Runtime, RuntimeCapabilities
from levier_runtime.config import MiddlewareConfig, RuntimeConfig
from levier_runtime.driver import driver_run_function


def test_runtime_config() -> RuntimeConfig:
    return RuntimeConfig(
        runtime_name="driver-test",
        instance_id="driver-test-1",
        middleware=MiddlewareConfig(
            address="127.0.0.1:9090",
            auth_token="test-token",
        ),
    )


class DriverWithHooks:
    capabilities = RuntimeCapabilities(
        checkpoint=True,
        custom_events=True,
        waiting=True,
    )

    def __init__(self, calls: list[str]) -> None:
        self.calls = calls

    async def on_start(self, runtime: Runtime) -> None:
        self.calls.append("start")

    async def on_shutdown(self) -> None:
        self.calls.append("shutdown")

    async def run(self, ctx) -> str:
        self.calls.append("run")
        return "completed"


class DriverTest(unittest.IsolatedAsyncioTestCase):
    async def test_driver_lifecycle_and_run(self) -> None:
        calls: list[str] = []
        driver = DriverWithHooks(calls)
        runtime = Runtime(config=test_runtime_config())

        async def fake_serve(run, *, handle_signals: bool = True) -> None:
            calls.append("serve")
            status = await run(object())
            self.assertEqual(status, "completed")

        with patch.object(runtime, "serve", new=fake_serve):
            await runtime.serve_driver(driver, handle_signals=False)

        self.assertEqual(calls, ["start", "serve", "run", "shutdown"])
        self.assertTrue(driver.capabilities.checkpoint)
        self.assertTrue(driver.capabilities.custom_events)
        self.assertTrue(driver.capabilities.waiting)

    async def test_simple_callable_still_works(self) -> None:
        calls: list[str] = []

        async def simple_run(ctx) -> str:
            calls.append("run")
            return "completed"

        runtime = Runtime(config=test_runtime_config())

        async def fake_serve(run, *, handle_signals: bool = True) -> None:
            await run(object())

        with patch.object(runtime, "serve", new=fake_serve):
            await runtime.serve_driver(simple_run, handle_signals=False)

        self.assertEqual(calls, ["run"])

    def test_capability_mapping(self) -> None:
        caps = RuntimeCapabilities(checkpoint=True, runtime_tools=True)
        self.assertEqual(
            caps.as_dict(),
            {
                "checkpoint": True,
                "custom_events": False,
                "runtime_tools": True,
                "providers": False,
                "waiting": False,
                "sandbox": False,
                "longterm": False,
            },
        )

    def test_invalid_driver_is_rejected(self) -> None:
        with self.assertRaises(TypeError):
            driver_run_function(object())


if __name__ == "__main__":
    unittest.main()
