---
title: Runtime list
status: published
description: Instances, capacity and heartbeats.
updated: 2026-11-21
---

## 1. runtime list

```sh
levier runtime list
```

> [!OUTPUT]
> ```text
> runtime    instance         status    capacity  running  available  heartbeat
> react1     daemon-local-1   healthy   4         0        4          ...
> echo1      daemon-local-2   healthy   4         0        4          ...
> ```

Requires an admin token (profile or `LEVIER_ADMIN_TOKEN`).

## 2. Fields

| Field | Meaning |
|---|---|
| `runtime` | Route name |
| `instance` | Instance ID |
| `status` | healthy / other |
| `capacity` | Maximum concurrency |
| `running` | Current run count |
| `available` | Remaining capacity |
| `heartbeat` | Latest heartbeat |

## 3. JSON

```sh
levier runtime list --json
```

## 4. Scheduling judgement

```text
available > 0       can accept new runs
status != healthy   not scheduled
heartbeat too old   may be disconnected
no Runtime          runs stay queued
```

## 5. Common problems

| Symptom | Check first |
|---|---|
| `no connected runtimes` | Whether `levier up` / `runtime start` is running |
| Capacity full | Wait for runs to finish, or add instances / capacity |
| Heartbeat too old | Runtime process, network, token |
| Wrong name | Whether `--runtime` matches |

## Next steps

- Start a custom Runtime: [runtime start](runtime-start.md)
- Instances and capacity: [Instances and capacity](runtime-remote.md)
- Status overview: [Status](levier-status.md)
