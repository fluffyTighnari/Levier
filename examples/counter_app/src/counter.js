/**
 * Middleware — Counter App local state.
 *
 * This module is the App-owned part of a local tool. Middleware DESIGN §5.3
 * assigns local tool execution to the App; the Gateway only routes the call.
 *
 * Design references:
 * - docs/DESIGN.md §5.3 工具调用三分支
 * - proto/middleware/runtime/v1/runtime.proto ToolCallRequest
 * - proto/middleware/runtime/v1/runtime.proto ToolCallResult
 *
 * Copyright 2026 The Middleware Authors.
 */

export class Counter {
  #value;

  constructor(initial = 0) {
    if (!Number.isInteger(initial)) {
      throw new TypeError("initial counter value must be an integer");
    }
    this.#value = initial;
  }

  add(delta) {
    if (!Number.isInteger(delta)) {
      throw new TypeError("delta must be an integer");
    }
    this.#value += delta;
    return this.#value;
  }

  get() {
    return this.#value;
  }

  reset() {
    this.#value = 0;
    return this.#value;
  }
}
