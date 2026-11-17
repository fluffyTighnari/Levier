"""Levier — register the Counter App through real HTTP (P3.1).

Reads env, sends POST /admin/apps to a running Levier, and prints a
sanitized response. The one-time API key is never printed or persisted here.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import sys

from config import ConfigError, from_env, load_dotenv
from http_client import (
    HttpClient,
    HttpError,
    HttpTransportError,
    UrllibTransport,
    parse_create_app_response,
)


def main() -> int:
    load_dotenv()
    try:
        config = from_env()
        response = HttpClient(config, UrllibTransport()).create_app()
        app = parse_create_app_response(response)
    except ConfigError as exc:
        print(f"config error: {exc}", file=sys.stderr)
        return 2
    except (HttpError, HttpTransportError) as exc:
        print(f"request failed: {exc}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            {
                "status": response.status,
                "app_id": app.app_id,
                "name": app.name,
                "has_api_key": app.has_api_key,
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
