"""Thin built-in React Runtime entry point.

The Runtime SDK owns configuration, lifecycle, events, cancellation, and the
common CLI. This package only composes the optional ReactAgent helper with the
shared MCP client.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import os
import argparse
import asyncio
from pathlib import Path

import yaml

from levier_runtime import __version__ as RUNTIME_VERSION
from levier_runtime.cli import run_cli
from levier_runtime.llm import ProviderRouter
from levier_runtime.mcp import McpRuntimeClient
from levier_runtime.react import ReactAgent
from levier_runtime.runtime import Runtime
from levier_runtime.tools import ToolRouter

_DEFAULT_CONFIG = Path(__file__).resolve().parents[2] / "react.example.yaml"

_LEGACY_ENV = {
    "LEVIER_REACT_RUNTIME_ADDRESS": "LEVIER_MIDDLEWARE_ADDRESS",
    "LEVIER_REACT_RUNTIME_TLS": "LEVIER_MIDDLEWARE_TLS",
    "LEVIER_REACT_RUNTIME_TOKEN": "LEVIER_RUNTIME_TOKEN",
    "LEVIER_REACT_RUNTIME_TOKEN_FILE": "LEVIER_RUNTIME_TOKEN_FILE",
    "LEVIER_REACT_RUNTIME_INSTANCE_ID": "LEVIER_RUNTIME_INSTANCE_ID",
    "LEVIER_REACT_RUNTIME_TYPE": "LEVIER_RUNTIME_TYPE",
    "LEVIER_REACT_RUNTIME_MAX_CONCURRENT_RUNS": "LEVIER_RUNTIME_MAX_CONCURRENT_RUNS",
    "LEVIER_REACT_RUNTIME_NAME": "LEVIER_RUNTIME_NAME",
    "LEVIER_REACT_RUNTIME_LABELS": "LEVIER_RUNTIME_LABELS",
    "LEVIER_REACT_RUNTIME_LOG_LEVEL": "LEVIER_RUNTIME_LOG_LEVEL",
}


def _add_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--mcp-socket", default="", help="override MCP Runtime socket")


def _mcp_socket(config_path: str, override: str) -> str:
    """Read the optional shared MCP socket without coupling it to RuntimeConfig."""
    if override:
        return override
    for name in ("LEVIER_REACT_RUNTIME_MCP_SOCKET", "LEVIER_MCP_RUNTIME_SOCKET"):
        value = os.environ.get(name)
        if value:
            return value
    try:
        data = yaml.safe_load(Path(config_path).read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError):
        return ""
    if not isinstance(data, dict):
        return ""
    mcp = data.get("mcp_runtime")
    if not isinstance(mcp, dict):
        return ""
    socket = mcp.get("socket")
    return socket if isinstance(socket, str) else ""


async def _serve(runtime: Runtime, args: argparse.Namespace) -> None:
    mcp_socket = _mcp_socket(args.config, args.mcp_socket)
    mcp = McpRuntimeClient(mcp_socket) if mcp_socket else None
    tools = ToolRouter(mcp=mcp)
    try:
        await runtime.serve(ReactAgent(ProviderRouter(), tools))
    finally:
        if mcp is not None:
            await mcp.close()


def _print_extra(runtime: Runtime, args: argparse.Namespace) -> list[str]:
    return [f"mcp_socket: {_mcp_socket(args.config, args.mcp_socket) or '(disabled)'}"]


def main(argv: list[str] | None = None) -> int:
    return run_cli(
        prog="levier-react-runtime",
        version=RUNTIME_VERSION,
        default_config=_DEFAULT_CONFIG,
        config_env=("LEVIER_REACT_RUNTIME_CONFIG", "LEVIER_RUNTIME_CONFIG"),
        serve=_serve,
        add_arguments=_add_arguments,
        env_aliases=_LEGACY_ENV,
        print_config_extra=_print_extra,
        argv=argv,
    )


if __name__ == "__main__":
    raise SystemExit(main())
