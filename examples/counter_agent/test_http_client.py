"""Middleware — Counter App HTTP request construction tests (P2.2).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import unittest

from config import from_env
from http_client import HttpClient, HttpResponse, RecordingTransport


class HttpRequestTest(unittest.TestCase):
    def test_p2_2_builds_admin_app_request(self) -> None:
        config = from_env(
            {
                "MW_URL": "http://127.0.0.1:8080/",
                "MW_ADMIN_TOKEN": "admin-secret",
                "MW_APP_NAME": "retrofit-counter",
            }
        )
        transport = RecordingTransport(HttpResponse(status=201, headers={}, body=b"{}"))
        client = HttpClient(config, transport)

        response = client.create_app()

        self.assertEqual(response.status, 201)
        self.assertEqual(len(transport.requests), 1)
        request = transport.requests[0]
        self.assertEqual(request.method, "POST")
        self.assertEqual(request.url, "http://127.0.0.1:8080/admin/apps")
        self.assertEqual(request.headers["Authorization"], "Bearer admin-secret")
        self.assertEqual(request.headers["Content-Type"], "application/json")
        self.assertEqual(request.headers["Accept"], "application/json")
        self.assertEqual(
            request.json_body(),
            {"name": "retrofit-counter", "multiagent": False},
        )
        self.assertEqual(request.body, b'{"name":"retrofit-counter","multiagent":false}')


if __name__ == "__main__":
    unittest.main()
