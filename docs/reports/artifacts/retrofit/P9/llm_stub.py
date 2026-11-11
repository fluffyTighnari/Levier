"""Return a fixed HTTP status for /chat/completions (P9.6 probe)."""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

STATUS_FILE = Path(sys.argv[1])
PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 9099


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)
        try:
            status = int(STATUS_FILE.read_text().strip())
        except (OSError, ValueError):
            status = 500
        body = json.dumps({"error": {"message": f"stub HTTP {status}", "type": "stub"}}).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        print(json.dumps({"path": self.path, "status": status}), flush=True)

    def log_message(self, *args):
        return


HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
