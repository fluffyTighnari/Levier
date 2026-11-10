"""Middleware — obtain an App ws_token (P5.1).

Copyright 2026 The Middleware Authors.
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
    parse_ws_token_response,
)


def default_credentials_path() -> Path:
    value = os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "middleware" / "counter-app.json"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Obtain a ws_token with the App key")
    parser.add_argument("--credentials", default="", help="0600 credential file path")
    parser.add_argument("--mw-url", default=os.environ.get("MW_URL", "http://127.0.0.1:8080"))
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    path = Path(args.credentials).expanduser() if args.credentials else default_credentials_path()
    try:
        credentials = load_credentials(path)
        client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
        response = client.create_ws_token()
        token = parse_ws_token_response(response)
    except (CredentialsError, HttpError, HttpTransportError) as exc:
        print(f"ws-token failed: {exc}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            {
                "status": response.status,
                "ws_url": token.ws_url,
                "expires_in": token.expires_in,
                "has_ws_token": bool(token.token.reveal()),
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
