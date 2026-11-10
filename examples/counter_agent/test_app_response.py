"""Middleware — Counter App response parsing tests (P2.3).

Copyright 2026 The Middleware Authors.
"""

from __future__ import annotations

import json
import unittest

from http_client import (
    AppRegistration,
    HttpError,
    HttpResponse,
    parse_create_app_response,
)


def response(status: int, payload: dict) -> HttpResponse:
    return HttpResponse(
        status=status,
        headers={"Content-Type": "application/json"},
        body=json.dumps(payload).encode("utf-8"),
    )


class AppResponseTest(unittest.TestCase):
    def test_p2_3_parses_created_app(self) -> None:
        result = parse_create_app_response(
            response(
                201,
                {
                    "app": {"app_id": "app-1", "name": "retrofit-counter"},
                    "api_key": "mw_secret",
                },
            )
        )
        self.assertIsInstance(result, AppRegistration)
        self.assertEqual(result.app_id, "app-1")
        self.assertEqual(result.name, "retrofit-counter")
        self.assertEqual(result.api_key.reveal(), "mw_secret")
        self.assertTrue(result.has_api_key)
        self.assertNotIn("mw_secret", repr(result))

    def test_p2_3_duplicate_app_has_no_key(self) -> None:
        result = parse_create_app_response(
            response(
                200,
                {
                    "app": {"app_id": "app-1", "name": "retrofit-counter"},
                    "api_key": "",
                },
            )
        )
        self.assertFalse(result.has_api_key)

    def test_p2_3_missing_app_id_fails(self) -> None:
        with self.assertRaises(HttpError) as ctx:
            parse_create_app_response(response(201, {"app": {}, "api_key": "x"}))
        self.assertEqual(ctx.exception.code, "BAD_RESPONSE")

    def test_p2_3_error_envelope(self) -> None:
        with self.assertRaises(HttpError) as ctx:
            parse_create_app_response(
                response(
                    401,
                    {
                        "error": {
                            "code": "UNAUTHORIZED",
                            "message": "missing token",
                            "retryable": False,
                        }
                    },
                )
            )
        self.assertEqual(ctx.exception.status, 401)
        self.assertEqual(ctx.exception.code, "UNAUTHORIZED")
        self.assertEqual(ctx.exception.message, "missing token")

    def test_p2_3_invalid_json_fails(self) -> None:
        with self.assertRaises(HttpError) as ctx:
            parse_create_app_response(HttpResponse(201, {}, b"{"))
        self.assertEqual(ctx.exception.code, "BAD_RESPONSE")


if __name__ == "__main__":
    unittest.main()
