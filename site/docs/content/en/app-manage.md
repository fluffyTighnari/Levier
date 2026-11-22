---
title: "App management"
status: published
description: "Self-check, inspect and rotate App keys."
updated: 2026-11-21
---

## Self-check

### 1. app check

```sh
levier app check
```

With explicit credentials:

```sh
levier app check --credentials ./data/agent/credentials.json
```

> [!OUTPUT]
> ```text
> app check: PASS
> ```

On failure it prints `app check: FAIL` and lists each reason.

### 2. Checks

```text
Credentials format and version
File permissions
Server reachability
App key
Whether App status is active
WebSocket token
```

### 3. JSON

```sh
levier app check --json
```

Suitable for scripts or CI. Exit codes follow the global convention: 3 for authentication failure, 4 for network failure.

### 4. Common failures

| Symptom | Action |
|---|---|
| `read credentials` | Wrong path; check `LEVIER_CREDENTIALS` |
| File permission error | `chmod 600 <credentials>` |
| `status is inactive` | Re-run `levier init` or see Manage apps |
| WebSocket token failure | Check the server and the ws_token TTL |
| Server unreachable | `levier status` / `levier doctor` |

### 5. Low-level credentials

`app check` uses an explicit credentials path; day-to-day project work usually does not need it:

```sh
levier app show --credentials ./data/agent/credentials.json
```

## Manage apps

### 1. app list

```sh
levier app list
```

Administrator view; requires an admin token (profile or `LEVIER_ADMIN_TOKEN`).

### 2. app show

```sh
levier app show --credentials ./data/agent/credentials.json
```

> [!OUTPUT]
> ```text
> app_id              app_01...
> name                counter-agent
> status              active
> credentials         ./data/agent/credentials.json
> credential_version  1
> server              http://127.0.0.1:8080
> key_fingerprint     ...
> ```

Only the key fingerprint is shown, never the key itself.

### 3. app rotate-key

```sh
levier app rotate-key \
  --credentials ./data/agent/credentials.json \
  --yes
```

Rotation:

```text
Invalidates the old key immediately
Writes the credentials file back atomically
Keeps a protected .bak backup
```

After rotation, run again:

```sh
levier app check --credentials ./data/agent/credentials.json
```

### 4. Credentials recovery

If the key is lost:

```sh
levier init --rotate
```

Or the low-level form:

```sh
levier app init \
  --name counter-agent \
  --credentials-out ./data/agent/credentials.json \
  --rotate --yes
```

Do not delete the database or the credentials volume to "start over".

### 5. Low-level app init

Use only when you need explicit control over the credentials path:

```sh
levier app init \
  --name counter-agent \
  --credentials-out ./data/agent/credentials.json \
  --mcp-ref echo
```

Arguments:

```text
--name
--credentials-out
--mcp-ref
--rotate
--yes
--force
--dry-run
--json
```

### 6. Permissions

`app list` and `app rotate-key` require an admin token; `app show` / `app check`
use the App's own key. Do not put the admin token in the environment of a long-running App.

## Next steps

- App core: [App core workflow](app-core.md)
- MCP and reconnects: [MCP references and reconnects](app-mcp.md)
- SDK: [SDK reference](sdk.md)
