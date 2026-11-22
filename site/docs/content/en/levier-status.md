---
title: "Stack lifecycle"
status: published
description: "Check status, read logs and stop the stack."
updated: 2026-11-21
---

## Status

### 1. levier status

```sh
levier status
```

> [!OUTPUT]
> ```text
> server      http://127.0.0.1:8080
> status      ok
> version     ...
> uptime      ...
> health      ok
> ready       ok
> database    up
> daemons     connected=2 healthy=2 capacity=8 used=0 available=8
> apps        registered=1 websocket=1
> runs        total=3 active=0 ...
> workflow    total=... pending=... leased=... dead=...
> ```

### 2. Fields

| Field | What to check |
|---|---|
| `status` | Aggregate status |
| `health` | Process liveness |
| `ready` | Dependencies (PostgreSQL and others) ready |
| `database` | Database connection |
| `daemons` | Runtime count, capacity, usage |
| `apps` | Registered Apps and WebSocket connections |
| `runs` | Total and active run counts |
| `workflow` | Task queue and dead letters |

### 3. JSON

```sh
levier status --json
```

Suited to monitoring and scripts. Do not parse the display text.

### 4. Judging health

```text
health=ok + ready=ok + database=up
```

is the minimum condition for continuing operations.

When `daemons connected=0`:

- Runs stay queued until a Runtime comes online;
- Check `levier runtime list` and `levier logs`.

### 5. Remote status

```sh
levier status --server https://levier.example.com
```

Requires network reachability; authentication comes from the profile / token.

## Stop

### 1. levier down

```sh
levier down
```

Stops the local stack. The database volume is kept by default, so the next `levier up` can continue using it.

### 2. Delete data volumes

```sh
levier down --volumes
```

Deletes the named volumes; local data such as runs, Apps and events is lost with them. Use only when you are sure it is no longer needed.

### 3. Specify a Compose file

```sh
levier down --file ./docker-compose.yml
```

### 4. Restart

```sh
levier down
levier up
```

If only the configuration changed, prefer:

```sh
levier up --build
```

### 5. After stopping

```sh
levier status
```

You should see a connection failure or unreachable. Continue after `levier up` is ready.

## Logs and processes

### 1. levier logs

```sh
levier logs
```

View local Compose logs.

Follow continuously:

```sh
levier logs --follow
```

Exit follow: `Ctrl+C`.

### 2. levier ps

```sh
levier ps
```

View Compose service status. Confirm whether `levier`, `postgres`, `mcp-runtime`,
`react-runtime` and `echo-runtime` are running.

### 3. Specify files

```sh
levier logs --file ./docker-compose.yml
levier ps --file ./docker-compose.yml
```

### 4. Locating logs

| Symptom | Where to look |
|---|---|
| Control plane fails to start | `levier` container logs |
| Database not ready | `postgres` container logs |
| Runtime not connected | The corresponding runtime container logs |
| MCP tool failure | `mcp-runtime` logs |
| Run stuck | `levier` + target runtime logs |

When you need to view events by Run:

```sh
levier run show <run_id> --events
```

### 5. Advanced

`levier stack logs` / `levier stack ps` wrap Compose directly and are kept for existing scripts.
For daily use, `levier logs` / `levier ps` are enough.

## Next steps

- Install and start: [Install, connect and start](levier-install.md)
- Diagnostics: [Diagnostics and maintenance](levier-doctor.md)
- Operations: [Operations view](levier-ops.md)
