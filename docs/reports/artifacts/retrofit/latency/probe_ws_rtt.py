"""Middleware — App WebSocket RTT probe.

Measures the round trip of App→MW→App ping/pong on a real ws_token
connection. Used only for the retrofit latency measurements.

Usage:
    PYTHONPATH=examples/counter_agent python3 probe_ws_rtt.py \
        --credentials /tmp/app.json --count 200
"""

from __future__ import annotations

import argparse
import json
import statistics
import time
from pathlib import Path

from credentials import load_credentials
from http_client import AppClient, UrllibTransport, parse_ws_token_response
from ws_client import WebSocketClient, websocket_url


def percentile(values: list[float], pct: float) -> float:
    ordered = sorted(values)
    index = min(len(ordered) - 1, int(round((pct / 100) * (len(ordered) - 1))))
    return ordered[index]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--credentials", required=True)
    parser.add_argument("--mw-url", default="http://127.0.0.1:8080")
    parser.add_argument("--count", type=int, default=200)
    parser.add_argument("--warmup", type=int, default=10)
    args = parser.parse_args()

    credentials = load_credentials(Path(args.credentials))
    client = AppClient(args.mw_url, credentials.app_id, credentials.api_key, UrllibTransport())

    token_started = time.perf_counter()
    token = parse_ws_token_response(client.create_ws_token())
    token_ms = (time.perf_counter() - token_started) * 1000

    connect_started = time.perf_counter()
    url = websocket_url(args.mw_url, token.ws_url, credentials.app_id, token.token.reveal())
    ws = WebSocketClient.connect(url)
    connect_ms = (time.perf_counter() - connect_started) * 1000

    samples: list[float] = []
    try:
        for index in range(args.warmup + args.count):
            started = time.perf_counter()
            ws.send_json({"type": "ping"})
            while True:
                message = ws.recv_json(timeout=10.0)
                if message.get("type") == "pong":
                    break
            elapsed = (time.perf_counter() - started) * 1000
            if index >= args.warmup:
                samples.append(elapsed)
    finally:
        ws.close()

    print(json.dumps({
        "app_id": credentials.app_id,
        "token_ms": round(token_ms, 3),
        "connect_ms": round(connect_ms, 3),
        "count": len(samples),
        "rtt_ms": {
            "min": round(min(samples), 3),
            "p50": round(statistics.median(samples), 3),
            "p90": round(percentile(samples, 90), 3),
            "p99": round(percentile(samples, 99), 3),
            "max": round(max(samples), 3),
            "mean": round(statistics.fmean(samples), 3),
            "stdev": round(statistics.pstdev(samples), 3),
        },
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
