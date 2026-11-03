<!-- Middleware — repository landing page. -->

# Middleware

Middleware is the control plane and tool/event mediation layer for agent
runs executed by a stripped-down Python harness.

- Go 1.26 single binary for Register / Control / Gateway / Connection.
- PostgreSQL 16+ for RunStore and the RunTaskQueue.
- Python 3.12 harness for ReAct, LLM adapters, tools, MCP, and checkpoints.
- Design baseline: `docs/DESIGN.md` v0.2.
- Execution plan: `docs/IMPLEMENTATION_PLAN_V2.md`.

## Quick start

```sh
make build
make test
make lint
```

The current binary is an empty lifecycle skeleton; business packages are
introduced step by step by the V2 plan.
