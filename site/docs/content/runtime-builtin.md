---
title: 内置 Runtime
status: published
description: react1、echo1 与 conformance。
updated: 2026-11-21
---

## 1. react1

内置 ReAct Runtime，适合常规“模型 + 工具循环”。

```sh
levier run "Add 2 and 3." --runtime react1
```

需要先配置模型：

```sh
levier model connect --url ... --key ... --model ...
```

## 2. echo1

最小 Echo Runtime，不调用模型，不调用工具。

```sh
levier run "hello" --runtime echo1
```

适合验证：

```text
控制面
凭据
调度
事件
```

不适合验证模型或工具。

## 3. 内置 CLI

```text
levier-echo-runtime --config PATH --check --print-config --version --log-level LEVEL
levier-react-runtime --config PATH --check --print-config --version --log-level LEVEL
```

`--print-config` 会隐藏 token。

## 4. 选择建议

| 目标 | 选择 |
|---|---|
| 验证整条链路 | `echo1` |
| 常规 Agent 循环 | `react1` |
| 自定义 prompt / 工具顺序 | 自己的 `run(ctx)` |
| 完全不用模型 | 自己的 Runtime |

## 5. conformance

Echo 是最小 conformance 示例：它只实现 `run(ctx)` 契约，不依赖 React、LLM
或 ToolRouter。写自己的 Runtime 时可以参考它的行为。

## 下一步

- 写 Runtime：[Runtime 核心流程](runtime-core.md)
- Context：[Context](runtime-context.md)
- 实例与容量：[实例与容量](runtime-remote.md)
