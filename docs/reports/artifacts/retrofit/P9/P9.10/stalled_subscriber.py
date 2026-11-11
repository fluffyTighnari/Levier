"""Subscribe to a run, then stop reading to emulate a slow consumer (P9.10).

It prints caught_up after the replay, then sleeps without calling recv, so the
server's socket send buffer fills on the next published event.
"""

from __future__ import annotations

import argparse
import json
import time
from pathlib import Path

from credentials import load_credentials
from http_client import AppClient, UrllibTransport, parse_ws_token_response
from ws_client import WebSocketClient, websocket_url


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--credentials", required=True)
    parser.add_argument("--run-id", action="append", required=True)
    parser.add_argument("--mw-url", default="http://127.0.0.1:8080")
    parser.add_argument("--stall-seconds", type=float, default=60.0)
    args = parser.parse_args()

    credentials = load_credentials(Path(args.credentials))
    client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())
    token = parse_ws_token_response(client.create_ws_token())
    url = websocket_url(args.mw_url, token.ws_url, credentials.app_id, token.token.reveal())
    ws = WebSocketClient.connect(url)
    for run_id in args.run_id:
        ws.send_json({
            "type": "subscribe",
            "run_id": run_id,
            "last_attempt": 0,
            "last_seq": 0,
        })
        try:
            while True:
                message = ws.recv_json(timeout=5.0)
                if message.get("type") == "caught_up":
                    print(json.dumps({"caught_up": message}), flush=True)
                    break
        except Exception as exc:  # noqa: BLE001 - probe should explain failure.
            print(json.dumps({"subscribe_failed": str(exc)}), flush=True)
            ws.close()
            return 1

    print(json.dumps({"stalled": True, "seconds": args.stall_seconds}), flush=True)
    time.sleep(args.stall_seconds)
    ws.close()
    print(json.dumps({"closed": True}), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
