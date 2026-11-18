"""Levier — submit one App run for retrofit verification (P4.1).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from credentials import CredentialsError, load_credentials
from http_client import (
    AppClient,
    HttpError,
    HttpTransportError,
    UrllibTransport,
    parse_run_response,
)


def default_credentials_path() -> Path:
    value = os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "middleware" / "counter-app.json"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Submit one run")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("LEVIER_APP_URL") or os.environ.get("LEVIER_URL") or os.environ.get("MW_URL") or "http://127.0.0.1:8080")
    parser.add_argument("--strategy", default="echo")
    parser.add_argument("--prompt", default="p4.1 task assign probe")
    parser.add_argument("--provider", default="anthropic")
    parser.add_argument("--model", default="claude-test")
    parser.add_argument("--max-steps", type=int, default=8)
    parser.add_argument("--tool-ms", type=int, default=10000, help="per local tool call timeout")
    parser.add_argument("--run-ms", type=int, default=30000, help="run timeout")
    parser.add_argument("--request-id", default="", help="X-Request-ID / trace correlation")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        response = client.create_run(
            {
                "runtime": "normal",
                "strategy": {"name": args.strategy, "params": {"max_steps": args.max_steps}},
                "input": [{"role": "user", "content": args.prompt}],
                "model": {"provider": args.provider, "model": args.model},
                "timeouts": {"run_ms": args.run_ms, "tool_ms": args.tool_ms},
            },
            request_id=args.request_id,
        )
        run = parse_run_response(response)
    except (CredentialsError, HttpError, HttpTransportError) as exc:
        print(f"submit failed: {exc}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            {
                "status": response.status,
                "run_id": run.run_id,
                "run_status": run.status,
                "ws_url": run.ws_url,
                "has_ws_token": bool(run.ws_token.reveal()),
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
