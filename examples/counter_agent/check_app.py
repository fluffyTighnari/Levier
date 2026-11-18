"""Levier — verify App runtime credentials (P3.3).

Loads the 0600 credential file and calls GET /api/v1/apps/{app_id} with the
App API key. The key is never printed.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

from credentials import CredentialsError, load_credentials
from http_client import AppClient, HttpError, HttpTransportError, UrllibTransport, parse_app_view


def default_credentials_path() -> Path:
    value = os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "middleware" / "counter-app.json"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Verify App key authentication")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("LEVIER_APP_URL") or os.environ.get("LEVIER_URL") or os.environ.get("MW_URL") or "http://127.0.0.1:8080")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        response = client.get_app()
        app = parse_app_view(response)
    except (CredentialsError, HttpError, HttpTransportError) as exc:
        print(f"auth check failed: {exc}", file=sys.stderr)
        return 1

    print(json.dumps({"status": response.status, "app_id": app.app_id, "name": app.name, "app_status": app.status}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
