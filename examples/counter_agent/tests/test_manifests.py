"""Levier — Counter App manifest shape tests (P1.7).

Copyright 2026 The Levier Authors.
"""

from __future__ import annotations

import json
import unittest

from tools import TOOL_DEFINITIONS, tool_manifests


class ManifestTest(unittest.TestCase):
    def test_p1_7_manifest_dual_shape(self) -> None:
        manifests = tool_manifests()
        self.assertEqual(len(manifests), len(TOOL_DEFINITIONS))

        for definition, manifest in zip(TOOL_DEFINITIONS, manifests):
            self.assertEqual(manifest["name"], definition["name"])
            self.assertEqual(manifest["description"], definition["description"])
            self.assertNotIn("timeout_ms", manifest)
            # Registration shape: schema object.
            self.assertEqual(definition["schema"]["type"], "object")
            # TaskAssign shape: input_schema_json string.
            self.assertEqual(
                json.loads(manifest["input_schema_json"]),
                definition["schema"],
            )
            json.dumps(definition)
            json.dumps(manifest)

    def test_p1_7_manifest_is_isolated(self) -> None:
        first = tool_manifests()
        first[0]["name"] = "mutated"
        second = tool_manifests()
        self.assertEqual(second[0]["name"], "counter_add")


if __name__ == "__main__":
    unittest.main()
