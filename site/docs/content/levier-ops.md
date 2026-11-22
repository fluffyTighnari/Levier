---
title: ops 运维视图
status: published
description: runs、daemons、tasks。
updated: 2026-11-21
---

## 1. ops runs

```sh
levier ops runs list
```

过滤：

```sh
levier ops runs list --status failed,timeout --limit 50
levier ops runs list --app-id app_01... --cursor ...
```

单条：

```sh
levier ops runs show --run-id 01J...
```

取消：

```sh
levier ops runs cancel --run-id 01J... --reason "stuck" --yes
```

## 2. ops daemons

```sh
levier ops daemons
```

显示 Runtime / daemon 的连接、容量、运行数与心跳。

与 `levier runtime list` 的区别：

```text
runtime list   面向调度视图
ops daemons    面向管理员/诊断视图
```

## 3. ops tasks

```sh
levier ops tasks
```

默认查看 `DEAD` workflow task。可按状态、类型、Run 过滤：

```sh
levier ops tasks --status DEAD --limit 50
levier ops tasks --run-id 01J...
```

## 4. 权限

`ops` 是管理员入口，需要 admin token：

```text
--admin-token
LEVIER_ADMIN_TOKEN
profile 中显式保存的 token
```

不要把它放进 App 的运行环境。

## 5. JSON

```sh
levier ops runs list --json
levier ops daemons --json
levier ops tasks --json
```

## 6. 典型用法

| 场景 | 命令 |
|---|---|
| 找出所有失败 Run | `ops runs list --status failed` |
| 看某个 App 的 Run | `ops runs list --app-id <id>` |
| 看某个 Runtime | `ops daemons` |
| 查死信任务 | `ops tasks` |
| 强制取消卡住的 Run | `ops runs cancel --run-id <id> --yes` |

## 下一步

- Run 详情：[查看与取消 Run](levier-run.md)
- Runtime 列表：[Runtime 列表](levier-runtime-list.md)
- 维护与迁移：[config / migrate / maintenance](levier-doctor.md)
