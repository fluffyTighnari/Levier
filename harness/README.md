<!-- Middleware — stripped-down Python harness. -->

# Middleware Harness

The harness is the execution-side daemon for the middleware control plane.
It owns the RuntimeChannel client, ReAct loop, LLM adapters, tool bridge,
MCP provider, and checkpointing.

P3-23 provides:

- configuration loading with `HARNESS_*` overrides
- JSON/text structured logging
- health state snapshot
- a start/stop process skeleton

## Run

```sh
PYTHONPATH=harness/src:harness/src/gen python -m middleware_harness \
  --config harness/config.example.yaml --check
```

or start the skeleton:

```sh
PYTHONPATH=harness/src:harness/src/gen python -m middleware_harness \
  --config harness/config.example.yaml
```

RuntimeChannel, ReAct, LLM, tools, and MCP are added in P3-24 onward.
