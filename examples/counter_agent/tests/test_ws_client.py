"""Levier — WebSocket URL/handshake unit tests (P5.2).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import unittest

from ws_client import accept_key, websocket_url


class WebSocketTest(unittest.TestCase):
    def test_p5_2_builds_url(self) -> None:
        url = websocket_url(
            "http://127.0.0.1:8080",
            "/api/v1/ws",
            "app-1",
            "token-1",
        )
        self.assertEqual(url, "ws://127.0.0.1:8080/api/v1/ws?app_id=app-1&token=token-1")

        secure = websocket_url(
            "https://mw.example.test/",
            "/api/v1/ws",
            "app-1",
            "token-1",
        )
        self.assertEqual(secure, "wss://mw.example.test/api/v1/ws?app_id=app-1&token=token-1")

    def test_p5_2_accept_key(self) -> None:
        # RFC 6455 example.
        self.assertEqual(
            accept_key("dGhlIHNhbXBsZSBub25jZQ=="),
            "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=",
        )


if __name__ == "__main__":
    unittest.main()
