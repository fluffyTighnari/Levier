from __future__ import annotations

import json
import unittest

from levier_app.errors import HTTPError
from levier_app.http import HttpResponse, LevierClient


class FakeTransport:
    def __init__(self, *responses: HttpResponse) -> None:
        self.responses = list(responses)
        self.requests = []

    def send(self, request):
        self.requests.append(request)
        if not self.responses:
            raise AssertionError("unexpected request")
        return self.responses.pop(0)


def response(status: int, payload: dict) -> HttpResponse:
    return HttpResponse(status=status, headers={}, body=json.dumps(payload).encode())


class HttpTest(unittest.TestCase):
    def test_update_tools_and_ws_token(self) -> None:
        transport = FakeTransport(
            response(200, {"app_id": "app-1", "name": "demo", "status": "active"}),
            response(200, {
                "ws_url": "/api/v1/ws",
                "ws_token": "token",
                "expires_in": 60,
            }),
        )
        client = LevierClient("http://levier:8080/", "app-1", "secret", transport)

        self.assertEqual(client.get_app()["status"], "active")
        ws = client.create_ws_token()
        self.assertEqual(ws.ws_url, "/api/v1/ws")
        self.assertEqual(ws.token.reveal(), "token")
        self.assertNotIn("token", repr(ws))
        self.assertEqual(ws.expires_in, 60)
        self.assertEqual(transport.requests[0].headers["Authorization"], "Bearer secret")
        self.assertEqual(transport.requests[0].url, "http://levier:8080/api/v1/apps/app-1")

    def test_update_tools_sends_full_manifest(self) -> None:
        transport = FakeTransport(response(200, {"app_id": "app-1"}))
        client = LevierClient("http://levier:8080", "app-1", "secret", transport)
        client.update_tools("demo", [{
            "name": "counter_add",
            "description": "Add",
            "schema": {"type": "object"},
            "timeout_ms": 10000,
        }])
        request = transport.requests[0]
        self.assertEqual(request.url, "http://levier:8080/api/v1/apps")
        self.assertEqual(request.headers["Authorization"], "Bearer secret")
        payload = json.loads(request.body)
        self.assertEqual(payload["name"], "demo")
        self.assertFalse(payload["multiagent"])
        self.assertEqual(payload["tools"][0]["name"], "counter_add")

    def test_http_error_is_normalized(self) -> None:
        transport = FakeTransport(response(401, {
            "error": {"code": "UNAUTHORIZED", "message": "bad key"},
        }))
        client = LevierClient("http://levier:8080", "app-1", "bad", transport)
        with self.assertRaises(HTTPError) as raised:
            client.get_app()
        self.assertEqual(raised.exception.status, 401)
        self.assertEqual(raised.exception.code, "UNAUTHORIZED")


if __name__ == "__main__":
    unittest.main()
