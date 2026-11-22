---
title: Runtime 列表
status: published
description: runtime list 的实例、容量与心跳。
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

需要 admin token（profile 或 `LEVIER_ADMIN_TOKEN`）。

## 2. 字段

| 字段 | 含义 |
|---|---|
| `runtime` | 路由名 |
| `instance` | 实例 ID |
| `status` | healthy / 其他 |
| `capacity` | 最大并发 |
| `running` | 当前 Run 数 |
| `available` | 剩余容量 |
| `heartbeat` | 最近心跳 |

## 3. JSON

```sh
levier runtime list --json
```

## 4. 调度判断

```text
available > 0       可以接收新 Run
status != healthy   不参与调度
heartbeat 过旧      可能已断线
没有 Runtime        Run 会一直 queued
```

## 5. 常见问题

| 现象 | 先检查 |
|---|---|
| `no connected runtimes` | `levier up` / `runtime start` 是否运行 |
| 容量已满 | 等 Run 结束，或增加实例 / 容量 |
| 心跳过旧 | Runtime 进程、网络、token |
| 名称不对 | `--runtime` 是否匹配 |

## 下一步

- 启动自定义 Runtime：[runtime start](runtime-start.md)
- 实例与容量：[实例与容量](runtime-remote.md)
- 状态总览：[状态](levier-status.md)
