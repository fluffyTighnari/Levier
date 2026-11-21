# React Runtime

React is the built-in ReAct Runtime and the reference composition layer for
runtime authors. The package is intentionally thin: it loads configuration,
wires the optional `levier_runtime.react` helper, and serves `run(ctx)` through
the Runtime SDK. Tool access, including MCP, comes from `ctx.toolbox()`.

```text
execution/react_runtime/
  react.example.yaml
  src/levier_react_runtime/
    __init__.py
    __main__.py       # common CLI shell + ReactAgent composition
```

## What lives where

| Layer | Owner |
|---|---|
| RuntimeChannel, lifecycle, events, cancellation, tool transport | `levier-runtime` |
| ReAct loop, provider router, checkpoint codec | optional `levier_runtime.react` / `.llm` |
| Toolbox, local tool bridge, MCP client | Runtime SDK (`ctx.toolbox()`) |
| Shared MCP process | `execution/mcp_runtime` |
| Configuration and process entry point | this package |

The ReAct helper is optional. A custom runtime can use the SDK alone, or import
`ReactAgent` when it wants the same loop.

## Configuration

`react.example.yaml` uses the standard Runtime SDK shape:

```yaml
middleware:
  address: "127.0.0.1:9090"
  auth_token: "change-me"

runtime:
  name: "react1"
  instance_id: "daemon-local-1"
  type: "normal"
  max_concurrent_runs: 4

logging:
  level: "info"
```

The SDK reads `middleware`, `runtime`, and `logging`. MCP is resolved per run by
`ctx.toolbox()` from the profile or `LEVIER_MCP_RUNTIME_SOCKET`; there is no
React-specific MCP configuration.

Use the SDK environment names:

```text
LEVIER_RUNTIME_CONFIG
LEVIER_MIDDLEWARE_ADDRESS
LEVIER_MIDDLEWARE_TLS
LEVIER_RUNTIME_TOKEN
LEVIER_RUNTIME_TOKEN_FILE
LEVIER_RUNTIME_NAME
LEVIER_RUNTIME_INSTANCE_ID
LEVIER_RUNTIME_TYPE
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS
LEVIER_RUNTIME_LABELS
LEVIER_RUNTIME_LOG_LEVEL
```

## Install

From `execution/react_runtime/`:

```sh
uv sync --locked
```

or with the standard library:

```sh
python3 -m venv .venv
.venv/bin/python -m ensurepip
.venv/bin/python -m pip install -e ../agent_core
.venv/bin/python -m pip install -e ../runtime_sdk
.venv/bin/python -m pip install -e .
```

## Run

From `execution/react_runtime/`:

```sh
PYTHONPATH=src:../runtime_sdk/src:../agent_core/src .venv/bin/python -m levier_react_runtime \
  --config react.example.yaml --check
```

```sh
PYTHONPATH=src:../runtime_sdk/src:../agent_core/src .venv/bin/python -m levier_react_runtime \
  --config react.example.yaml
```

CLI:

```text
--config PATH       runtime YAML
--check             validate and exit
--print-config      print redacted effective config
--version           print package version
--log-level LEVEL   override logging level
```

Start the shared MCP Runtime separately when MCP bindings are used:

```sh
cd ../mcp_runtime
PYTHONPATH=src .venv/bin/python -m levier_mcp_runtime --socket /tmp/levier-mcp.sock
```

Or let `levier up` provide the local TCP endpoint through the user profile.

## Tests

```sh
make react-runtime-test
make runtime-sdk-test
```

`runtime-sdk-test` covers the optional ReAct/LLM/tool helpers; the React test
suite covers the thin CLI composition layer.
