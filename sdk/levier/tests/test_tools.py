"""Tests for the small `levier` authoring surface."""

from __future__ import annotations

import unittest

from levier import tool
from levier.tools import schema_from_function


class ToolDecoratorTest(unittest.TestCase):
    def test_infers_name_description_and_schema(self) -> None:
        @tool
        def counter_add(delta: int, label: str = "x") -> dict:
            """Add delta to the counter."""
            return {"value": delta}

        self.assertEqual(counter_add.name, "counter_add")
        self.assertEqual(counter_add.description, "Add delta to the counter.")
        self.assertEqual(
            counter_add.schema,
            {
                "type": "object",
                "properties": {
                    "delta": {"type": "integer"},
                    "label": {"type": "string", "default": "x"},
                },
                "additionalProperties": False,
                "required": ["delta"],
            },
        )

    def test_name_override_and_direct_call(self) -> None:
        @tool("add")
        def counter_add(delta: int) -> dict:
            return {"value": delta}

        self.assertEqual(counter_add.name, "add")
        self.assertEqual(counter_add.invoke({"delta": 2}), {"value": 2})
        self.assertEqual(counter_add(delta=3), {"value": 3})

    def test_optional_and_collection_types(self) -> None:
        def search(query: str, tags: list[str] | None = None) -> dict:
            return {}

        schema = schema_from_function(search)
        self.assertEqual(schema["properties"]["query"], {"type": "string"})
        self.assertEqual(
            schema["properties"]["tags"],
            {"type": "array", "items": {"type": "string"}, "default": None},
        )


if __name__ == "__main__":
    unittest.main()
