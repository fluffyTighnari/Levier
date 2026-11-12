<!-- Middleware — language-neutral agent core. -->

# Agent Core

Loop, tool orchestration, events, and cancellation shared by daemons.

- Contract: `docs/AGENT_CORE_CONTRACT.md`
- No Middleware / Harness / MCP / protobuf imports.
- Adapters own transport, MW event mapping, tool bridges, and persistence.

## Test

```sh
make agent-core-test
# or
PYTHONPATH=agent_core/src python3 -m unittest discover -s agent_core/tests -v
```

## Install into the harness venv

```sh
cd harness && uv sync --locked
```

`harness/pyproject.toml` depends on `middleware-agent-core` via a local path.
