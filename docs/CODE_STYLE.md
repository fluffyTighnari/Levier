<!-- Levier — standardized file header convention. -->

# Levier 文件头规范

| 项目 | 内容 |
|---|---|
| 状态 | 强制 |
| 生效 | 2026-11-03，适用于后续所有新增/修改文件 |
| 例外 | 生成文件（buf/go generate、锁文件）不手工修改 |

## 1. 目标

每个源码/配置文件顶部必须有统一注释，说明文件归属、职责与版权。
注释不能包含密钥、内部 URL、个人邮箱。

## 2. Go

```go
// Package workflow implements the PostgreSQL-backed RunTaskQueue.
//
// Levier — internal/workflow
// Copyright 2026 The Levier Authors.
package workflow
```

实现文件（非 `doc.go`）：

```go
// Levier — internal/workflow
//
// Claim and lease semantics for run tasks.
//
// Copyright 2026 The Levier Authors.
package workflow
```

规则：

- `doc.go` 的第一句必须是 `Package <name> ...`。
- 路径使用仓库相对路径，例如 `internal/workflow`、`cmd/levier`。
- 不在头部写作者个人姓名或邮箱。

## 3. Python

```python
"""Levier — harness RuntimeChannel client.

Owns the gRPC stream, Hello/Heartbeat, and reconnect loop.

Copyright 2026 The Levier Authors.
"""
```

## 4. YAML / TOML / Makefile / SQL / Proto

使用对应语言的注释符：

```yaml
# Levier — CI workflow.
# Copyright 2026 The Levier Authors.
```

```sql
-- Levier — RunTaskQueue schema.
-- Copyright 2026 The Levier Authors.
```

```proto
// Levier — RuntimeChannel v1 contract.
// Copyright 2026 The Levier Authors.
```

## 5. Markdown

文档使用标题 + 元数据表；可选的 HTML 顶部注释用于说明文件归属：

```markdown
<!-- Levier — implementation plan. -->

# ...
```

## 6. 生成文件

`buf generate`、`go generate` 产出的文件由生成器保留自己的头；不要手工添加或修改。

## 7. JSON 数据文件

JSON 不支持注释。JSON fixture/数据文件不强制顶部注释，但必须有相邻 README 说明用途与来源。
测试黄金样本属于此类例外。

## 8. 目录与结构护栏

`make structure-check` 检查以下规则：

```text
顶层目录必须存在；agent_core/、harness/、mcp_runtime/ 不能回到根目录
每个顶层单元必须有 README.md
internal / cmd / execution 不能引用 examples/
agent_core 不能 import harness / MCP / protobuf
cmd/* 只放 package main
gen/*.go 必须标记为生成代码
operational 文件不能引用搬迁前的旧路径
```

Go internal 包依赖方向由 `make deps-check` 检查。

新增顶层目录、移动单元或改变 import 方向时，先跑：

```sh
make structure-check
make deps-check
```
