"""CLI shell for App processes.

An App only registers its tools and calls :meth:`LevierApp.run_cli`; this
module owns argument parsing, credential loading, logging, and clean CLI
errors.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import logging
import sys
from typing import Callable, Sequence

from .app import LevierApp
from .credentials import CredentialsError

RegisterFunction = Callable[[LevierApp], None]


def run_app(
    register: RegisterFunction | None = None,
    *,
    argv: Sequence[str] | None = None,
    prog: str = "levier-app",
) -> int:
    """Create an App from credentials, register tools, and serve it."""
    parser = argparse.ArgumentParser(prog=prog)
    parser.add_argument(
        "--credentials",
        default="",
        help="credentials file (or LEVIER_CREDENTIALS)",
    )
    parser.add_argument("--server", default="", help="Levier HTTP address")
    parser.add_argument(
        "--mcp-ref",
        default="",
        help="comma-separated MCP refs; defaults to LEVIER_APP_MCP_REFS",
    )
    parser.add_argument("--max-calls", type=int, default=0, help="stop after N tool calls")
    parser.add_argument("--idle-timeout", type=float, default=0.0, help="stop after N idle seconds")
    parser.add_argument("--ping-interval", type=float, default=20.0, help="WebSocket ping interval")
    parser.add_argument("--reconnect-initial", type=float, default=0.5, help="initial reconnect delay")
    parser.add_argument("--reconnect-max", type=float, default=30.0, help="maximum reconnect delay")
    parser.add_argument("--log-level", default="info", help="logging level")
    parser.add_argument("--version", action="store_true", help="print SDK version")
    args = parser.parse_args(argv)

    if args.version:
        from . import __version__

        print(f"{prog} {__version__}")
        return 0

    logging.basicConfig(level=getattr(logging, args.log_level.upper(), logging.INFO))
    mcp_refs = _split_csv(args.mcp_ref) if args.mcp_ref else None
    try:
        app = LevierApp.from_credentials(
            args.credentials or None,
            server=args.server or None,
            mcp_refs=mcp_refs,
        )
    except CredentialsError as exc:
        print(f"credentials error: {exc}", file=sys.stderr)
        print("create credentials with: levier app init", file=sys.stderr)
        return 2
    if register is not None:
        register(app)
    return app.run(
        max_calls=max(0, args.max_calls),
        idle_timeout=max(0.0, args.idle_timeout),
        ping_interval=max(0.0, args.ping_interval),
        reconnect_initial=max(0.0, args.reconnect_initial),
        reconnect_max=max(0.0, args.reconnect_max),
    )


def _split_csv(value: str) -> list[str]:
    return [item.strip() for item in value.split(",") if item.strip()]
