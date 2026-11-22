---
title: Operations view
status: published
description: ops runs, ops daemons and ops tasks.
updated: 2026-11-21
---

## 1. ops runs

```sh
levier ops runs list
```

Filter:

```sh
levier ops runs list --status failed,timeout --limit 50
levier ops runs list --app-id app_01... --cursor ...
```

Single run:

```sh
levier ops runs show --run-id 01J...
```

Cancel:

```sh
levier ops runs cancel --run-id 01J... --reason "stuck" --yes
```

## 2. ops daemons

```sh
levier ops daemons
```

Shows Runtime / daemon connections, capacity, running count and heartbeat.

Difference from `levier runtime list`:

```text
runtime list   scheduling-oriented view
ops daemons    administrator / diagnostic view
```

## 3. ops tasks

```sh
levier ops tasks
```

Shows `DEAD` workflow tasks by default. Filter by status, type or Run:

```sh
levier ops tasks --status DEAD --limit 50
levier ops tasks --run-id 01J...
```

## 4. Permissions

`ops` is an admin entry point and requires an admin token:

```text
--admin-token
LEVIER_ADMIN_TOKEN
token explicitly stored in the profile
```

Do not put it in the App's runtime environment.

## 5. JSON

```sh
levier ops runs list --json
levier ops daemons --json
levier ops tasks --json
```

## 6. Typical usage

| Scenario | Command |
|---|---|
| Find all failed runs | `ops runs list --status failed` |
| View runs for an App | `ops runs list --app-id <id>` |
| View a Runtime | `ops daemons` |
| Look up dead-letter tasks | `ops tasks` |
| Force-cancel a stuck run | `ops runs cancel --run-id <id> --yes` |

## Next steps

- Run details: [Inspect and cancel runs](levier-run.md)
- Runtime list: [Runtime list](levier-runtime-list.md)
- Maintenance and migrations: [config / migrate / maintenance](levier-doctor.md)
