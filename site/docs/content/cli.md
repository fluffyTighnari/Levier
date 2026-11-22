---
title: CLI 总览
status: published
description: 主路径、观测、高级、兼容、--json 与退出码。
updated: 2026-11-21
---

## 1. 主路径

```text
levier up
levier init [name]
levier model connect --url URL --key KEY --model MODEL
levier dev
levier run "prompt"
levier runtime init
levier runtime start
levier status
levier logs
levier down
```

| 命令 | 用途 | 细节 |
|---|---|---|
| `up` | 启动本地栈 | [启动](levier-install.md) |
| `init` | 项目 App 身份 | [项目初始化](app-dev.md) |
| `model connect` | 模型 | [模型连接](pre-model.md) |
| `dev` | 启动 App | [dev 与 serve](app-dev.md) |
| `run` | 提交 Run | [提交 Run](levier-run.md) |
| `runtime init` | 生成 Runtime | [runtime init](runtime-start.md) |
| `runtime start` | 启动 Runtime | [runtime start](runtime-start.md) |
| `status` | 状态 | [状态](levier-status.md) |
| `logs` | 日志 | [日志与进程](levier-status.md) |
| `down` | 停止 | [停止](levier-status.md) |

## 2. 观测

```text
levier status
levier run show <run_id> --events
levier runtime list
levier logs --follow
```

## 3. 高级

```text
levier setup
levier serve
levier doctor
levier describe
levier selfcheck [--deep]
levier app ...
levier mcp ...
levier run submit|watch|show|cancel
levier stack ...
levier ops ...
levier migrate ...
levier maintenance ...
levier config ...
```

它们仍然可用，但不出现在主帮助或黄金路径里。

## 4. 兼容

旧入口保留：

```text
levier provider ...      # 已收敛到 model
levier app ...           # 内部/自动化
levier run submit ...    # 低层脚本
levier stack ...         # Compose 包装
levier admin ...         # 运维命名空间
```

新脚本优先使用主路径；需要精确控制时再用高级命令。

## 5. --json

主路径命令支持 `--json`：

```sh
levier status --json
levier model list --json
levier run "hello" --json
levier runtime list --json
```

不要解析人类可读文本。

## 6. 退出码

```text
0  success
1  operation failed
2  invalid command / arguments
3  authentication / authorization failure
4  retryable / upstream failure
5  refused or timed out
```

## 下一步

- 环境变量：[环境变量](env.md)
- 错误：[错误与排查](errors.md)
- SDK 总览：[SDK 总览](sdk.md)
