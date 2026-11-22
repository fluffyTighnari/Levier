---
title: "Deploy and capacity"
status: published
description: "Inspect instances and capacity, and deploy a remote runtime with tokens and TLS."
updated: 2026-11-21
---

## Instances and capacity

### 1. runtime list

```sh
levier runtime list
```

> [!OUTPUT]
> ```text
> runtime    instance         status    capacity  running  available  heartbeat
> my-runtime daemon-local-1   healthy   4         0        4          ...
> ```

For field and scheduling semantics, see [Runtime list](levier-runtime-list.md).

### 2. Multiple instances

Multiple instances can run under the same `runtime_name`:

```sh
levier runtime start --name my-runtime
levier runtime start --name my-runtime --token ...
```

The control plane schedules by capacity; the instance ID distinguishes connections.

### 3. Capacity

`RuntimeConfig` can set:

```python
RuntimeConfig(
    runtime_name="my-runtime",
    instance_id="daemon-local-1",
    middleware=MiddlewareConfig("127.0.0.1:9090", auth_token="..."),
    max_concurrent_runs=4,
)
```

Or the environment variable:

```text
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS
```

### 4. Heartbeats

Instances that have not sent a heartbeat for a long time are excluded from scheduling. Check:

```sh
levier runtime list
levier logs
```

### 5. Common issues

| Symptom | Check first |
|---|---|
| New instance does not appear | token / gRPC address |
| Stuck in queued | All instances are at capacity |
| Duplicate instances | Is instance_id unique? |
| Stale heartbeat | Process, network, control plane |

## Remote deployment and YAML

### 1. Remote deployment

```sh
levier runtime start \
  --grpc levier.example.com:9090 \
  --token <runtime-token> \
  --name my-runtime
```

Or environment variables:

```sh
export LEVIER_MIDDLEWARE_ADDRESS=levier.example.com:9090
export LEVIER_RUNTIME_TOKEN=...
export LEVIER_RUNTIME_NAME=my-runtime
levier runtime start
```

### 2. TLS

```sh
export LEVIER_MIDDLEWARE_TLS=true
```

Certificates and hostnames must match. Do not enable TLS when the control plane gRPC does not have TLS enabled.

### 3. YAML

When no YAML is passed, `Runtime.start()` infers the configuration from the profile / environment. To configure explicitly:

```yaml
middleware:
  address: "levier.example.com:9090"
  auth_token: "..."

runtime:
  name: "my-runtime"
  instance_id: "daemon-remote-1"
  type: "normal"
  max_concurrent_runs: 4

logging:
  level: "info"
```

```python
Runtime.from_config("runtime.yaml").run(run)
```

### 4. Environment variables

```text
LEVIER_RUNTIME_CONFIG
LEVIER_MIDDLEWARE_ADDRESS
LEVIER_RUNTIME_NAME
LEVIER_RUNTIME_INSTANCE_ID
LEVIER_RUNTIME_TOKEN / _FILE
LEVIER_RUNTIME_TYPE
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS
LEVIER_RUNTIME_LABELS
LEVIER_RUNTIME_LOG_LEVEL
```

### 5. Migration

Migrating from an old Runtime to the high-level `levier` API:

```text
Keep the run(ctx) logic
Delete the daemon / channel / heartbeat code
Use ctx.toolbox() instead of a custom ToolRouter
Use ctx.chat() / ctx.invoke_all() instead of a custom model and tool loop
```

Keep only middleware / runtime / logging in the YAML; the MCP address is provided
by the profile or by `LEVIER_MCP_RUNTIME_SOCKET`.

### 6. Troubleshooting

| Symptom | Check first |
|---|---|
| TLS handshake | Certificate, hostname, TLS switch |
| `UNAUTHENTICATED` | runtime token |
| Connection timeout | Firewall, gRPC port |
| Frequent reconnects | Network, token, control plane logs |

## Next steps

- Runtime setup: [Set up a runtime](runtime-start.md)
- Runtime core: [Runtime core workflow](runtime-core.md)
- Remote control plane: [Remote control plane](levier-remote.md)
