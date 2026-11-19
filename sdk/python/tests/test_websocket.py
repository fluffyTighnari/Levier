from __future__ import annotations

import unittest

from levier_app.websocket import websocket_url


class WebSocketUrlTest(unittest.TestCase):
    def test_relative_and_absolute_urls(self) -> None:
        self.assertEqual(
            websocket_url("http://127.0.0.1:8080", "/api/v1/ws", "app-1", "token"),
            "ws://127.0.0.1:8080/api/v1/ws?app_id=app-1&token=token",
        )
        self.assertEqual(
            websocket_url("https://levier.example", "wss://ws.example/ws", "app-1", "token"),
            "wss://ws.example/ws?app_id=app-1&token=token",
        )


if __name__ == "__main__":
    unittest.main()
