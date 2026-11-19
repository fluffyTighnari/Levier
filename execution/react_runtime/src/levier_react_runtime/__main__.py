"""Thin built-in React Runtime entry point.

Almost all ReAct behavior lives in the optional `levier_runtime.react` helper.
This package is the composition layer: configuration, MCP socket, and run(ctx).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import asyncio
import logging
import os
import sys
from pathlib import Path
from typing import Sequence

import yaml

from levier_runtime import Runtime, __version__ as RUNTIME_VERSION
from levier_runtime.llm import ProviderRouter
from levier_runtime.mcp import McpRuntimeClient
from levier_runtime.react import ReactAgent
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


def _apply_legacy_env() -> None:
    """Keep existing Compose/deploy variable names working."""
    for old, new in _LEGACY_ENV.items():
        value = os.environ.get(old)
        if value is not None and new not in os.environ:
            os.environ[new] = value


def _mcp_socket(config_path: str, override: str = "") -> str:
    """Read the optional shared MCP socket without coupling it to RuntimeConfig."""
    if override:
        return override
    for name in ("LEVIER_REACT_RUNTIME_MCP_SOCKET", "LEVIER_MCP_RUNTIME_SOCKET"):
        override = os.environ.get(name)
        if override:
            return override
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


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="levier-react-runtime")
    parser.add_argument(
        "--config",
        default=os.environ.get("LEVIER_REACT_RUNTIME_CONFIG")
        or os.environ.get("LEVIER_RUNTIME_CONFIG")
        or str(_DEFAULT_CONFIG),
        help="path to React Runtime YAML configuration",
    )
    parser.add_argument("--check", action="store_true", help="validate configuration")
    parser.add_argument("--print-config", action="store_true", help="print redacted configuration")
    parser.add_argument("--version", action="store_true", help="print version")
    parser.add_argument("--log-level", default="", help="override log level")
    parser.add_argument("--mcp-socket", default="", help="override MCP Runtime socket")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.version:
        print(f"levier-react-runtime {RUNTIME_VERSION}")
        return 0
    _apply_legacy_env()
    if args.log_level:
        os.environ["LEVIER_RUNTIME_LOG_LEVEL"] = args.log_level
    try:
        runtime = Runtime.from_config(args.config)
    except Exception as exc:
        print(f"react runtime config error: {exc}", file=sys.stderr)
        return 1

    level = getattr(logging, runtime.config.log_level.upper(), logging.INFO)
    logging.basicConfig(level=level)
    if args.check:
        print(f"runtime {runtime.config.runtime_name} / {runtime.config.instance_id}")
        return 0
    if args.print_config:
        print_effective_config(runtime, _mcp_socket(args.config, args.mcp_socket))
        return 0

    try:
        asyncio.run(_serve(runtime, _mcp_socket(args.config, args.mcp_socket)))
    except KeyboardInterrupt:
        pass
    return 0


def print_effective_config(runtime: Runtime, mcp_socket: str) -> None:
    config = runtime.config
    print(f"runtime: {config.runtime_name}")
    print(f"instance_id: {config.instance_id}")
    print(f"runtime_type: {config.runtime_type}")
    print(f"middleware_address: {config.middleware.address}")
    print(f"middleware_tls: {str(config.middleware.tls).lower()}")
    print("middleware_auth_token: ***" if config.middleware.auth_token else "middleware_auth_token: (empty)")
    print(f"mcp_socket: {mcp_socket or '(disabled)'}")
    print(f"max_concurrent_runs: {config.max_concurrent_runs}")
    print(f"log_level: {config.log_level}")


async def _serve(runtime: Runtime, mcp_socket: str) -> None:
    mcp = McpRuntimeClient(mcp_socket) if mcp_socket else None
    tools = ToolRouter(mcp=mcp)
    try:
        await runtime.serve(ReactAgent(ProviderRouter(), tools))
    finally:
        if mcp is not None:
            await mcp.close()


if __name__ == "__main__":
    raise SystemExit(main())
