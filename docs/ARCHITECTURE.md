# Architecture

Levier connects apps to execution runtimes through one control plane. Apps use
the App SDK; runtimes use the Runtime SDK; MCP tools live in their own shared
runtime process.

See `docs/DESIGN.md`, `docs/APP_ONBOARDING.md`, `docs/RUNTIME_ONBOARDING.md`,
and `docs/MCP.md` for the detailed contracts.

## Process view

```text
App             App SDK + WebSocket
Levier      control plane: API, run store, scheduler, gateway, events
React Runtime   built-in ReAct runtime (thin composition layer)
Echo Runtime    minimal run(ctx) reference runtime
MCP Runtime     independent shared MCP process and socket
PostgreSQL      runs, events, sessions, apps, tools, registrations
```

## Control-plane packages

```text
contract        wire contracts and event names
runstore        runs / events / sessions / apps persistence
register        app, tool, MCP, material, and secret registration
workflow        durable run workflow and worker loop
control         scheduling, attempts, leases, event handling, transitions
gateway         local/global tool invocation routing
connection      runtime/app connections and channels
httpapi         HTTP + WebSocket API
observe         logs, metrics, health
cmd/levier      dependency wiring and entry points
```

Dependency direction:

```text
httpapi   -> register / runstore / connection / workflow
control   -> register / runstore / workflow / connection / protocol
gateway   -> register / connection
register  -> runstore
runstore  -> nothing internal
contract  -> nothing internal
```

`tests/checks/check_deps.sh` enforces this direction.

## Execution layout

```text
execution/
  runtime_sdk/     RuntimeChannel, lifecycle, RunContext, events, cancellation
                   optional: react, llm, tools, mcp client
  mcp_runtime/     independent MCP process and socket
  echo_runtime/    minimal built-in runtime / authoring example
  react_runtime/   thin config + composition for ReactAgent
  agent_core/      optional ReAct loop library
```

`runtime_sdk` core has no React, LLM, MCP server, or ToolRouter dependency.
`react_runtime` depends on `agent_core` and the optional SDK helpers.

## Run lifecycle

```text
App POST /runs
  -> handleCreateRun
  -> runstore: RUN_DISPATCH / RUN_TIMEOUT
  -> workflow
  -> control.Handle
      -> register.AssembleTaskMaterials
      -> AssignRun(attempt, lease_epoch)
      -> TaskAssign -> selected Runtime
  -> Runtime events over RuntimeChannel
      -> control.handleEvent
      -> runstore.AppendEvent
      -> appHub.Publish
  -> RUN_STATUS terminal
      -> control.Transition
      -> cancel tasks / publish completion
```

## Tool paths

```text
local    Runtime -> ToolCallRequest -> gateway.Invoker -> AppHub -> App WS
global   Runtime -> ToolCallRequest -> gateway.Invoker -> executor (HTTP/webhook)
MCP      Runtime -> mcp_runtime JSON-RPC -> MCP server
```

- local/global calls pass through Gateway and are recorded in `tool_invocations`.
- MCP calls do not pass through Gateway.
- MCP tools are opened per run from the TaskAssign `mcp_bindings`.

## Runtime boundary

A runtime writes `run(ctx)` and optional loop logic. The Runtime SDK owns:

- channel connection, Hello, heartbeat, reconnect
- Ack and lease_epoch handling
- event sequence numbers and terminal status
- cancellation and checkpoint transport
- automatic `TOOL_CALL_BEGIN` / `TOOL_CALL_END`
- `ctx.error`, `ctx.logger`, and uncaught-exception mapping to failed runs

React/LLM/tools/checkpoint are optional authoring helpers, not the Runtime
core. Echo imports none of them and remains the minimal conformance example.
