---
title: "Diagnostics and maintenance"
status: published
description: "Run doctor / describe / selfcheck, and validate, migrate or maintain the control plane."
updated: 2026-11-21
---

## doctor / describe / selfcheck

### 1. doctor

```sh
levier doctor
```

Checks:

```text
Control plane YAML
HTTP reachability
Database
Migrations
daemon
Optional App credentials
```

Outputs `PASS` / `FAIL` per item; failed items include a `fix`.

```sh
levier doctor --server http://127.0.0.1:8080
levier doctor --config ./levier.yaml
levier doctor --json
```

### 2. describe

```sh
levier describe
```

Reads `/meta` and displays:

```text
service / version / commit
api / ws protocol
runtime types
limits
endpoints
```

Use it to confirm control plane capabilities and limits.

### 3. selfcheck

```sh
levier selfcheck
```

Checks hop by hop:

```text
config
http /healthz
database
migrations
http /readyz
service /meta
service /status
daemon registered
```

> [!OUTPUT]
> ```text
> selfcheck: PASS
> ```

Deep check:

```sh
levier selfcheck --deep
```

Performs active daemon / WebSocket / tool / MCP checks.

### 4. Reading the results

```text
PASS    this hop is normal
FAIL    this hop failed; later hops may fail as a result
fix     the suggested fix command or checkpoint
```

Start fixing from the first `FAIL`; do not change several places at once.

### 5. Common failures

| Hop | Check first |
|---|---|
| config | YAML path and syntax |
| http | Whether the control plane is running, port |
| database | PostgreSQL, DSN |
| migrations | `levier migrate status` |
| daemon | `levier runtime list` |
| App | Credentials path and `app check` |

## config / migrate / maintenance

### 1. config check

```sh
levier config check --config ./levier.yaml
```

Production mode:

```sh
levier config check --config ./levier.yaml --production
```

Rejects example default tokens / passwords.

### 2. config print

```sh
levier config print --config ./levier.yaml
```

Prints the redacted configuration. Secrets are not output.

### 3. migrate

```sh
levier migrate status --config ./levier.yaml
levier migrate up --config ./levier.yaml
```

`status` shows the current version, applied migrations and the pending list.

### 4. maintenance

```sh
levier maintenance status --config ./levier.yaml
levier maintenance prune --config ./levier.yaml
```

`prune` is dry-run by default; it deletes only after confirmation:

```sh
levier maintenance prune --config ./levier.yaml --yes
```

The unit of cleanup is terminal runs:

```text
Only delete runs in completed / failed / cancelled / timeout
that exceed retention.run_days
Do not delete queued / assigned / running
```

### 5. retention configuration

```yaml
retention:
  enabled: false
  run_days: 30
  session_days: 7
  batch_size: 1000
```

Corresponding environment variables:

```text
LEVIER_RETENTION_ENABLED
LEVIER_RETENTION_RUN_DAYS
LEVIER_RETENTION_SESSION_DAYS
LEVIER_RETENTION_BATCH_SIZE
```

### 6. Order

```text
config check
  → migrate up
  → maintenance status
  → maintenance prune --yes (after confirmation)
```

## Next steps

- Stack lifecycle: [Stack lifecycle](levier-status.md)
- Operations: [Operations view](levier-ops.md)
- Remote control plane: [Remote control plane](levier-remote.md)
