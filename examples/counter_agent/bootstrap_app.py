"""Middleware — reusable App bootstrap.

Creates (or rotates) an App API key with the Admin token and stores the
runtime credentials in a 0600 file. The plaintext key is never printed.

Design references:
- DESIGN §4.2: Admin app bootstrap and App authentication.
- DESIGN §13.1/§13.2: admin token vs App key, secret handling.

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from dataclasses import replace
from pathlib import Path

from config import ConfigError, from_env, load_dotenv
from credentials import save_credentials, AppCredentials
from http_client import (
    HttpClient,
    HttpError,
    HttpTransportError,
    UrllibTransport,
    parse_create_app_response,
    parse_rotate_key_response,
)


def default_credentials_path() -> Path:
    value = os.environ.get("COUNTER_APP_CREDENTIALS", "")
    if value:
        return Path(value).expanduser()
    return Path.home() / ".config" / "middleware" / "counter-app.json"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Bootstrap a Middleware App")
    parser.add_argument("--name", default="", help="App name; defaults to MW_APP_NAME")
    parser.add_argument("--credentials-out", default="", help="0600 credential file path")
    parser.add_argument("--rotate", action="store_true", help="always rotate the API key")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    load_dotenv()
    try:
        config = from_env()
        if args.name:
            config = replace(config, app_name=args.name)
    except ConfigError as exc:
        print(f"config error: {exc}", file=sys.stderr)
        return 2

    output = Path(args.credentials_out).expanduser() if args.credentials_out else default_credentials_path()
    transport = UrllibTransport()
    client = HttpClient(config, transport)
    try:
        created = client.create_app()
        app = parse_create_app_response(created)
        rotated = False
        api_key = app.api_key
        if args.rotate or not app.has_api_key:
            rotated = True
            api_key = parse_rotate_key_response(client.rotate_app_key(app.app_id))
        save_credentials(
            output,
            AppCredentials(app_id=app.app_id, app_name=app.name, api_key=api_key),
        )
    except (HttpError, HttpTransportError, OSError, ValueError) as exc:
        print(f"bootstrap failed: {exc}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            {
                "app_id": app.app_id,
                "name": app.name,
                "credentials": str(output),
                "rotated": rotated,
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
