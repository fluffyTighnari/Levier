/**
 * Middleware — Counter App tool dispatcher tests.
 *
 * Copyright 2026 The Middleware Authors.
 */

import test from "node:test";
import assert from "node:assert/strict";

import { Counter } from "../src/counter.js";
import { createToolDispatcher } from "../src/tools.js";

test("P1.2 dispatcher counter_add", () => {
  const counter = new Counter();
  const dispatch = createToolDispatcher(counter);
  const result = dispatch("counter_add", JSON.stringify({ delta: 2 }));
  assert.deepEqual(result, { ok: true, resultJson: JSON.stringify({ value: 2 }) });
  assert.equal(counter.get(), 2);
});

test("P1.3 dispatcher counter_get", () => {
  const counter = new Counter(7);
  const dispatch = createToolDispatcher(counter);
  const result = dispatch("counter_get", "{}");
  assert.deepEqual(result, { ok: true, resultJson: JSON.stringify({ value: 7 }) });
  assert.equal(counter.get(), 7);
});

test("P1.4 dispatcher counter_reset", () => {
  const counter = new Counter(9);
  const dispatch = createToolDispatcher(counter);
  const result = dispatch("counter_reset", "{}");
  assert.deepEqual(result, { ok: true, resultJson: JSON.stringify({ value: 0 }) });
  assert.equal(counter.get(), 0);
});

test("P1.5 unknown tool returns TOOL_NOT_FOUND", () => {
  const dispatch = createToolDispatcher(new Counter());
  const result = dispatch("counter_missing", "{}");
  assert.equal(result.ok, false);
  assert.equal(result.error.code, "TOOL_NOT_FOUND");
  assert.equal(result.error.retryable, false);
});

test("P1.6 invalid arguments return structured errors", () => {
  const dispatch = createToolDispatcher(new Counter());

  const malformed = dispatch("counter_add", "{");
  assert.equal(malformed.ok, false);
  assert.equal(malformed.error.code, "TOOL_EXEC_ERROR");
  assert.match(malformed.error.message, /invalid arguments_json/);

  const missing = dispatch("counter_add", "{}");
  assert.equal(missing.ok, false);
  assert.equal(missing.error.code, "TOOL_EXEC_ERROR");
  assert.match(missing.error.message, /requires delta/);

  const wrongType = dispatch("counter_add", JSON.stringify({ delta: "2" }));
  assert.equal(wrongType.ok, false);
  assert.equal(wrongType.error.code, "TOOL_EXEC_ERROR");
  assert.match(wrongType.error.message, /must be an integer/);

  const arrayArgs = dispatch("counter_add", "[]");
  assert.equal(arrayArgs.ok, false);
  assert.equal(arrayArgs.error.code, "TOOL_EXEC_ERROR");
  assert.match(arrayArgs.error.message, /must be a JSON object/);
});
