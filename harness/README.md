<!-- Middleware — stripped-down Python harness. -->

# Middleware Harness

The harness is the execution-side daemon for the middleware control plane.
It owns the RuntimeChannel client, ReAct loop, LLM adapters, tool bridge,
MCP provider, and checkpointing.

P0-05 creates only the package skeleton. P3 implements the runtime.
