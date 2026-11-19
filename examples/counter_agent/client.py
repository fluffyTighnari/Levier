#!/usr/bin/env python3
"""Submit a Run for the Counter App using the public App SDK.

This is the client side of the example. The long-running App in app.py owns the
local tools; this script uses the same credentials to submit and wait for a Run.

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import argparse
import json
import os
import sys

from levier_app import CredentialsError, LevierClient


def main() -> int:
    parser = argparse.ArgumentParser(description="Submit a Counter App Run")
    parser.add_argument(
        "--prompt",
        default="Add 2, then add 3, then read the counter.",
    )
    parser.add_argument(
        "--runtime",
        default=os.environ.get("LEVIER_RUNTIME_NAME", "react1"),
    )
    parser.add_argument(
        "--provider",
        default=os.environ.get("LEVIER_MODEL_PROVIDER", ""),
        help="provider name registered in the control plane",
    )
    parser.add_argument(
        "--model",
        default=os.environ.get("LEVIER_MODEL_NAME", ""),
        help="model name; defaults to the provider default",
    )
    parser.add_argument("--timeout", type=float, default=300.0)
    args = parser.parse_args()

    try:
        client = LevierClient.from_credentials()
    except CredentialsError as exc:
        print(f"credentials error: {exc}", file=sys.stderr)
        return 2

    providers = client.list_model_providers()
    if not args.provider and providers:
        defaults = [item for item in providers if item.get("default")]
        if defaults:
            args.provider = str(defaults[0].get("name") or "")
    if args.provider:
        print(f"provider: {args.provider}")
    if args.model:
        print(f"model: {args.model}")

    submitted = client.submit_run(
        args.prompt,
        runtime=args.runtime,
        provider=args.provider,
        model=args.model,
    )
    run_id = str(submitted.get("run_id") or "")
    print(f"run: {run_id}")
    try:
        result = client.wait_run(run_id, timeout=args.timeout)
    except TimeoutError as exc:
        print(str(exc), file=sys.stderr)
        return 5

    events = client.get_events(run_id)
    final = [
        event.get("data", {}).get("content")
        for event in events
        if event.get("event_type") == "MESSAGE_COMPLETED"
        and isinstance(event.get("data"), dict)
    ]
    print(json.dumps({"run": result, "final": final[-1] if final else None}, indent=2))
    return 0 if result.get("status") == "completed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
