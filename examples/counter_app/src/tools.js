/**
 * Middleware — Counter App local tool dispatcher.
 *
 * The App receives name + arguments_json from local_tool_call and returns a
 * structured success/error for local_tool_result.
 *
 * Design references:
 * - docs/DESIGN.md §5.3 工具调用三分支
 * - docs/DESIGN.md §4.5 错误模型
 * - proto/middleware/runtime/v1/runtime.proto ErrorInfo
 * - internal/gateway/invoker.go local error mapping
 *
 * Copyright 2026 The Middleware Authors.
 */

import { Counter } from "./counter.js";

export const TOOL_ERRORS = Object.freeze({
  notFound: { code: "TOOL_NOT_FOUND", message: "tool not found", retryable: false },
  exec: { code: "TOOL_EXEC_ERROR", message: "tool execution failed", retryable: false },
});

function execError(message) {
  return {
    ok: false,
    error: {
      ...TOOL_ERRORS.exec,
      message,
    },
  };
}

function parseArguments(argumentsJson) {
  if (argumentsJson === undefined || argumentsJson === null || argumentsJson.trim() === "") {
    return {};
  }
  const parsed = JSON.parse(argumentsJson);
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new TypeError("arguments_json must be a JSON object");
  }
  return parsed;
}

export function createToolDispatcher(counter = new Counter()) {
  const handlers = {
    counter_add(args) {
      if (!Object.hasOwn(args, "delta")) {
        throw new TypeError("counter_add requires delta");
      }
      if (!Number.isInteger(args.delta)) {
        throw new TypeError("counter_add.delta must be an integer");
      }
      return { value: counter.add(args.delta) };
    },
    counter_get() {
      return { value: counter.get() };
    },
    counter_reset() {
      return { value: counter.reset() };
    },
  };

  return function dispatch(toolName, argumentsJson) {
    const handler = handlers[toolName];
    if (!handler) {
      return {
        ok: false,
        error: { ...TOOL_ERRORS.notFound, message: `unknown tool ${toolName}` },
      };
    }

    let args;
    try {
      args = parseArguments(argumentsJson);
    } catch (error) {
      return execError(`invalid arguments_json: ${error.message}`);
    }

    try {
      const result = handler(args);
      return { ok: true, resultJson: JSON.stringify(result) };
    } catch (error) {
      return execError(error.message || "tool execution failed");
    }
  };
}
