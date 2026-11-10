/**
 * Middleware — Counter App tool manifest tests.
 *
 * Copyright 2026 The Middleware Authors.
 */

import test from "node:test";
import assert from "node:assert/strict";

import { toolDefinitions, toolManifests } from "../src/manifests.js";

test("P1.7 tool manifests", () => {
  const definitions = toolDefinitions();
  assert.deepEqual(
    definitions.map((definition) => definition.name),
    ["counter_add", "counter_get", "counter_reset"],
  );
  for (const definition of definitions) {
    assert.equal(typeof definition.schema, "object");
    assert.equal(definition.schema.type, "object");
  }
  assert.deepEqual(definitions[0].schema.required, ["delta"]);

  const manifests = toolManifests();
  assert.equal(manifests.length, 3);
  for (const manifest of manifests) {
    const schema = JSON.parse(manifest.input_schema_json);
    assert.equal(schema.type, "object");
    assert.equal(typeof manifest.description, "string");
  }
  assert.deepEqual(JSON.parse(manifests[0].input_schema_json).required, ["delta"]);
});

test("P1.7 manifests are isolated copies", () => {
  const first = toolDefinitions();
  first[0].schema.required.push("delta");
  first[0].schema.type = "mutated";

  const second = toolDefinitions();
  assert.deepEqual(second[0].schema.required, ["delta"]);
  assert.equal(second[0].schema.type, "object");
});
