"""Levier — MCP runtime process entry point.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import asyncio
import logging
import os
import signal
from typing import Sequence

from levier_mcp_runtime import __version__
from levier_mcp_runtime.profile import default_mcp_socket
from levier_mcp_runtime.server import McpRuntimeServer, serve_stdio
from levier_mcp_runtime.secrets import ChainResolver, EnvSecretResolver, FileSecretResolver
from levier_mcp_runtime.sessions import SessionManager

_DEFAULT_SOCKET = "/tmp/levier-mcp.sock"


def _first_env(*names: str) -> str | None:
    for name in names:
        value = os.environ.get(name)
        if value:
            return value
    return None


def build_parser() -> argparse.ArgumentParser:
    """Build the CLI parser."""
    parser = argparse.ArgumentParser(prog="levier-mcp-runtime")
    default_endpoint = (
        _first_env("LEVIER_MCP_SOCKET", "MCP_RUNTIME_SOCKET")
        or default_mcp_socket()
        or ""
    )
    parser.add_argument(
        "--socket",
        default="" if default_endpoint.startswith("tcp://") else (default_endpoint or _DEFAULT_SOCKET),
        help="Unix domain socket path",
    )
    parser.add_argument(
        "--addr",
        default=(
            default_endpoint
            if default_endpoint.startswith("tcp://")
            else _first_env("LEVIER_MCP_ADDR", "MCP_RUNTIME_ADDR") or ""
        ),
        help="TCP listen address, for example 127.0.0.1:9091",
    )
    parser.add_argument(
        "--stdio",
        action="store_true",
        help="serve one client over stdin/stdout instead of a socket",
    )
    parser.add_argument(
        "--secrets-file",
        default=_first_env("LEVIER_MCP_SECRETS_FILE", "MCP_RUNTIME_SECRETS_FILE") or "",
        help="optional 0600 JSON file of secret name -> value",
    )
    parser.add_argument(
        "--log-level",
        default=_first_env("LEVIER_MCP_LOG_LEVEL", "MCP_RUNTIME_LOG_LEVEL") or "info",
        help="debug, info, warning, or error",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """Run the MCP runtime."""
    args = build_parser().parse_args(argv)
    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    logger = logging.getLogger("mcp_runtime")
    resolver = EnvSecretResolver()
    if args.secrets_file:
        resolver = ChainResolver([FileSecretResolver.load(args.secrets_file), resolver])
    manager = SessionManager(secrets=resolver)

    if args.stdio:
        asyncio.run(serve_stdio(manager, version=__version__, logger=logger))
        return 0

    socket_path = args.socket
    tcp_address = args.addr
    if os.name == "nt" and socket_path == _DEFAULT_SOCKET and not tcp_address:
        socket_path = ""
    if socket_path.startswith("tcp://"):
        tcp_address = socket_path
        socket_path = ""
    if not socket_path and not tcp_address and os.name == "nt":
        tcp_address = "127.0.0.1:9091"
        socket_path = ""
    server = McpRuntimeServer(
        manager,
        socket_path=socket_path,
        tcp_address=tcp_address,
        version=__version__,
        logger=logger,
    )

    async def run() -> None:
        stop = asyncio.Event()
        loop = asyncio.get_running_loop()
        for sig in (signal.SIGINT, signal.SIGTERM):
            try:
                loop.add_signal_handler(sig, stop.set)
            except NotImplementedError:
                pass
        await server.start()
        waiter = asyncio.create_task(stop.wait())
        try:
            await asyncio.wait({waiter}, timeout=None)
        finally:
            waiter.cancel()
            await server.close()

    try:
        asyncio.run(run())
    except KeyboardInterrupt:
        pass
    logger.info("mcp runtime stopped")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
