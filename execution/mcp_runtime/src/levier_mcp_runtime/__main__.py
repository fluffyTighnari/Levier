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
from levier_mcp_runtime.server import McpRuntimeServer, serve_stdio
from levier_mcp_runtime.secrets import ChainResolver, EnvSecretResolver, FileSecretResolver
from levier_mcp_runtime.sessions import SessionManager

_DEFAULT_SOCKET = "/tmp/levier-mcp.sock"


def build_parser() -> argparse.ArgumentParser:
    """Build the CLI parser."""
    parser = argparse.ArgumentParser(prog="levier-mcp-runtime")
    parser.add_argument(
        "--socket",
        default=os.environ.get("MCP_RUNTIME_SOCKET", _DEFAULT_SOCKET),
        help="Unix domain socket path",
    )
    parser.add_argument(
        "--stdio",
        action="store_true",
        help="serve one client over stdin/stdout instead of a Unix socket",
    )
    parser.add_argument(
        "--secrets-file",
        default=os.environ.get("MCP_RUNTIME_SECRETS_FILE", ""),
        help="optional 0600 JSON file of secret name -> value",
    )
    parser.add_argument(
        "--log-level",
        default=os.environ.get("MCP_RUNTIME_LOG_LEVEL", "info"),
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

    server = McpRuntimeServer(
        manager,
        socket_path=args.socket,
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
