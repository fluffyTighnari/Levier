"""M1: runtime resolves ${secret:...} before starting a real MCP server."""

from __future__ import annotations

import asyncio
import json
import os
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[5]
sys.path.insert(0, str(ROOT / "harness" / "src"))
sys.path.insert(0, str(ROOT / "harness" / "src" / "gen"))

from middleware.runtime.v1 import runtime_pb2

from middleware_harness.mcp import McpRuntimeClient, McpRuntimeError


async def main() -> int:
    runtime_python = ROOT / "mcp_runtime" / ".venv" / "bin" / "python"
    server = Path(__file__).resolve().parent / "secret_echo_server.py"
    with tempfile.TemporaryDirectory() as directory:
        socket_path = str(Path(directory) / "mcp.sock")
        env = os.environ.copy()
        env["PYTHONPATH"] = str(ROOT / "mcp_runtime" / "src")
        env["MIDDLEWARE_SECRET_PROBE_TOKEN"] = "resolved-from-env"
        process = await asyncio.create_subprocess_exec(
            str(runtime_python), "-m", "mcp_runtime",
            "--socket", socket_path, "--log-level", "info",
            cwd=str(ROOT), env=env,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
        )
        for _ in range(300):
            if Path(socket_path).exists():
                break
            await asyncio.sleep(0.01)
        client = McpRuntimeClient(socket_path)
        try:
            health = await client.health()
            binding = runtime_pb2.McpBinding(
                server_name="secret-echo",
                transport="stdio",
                command=str(runtime_python),
                args=[str(server)],
                env={"PROBE_TOKEN": "${secret:probe_token}"},
                allowed_tools=["echo_token"],
                namespace="mcp__secret-echo__",
            )
            session = await client.open(binding)
            result = await client.call(
                session.session_id,
                "mcp__secret-echo__echo_token",
                "{}",
                5000,
            )
            await client.close_session(session.session_id)

            missing_code = None
            try:
                await client.open(runtime_pb2.McpBinding(
                    server_name="secret-missing",
                    transport="stdio",
                    command=str(runtime_python),
                    args=[str(server)],
                    env={"PROBE_TOKEN": "${secret:missing_token}"},
                ))
            except McpRuntimeError as exc:
                missing_code = exc.code

            print(json.dumps({
                "health": health,
                "tools": [tool.name for tool in session.tools],
                "call_ok": result.ok,
                "call_result": result.result_json,
                "missing_secret_code": missing_code,
            }, ensure_ascii=False))
        finally:
            await client.close()
            process.terminate()
            await process.wait()
            stderr = (await process.stderr.read()).decode(errors="replace") if process.stderr else ""
            Path(__file__).resolve().parent.joinpath("runtime.log").write_text(stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(asyncio.run(main()))
