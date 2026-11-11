"""OpenAI-compatible stub returning a large assistant message (P9.10 probe)."""

from __future__ import annotations

import argparse
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=9098)
    parser.add_argument("--bytes", type=int, default=12 * 1024 * 1024)
    return parser


class Handler(BaseHTTPRequestHandler):
    content = "x"

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)
        body = json.dumps({
            "choices": [{"message": {"role": "assistant", "content": self.content}}],
            "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        }).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        print(json.dumps({"path": self.path, "content_bytes": len(self.content)}), flush=True)

    def log_message(self, *args) -> None:
        return


def main() -> int:
    args = build_parser().parse_args()
    Handler.content = "x" * args.bytes
    HTTPServer(("127.0.0.1", args.port), Handler).serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
