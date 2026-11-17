"""Levier — RuntimeChannel cross-language golden test.

Parses the shared contract fixtures with the generated Python stubs and
checks that the canonical proto JSON round-trips without loss.

Copyright 2026 The Levier Authors.
"""

import json
import unittest
from pathlib import Path

from google.protobuf import json_format

from middleware.runtime.v1 import runtime_pb2


class ContractGoldenTest(unittest.TestCase):
    def test_envelopes_round_trip(self) -> None:
        path = Path(__file__).resolve().parent / "envelopes.json"
        fixtures = json.loads(path.read_text(encoding="utf-8"))

        for index, raw in enumerate(fixtures["messages"]):
            message = runtime_pb2.Envelope()
            json_format.Parse(json.dumps(raw), message)
            got = json_format.MessageToDict(
                message,
                preserving_proto_field_name=True,
            )
            self.assertEqual(got, raw, f"fixture {index} round-trip mismatch")


if __name__ == "__main__":
    unittest.main()
