"""Middleware — App key request/response tests (P3.3).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import unittest

from config import Secret
from http_client import (
    AppClient,
    HttpClient,
    HttpResponse,
    RecordingTransport,
    parse_app_view,
    parse_rotate_key_response,
)


class AppAuthTest(unittest.TestCase):
    def test_p3_3_rotate_key_request(self) -> None:
        from config import from_env

        config = from_env({"MW_ADMIN_TOKEN": "admin-secret"})
        transport = RecordingTransport(HttpResponse(200, {}, b'{"api_key":"mw_new"}'))
        client = HttpClient(config, transport)

        response = client.rotate_app_key("app-1")
        key = parse_rotate_key_response(response)

        request = transport.requests[0]
        self.assertEqual(request.method, "POST")
        self.assertEqual(request.url, "http://127.0.0.1:8080/admin/apps/app-1/rotate-key")
        self.assertEqual(request.headers["Authorization"], "Bearer admin-secret")
        self.assertEqual(key.reveal(), "mw_new")

    def test_p3_3_get_app_request_and_response(self) -> None:
        transport = RecordingTransport(
            HttpResponse(
                200,
                {},
                json.dumps({"app_id": "app-1", "name": "counter-app", "status": "active"}).encode(),
            )
        )
        client = AppClient("http://127.0.0.1:8080", "app-1", Secret("mw_secret"), transport)
        response = client.get_app()
        app = parse_app_view(response)

        request = transport.requests[0]
        self.assertEqual(request.method, "GET")
        self.assertEqual(request.url, "http://127.0.0.1:8080/api/v1/apps/app-1")
        self.assertEqual(request.headers["Authorization"], "Bearer mw_secret")
        self.assertEqual(app.app_id, "app-1")
        self.assertEqual(app.status, "active")

    def test_p3_4_update_tools_request(self) -> None:
        transport = RecordingTransport(
            HttpResponse(
                200,
                {},
                json.dumps({"app_id": "app-1", "name": "counter-app", "status": "active"}).encode(),
            )
        )
        client = AppClient("http://127.0.0.1:8080", "app-1", Secret("mw_secret"), transport)
        tools = [
            {
                "name": "counter_add",
                "description": "add",
                "schema": {"type": "object"},
                "timeout_ms": 10000,
            }
        ]
        response = client.update_tools("counter-app", tools)
        app = parse_app_view(response)

        request = transport.requests[0]
        self.assertEqual(request.method, "POST")
        self.assertEqual(request.url, "http://127.0.0.1:8080/api/v1/apps")
        self.assertEqual(request.headers["Authorization"], "Bearer mw_secret")
        self.assertEqual(request.json_body()["name"], "counter-app")
        self.assertEqual(request.json_body()["tools"], tools)
        self.assertEqual(app.app_id, "app-1")

    def test_p3_3_get_app_rejects_bad_key(self) -> None:
        transport = RecordingTransport(
            HttpResponse(
                401,
                {},
                json.dumps({"error": {"code": "UNAUTHORIZED", "message": "bad key"}}).encode(),
            )
        )
        client = AppClient("http://127.0.0.1:8080", "app-1", Secret("bad"), transport)
        with self.assertRaises(Exception) as ctx:
            parse_app_view(client.get_app())
        self.assertEqual(getattr(ctx.exception, "code", ""), "UNAUTHORIZED")


if __name__ == "__main__":
    unittest.main()
