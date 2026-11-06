<!-- Middleware — stripped-down Python harness. -->

# Middleware Harness

The harness is the execution-side daemon for the middleware control plane.
It owns the RuntimeChannel client, ReAct loop, LLM adapters, tool bridge,
MCP client, and checkpointing. The MCP runtime is a separate process under
`mcp_runtime/`; it is not part of this package.

P3-23 provides:

- configuration loading with `HARNESS_*` overrides
- JSON/text structured logging
- health state snapshot
- a start/stop process skeleton

## Environment

The harness owns its own virtual environment. From `harness/`:

```sh
uv sync --locked
```

or with the standard library:

```sh
python3 -m venv .venv
.venv/bin/python -m ensurepip
.venv/bin/python -m pip install -e .
```

## Run

From `harness/`:

```sh
PYTHONPATH=src:src/gen .venv/bin/python -m middleware_harness \
  --config config.example.yaml --check
```

or start the skeleton:

```sh
PYTHONPATH=src:src/gen .venv/bin/python -m middleware_harness \
  --config config.example.yaml
```

RuntimeChannel, ReAct, LLM, and tools are added in P3-24 onward. The
standalone MCP runtime is added in P3-29.
