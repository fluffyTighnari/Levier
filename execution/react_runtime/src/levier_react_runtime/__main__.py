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

from levier_runtime import Runtime
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


def _mcp_socket(config_path: str) -> str:
    """Read the optional shared MCP socket without coupling it to RuntimeConfig."""
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
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    _apply_legacy_env()
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

    try:
        asyncio.run(_serve(runtime, _mcp_socket(args.config)))
    except KeyboardInterrupt:
        pass
    return 0


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
