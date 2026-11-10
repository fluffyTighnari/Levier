/**
 * Middleware — Counter App local tool manifests.
 *
 * Produces the same tool metadata in both contract shapes:
 * - App registration HTTP uses ToolDefinition.schema (object).
 * - Harness TaskAssign uses ToolManifest.input_schema_json (string).
 *
 * Design references:
 * - docs/DESIGN.md §5.3 工具调用三分支
 * - docs/DESIGN.md §4.2 App HTTP API
 * - proto/middleware/runtime/v1/runtime.proto ToolManifest
 * - internal/httpapi/contracts.go ToolDefinition
 *
 * Copyright 2026 The Middleware Authors.
 */

const COUNTER_ADD_SCHEMA = Object.freeze({
  type: "object",
  properties: {
    delta: { type: "integer" },
  },
  required: ["delta"],
  additionalProperties: false,
});

const NO_ARG_SCHEMA = Object.freeze({
  type: "object",
  properties: {},
  additionalProperties: false,
});

const DEFINITIONS = Object.freeze([
  Object.freeze({
    name: "counter_add",
    description: "Add an integer delta to the counter. Use this for every arithmetic change.",
    schema: COUNTER_ADD_SCHEMA,
  }),
  Object.freeze({
    name: "counter_get",
    description: "Read the current counter value.",
    schema: NO_ARG_SCHEMA,
  }),
  Object.freeze({
    name: "counter_reset",
    description: "Reset the counter to zero.",
    schema: NO_ARG_SCHEMA,
  }),
]);

export function toolDefinitions() {
  return DEFINITIONS.map((definition) => ({
    name: definition.name,
    description: definition.description,
    schema: structuredClone(definition.schema),
  }));
}

export function toolManifests() {
  return DEFINITIONS.map((definition) => ({
    name: definition.name,
    description: definition.description,
    input_schema_json: JSON.stringify(definition.schema),
  }));
}
