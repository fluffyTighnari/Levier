/**
 * Middleware — Counter App local state tests.
 *
 * Copyright 2026 The Middleware Authors.
 */

import test from "node:test";
import assert from "node:assert/strict";

import { Counter } from "../src/counter.js";

test("P1.1 counter add/get/reset", () => {
  const counter = new Counter();
  assert.equal(counter.get(), 0);
  assert.equal(counter.add(2), 2);
  assert.equal(counter.add(3), 5);
  assert.equal(counter.get(), 5);
  assert.equal(counter.reset(), 0);
  assert.equal(counter.get(), 0);
});

test("P1.1 counter rejects non-integer values", () => {
  const counter = new Counter();
  assert.throws(() => counter.add("2"), /integer/);
  assert.throws(() => new Counter(1.5), /integer/);
});
