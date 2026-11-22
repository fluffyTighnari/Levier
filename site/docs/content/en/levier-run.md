---
title: "Runs"
status: published
description: "Submit a run and inspect or cancel it."
updated: 2026-11-21
---

## Submit a run

### 1. levier run

```sh
levier run "hello"
```

Defaults:

```text
Current project App
Default runtime
Default provider / model
Wait for the terminal state
Output the final message
```

### 2. Options

| Option | Description |
|---|---|
| `--runtime` | runtime_name |
| `--provider` / `--model` | Model override |
| `--max-steps` | Maximum ReAct steps |
| `--run-ms` / `--tool-ms` | Run / tool timeout |
| `--no-watch` | Submit only |
| `--with-app` | Start `./app.py` temporarily |
| `--app` | Specify the App file |
| `--python` | Specify Python |
| `--json` | Machine-readable |
| `--credentials` / `--server` | Explicit override |

### 3. --with-app

```sh
levier run "Add 2 and 3." --with-app
```

If an App is already connected, it is reused:

```text
using the already-connected App
```

### 4. --no-watch

```sh
levier run "hello" --no-watch
```

Returns `run_id` and `status`. Then:

```sh
levier run watch <run_id>
levier run show <run_id> --events
```

### 5. JSON

```sh
levier run "hello" --json
```

Contains fields such as `run_id` and `status`; suited to scripts.

### 6. Low-level submit

```sh
levier run submit \
  --runtime react1 \
  --prompt "hello" \
  --watch
```

The low-level command gives explicit control over `--idempotency-key`, `--request-id` and all timeouts.

## Inspect and cancel

### 1. run show

```sh
levier run show <run_id>
```

> [!OUTPUT]
> ```text
> run_id       01J...
> status       completed
> runtime      react1
> attempt      1
> session_id   ...
> trace_id     ...
> created_at   ...
> updated_at   ...
> ```

### 2. --events

```sh
levier run show <run_id> --events
```

```text
[1] RUN_STATUS            daemon
[2] MESSAGE_DELTA         daemon
[3] TOOL_CALL_BEGIN       daemon
[4] TOOL_CALL_END         app
[5] MESSAGE_COMPLETED     daemon
[6] RUN_STATUS            daemon
```

Event types are covered in [Runs and events](app-runs.md).

### 3. run cancel

```sh
levier run cancel <run_id> --yes
```

> [!OUTPUT]
> ```text
> run cancelled: 01J...
> status: cancelled
> ```

In non-interactive environments you must pass `--yes`. Cancellation is cooperative; runs that are already terminal do not roll back.

### 4. JSON

```sh
levier run show <run_id> --json
levier run show <run_id> --events --json
```

### 5. Low-level options

```sh
levier run show <run_id> --credentials ./data/agent/credentials.json
levier run cancel <run_id> --credentials ./data/agent/credentials.json --yes
```

### 6. Common problems

| Symptom | Check first |
|---|---|
| `run not found` | Whether run_id belongs to the current App |
| `UNAUTHORIZED` | Credentials path / App key |
| Stuck in `queued` | Whether `levier runtime list` shows an available Runtime |
| `failed` with no tool events | Look at `ERROR` events and Runtime logs |

## Next steps

- App runs: [Runs](app-runs.md)
- Runtime list: [Runtime list](levier-runtime-list.md)
- Operations: [Operations view](levier-ops.md)
