"""Middleware — register Counter App local tools (P3.4).

Uses the App API key from the 0600 credential file and sends the full local
tool list to POST /api/v1/apps. The key is never printed.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from credentials import CredentialsError, load_credentials
from http_client import AppClient, HttpError, HttpTransportError, UrllibTransport, parse_app_view
from tools import TOOL_DEFINITIONS


def default_credentials_path() -> Path:
    value = os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "middleware" / "counter-app.json"


def load_tools(path: str) -> list[dict]:
    """Load a tool list from a JSON file, or use the counter defaults."""
    if not path:
        return TOOL_DEFINITIONS
    with open(path, "r", encoding="utf-8") as handle:
        payload = json.load(handle)
    if isinstance(payload, dict):
        payload = payload.get("tools")
    if not isinstance(payload, list) or not payload:
        raise ValueError("tools file must contain a non-empty list or {'tools': [...]}")
    return payload


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Register App local tools")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("LEVIER_URL") or os.environ.get("MW_URL") or "http://127.0.0.1:8080")
    parser.add_argument("--tools-file", default="", help="optional JSON tool list")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        tools = load_tools(args.tools_file)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        response = client.update_tools(credentials.app_name, tools)
        app = parse_app_view(response)
    except (CredentialsError, HttpError, HttpTransportError, OSError, ValueError) as exc:
        print(f"tool registration failed: {exc}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            {
                "status": response.status,
                "app_id": app.app_id,
                "name": app.name,
                "app_status": app.status,
                "tool_count": len(tools),
                "tools": [tool["name"] for tool in tools],
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
