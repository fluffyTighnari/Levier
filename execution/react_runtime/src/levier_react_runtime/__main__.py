"""Thin built-in React Runtime entry point.

The Runtime SDK owns configuration, lifecycle, events, cancellation, the tool
toolbox, and the common CLI. This package only composes the optional ReactAgent
helper with the provider router.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

from pathlib import Path

from levier_runtime import __version__ as RUNTIME_VERSION
from levier_runtime.cli import run_cli
from levier_runtime.llm import ProviderRouter
from levier_runtime.react import ReactAgent
from levier_runtime.runtime import Runtime

_SOURCE_CONFIG = Path(__file__).resolve().parents[2] / "react.example.yaml"
_DEFAULT_CONFIG = (
    _SOURCE_CONFIG
    if _SOURCE_CONFIG.is_file()
    else Path(__file__).with_name("react.example.yaml")
)


async def _serve(runtime: Runtime, _args) -> None:
    await runtime.serve(ReactAgent(ProviderRouter()))


def main(argv: list[str] | None = None) -> int:
    return run_cli(
        prog="levier-react-runtime",
        version=RUNTIME_VERSION,
        default_config=_DEFAULT_CONFIG,
        config_env=("LEVIER_RUNTIME_CONFIG",),
        serve=_serve,
        argv=argv,
    )


if __name__ == "__main__":
    raise SystemExit(main())
