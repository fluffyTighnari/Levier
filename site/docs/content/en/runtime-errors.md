---
title: "Errors and checkpoints"
status: published
description: "Handle tool and model errors, cancellation and checkpoint state."
updated: 2026-11-21
---

## Errors and cancellation

### 1. ctx.error

```python
await ctx.error(
    "MY_ERROR",
    "something failed",
    retryable=False,
)
```

Parameters:

| Parameter | Description |
|---|---|
| `code` | Stable error code |
| `message` | Human-readable message |
| `retryable` | Whether the error is retryable |
| `data` | Optional structured data |

### 2. ToolResult

```python
result = await ctx.call_tool("search", {"query": "levier"})
if not result.ok:
    await ctx.error(result.error_code, result.error_message)
```

`ToolResult` already contains the error code; do not parse the display text.

### 3. Uncaught exceptions

Uncaught exceptions in the Runtime automatically become:

```text
ERROR event
Run failed
```

So you do not need to hand-write try/except in every branch, but key steps should still surface explicit error codes.

### 4. Cancellation

```python
if ctx.cancelled:
    return "cancelled"
```

Or wait:

```python
await ctx.cancel_event.wait()
```

Cancellation is cooperative. Long loops, long tool calls and long model calls should all check it periodically.

### 5. Events

Errors appear in:

```text
ERROR
TOOL_CALL_END
RUN_STATUS
```

Use:

```sh
levier run show <run_id> --events
```

to locate them.

### 6. Returning a terminal state

```python
return "completed"
return "failed"
return "cancelled"
```

The return value is the Run's terminal state; unknown strings are mapped per implementation, so prefer only these three.

## Checkpoint

### 1. Saving

```python
await ctx.checkpoint({"step": step, "messages": messages})
```

`bytes` or a mapping is supported. Requires `ctx.session_id`.

### 2. session

```python
ctx.session_id
ctx.resume_from
```

`resume_from` is the checkpoint state issued by the control plane; it is `None` when absent.

### 3. Resuming

```python
state = ctx.resume_from
if state:
    messages = state["messages"]
```

The checkpoint format is defined by the Runtime itself; the SDK only transports it.

### 4. Boundaries

Current MVP:

```text
checkpoint transport and saving are available
long-term checkpoint resume is not fully implemented
```

Do not treat it as a durable workflow engine; when you need strong recovery, persist to your own business storage.

### 5. Recommendations

- Keep checkpoints as small as possible.
- Do not put secrets in them.
- Version your state structure.
- Be able to rerun from the start on failure rather than relying on checkpoints alone.

## Next steps

- Runtime core: [Runtime core workflow](runtime-core.md)
- Context: [Context](runtime-context.md)
- Toolbox: [Toolbox](runtime-toolbox.md)
