"""M8: MCP HTTP transport drop mid-call, then recovery with a new session."""

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

RUNTIME_PY = ROOT / "mcp_runtime" / ".venv" / "bin" / "python"
SERVER_PY = ROOT / "mcp_runtime" / "tests" / "http_echo_server.py"


async def wait_port(port: int, timeout: float = 10.0) -> None:
    import socket

    deadline = asyncio.get_running_loop().time() + timeout
    while asyncio.get_running_loop().time() < deadline:
        try:
            reader, writer = await asyncio.open_connection("127.0.0.1", port)
            writer.close()
            await writer.wait_closed()
            return
        except OSError:
            await asyncio.sleep(0.05)
    raise RuntimeError(f"port {port} did not open")


async def stop(process: asyncio.subprocess.Process) -> None:
    if process.returncode is not None:
        return
    process.terminate()
    try:
        await asyncio.wait_for(process.wait(), 5)
    except asyncio.TimeoutError:
        process.kill()
        await process.wait()


async def main(transport: str, port: int) -> int:
    socket_path = str(Path(tempfile.mkdtemp()) / "mcp.sock")
    env = os.environ.copy()
    env["PYTHONPATH"] = str(ROOT / "mcp_runtime" / "src")
    runtime = await asyncio.create_subprocess_exec(
        str(RUNTIME_PY), "-m", "mcp_runtime", "--socket", socket_path, "--log-level", "warning",
        cwd=str(ROOT), env=env,
        stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
    )
    for _ in range(300):
        if Path(socket_path).exists():
            break
        await asyncio.sleep(0.01)

    server = await asyncio.create_subprocess_exec(
        str(RUNTIME_PY), str(SERVER_PY), "--transport", transport, "--port", str(port),
        cwd=str(ROOT), env=env,
        stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
    )
    await wait_port(port)

    client = McpRuntimeClient(socket_path)
    result: dict[str, object] = {"transport": transport}
    session = None
    try:
        url = (
            f"http://127.0.0.1:{port}/sse"
            if transport == "sse"
            else f"http://127.0.0.1:{port}/mcp"
        )
        namespace = f"mcp__{transport.replace('-', '_')}__"
        binding = runtime_pb2.McpBinding(
            server_name=f"{transport}-drop",
            transport=transport.replace("-", "_"),
            url=url,
            namespace=namespace,
        )
        session = await client.open(binding)
        result["opened"] = [tool.name for tool in session.tools]

        call = asyncio.create_task(
            client.call(session.session_id, namespace + "slow", '{"seconds":30}', 8000)
        )
        await asyncio.sleep(1.0)
        await stop(server)
        try:
            outcome = await asyncio.wait_for(call, 15.0)
            result["mid_call"] = {
                "ok": outcome.ok,
                "error_code": outcome.error_code,
            }
        except McpRuntimeError as exc:
            result["mid_call"] = {"ok": False, "error_code": exc.code}
        except asyncio.TimeoutError:
            result["mid_call"] = {"ok": False, "error_code": "CLIENT_TIMEOUT"}

        result["sessions_after_drop"] = (await client.health()).get("sessions")
        try:
            await client.close_session(session.session_id)
        except McpRuntimeError:
            pass
        result["sessions_after_close"] = (await client.health()).get("sessions")

        down_code = "opened"
        try:
            await asyncio.wait_for(client.open(binding), 12.0)
        except McpRuntimeError as exc:
            down_code = exc.code
        except asyncio.TimeoutError:
            down_code = "OPEN_TIMEOUT"
        result["open_while_down"] = down_code

        server = await asyncio.create_subprocess_exec(
            str(RUNTIME_PY), str(SERVER_PY), "--transport", transport, "--port", str(port),
            cwd=str(ROOT), env=env,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
        )
        await wait_port(port)
        fresh = await client.open(binding)
        echo = await client.call(fresh.session_id, namespace + "echo", '{"text":"recovered"}', 5000)
        result["recovered"] = {"ok": echo.ok, "result": echo.result_json}
        await client.close_session(fresh.session_id)
    finally:
        await client.close()
        await stop(server)
        await stop(runtime)

    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    transport = sys.argv[1] if len(sys.argv) > 1 else "sse"
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 9101
    raise SystemExit(asyncio.run(main(transport, port)))
