<!-- Middleware — standardized file header convention. -->

# Middleware 文件头规范

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
// Middleware — internal/workflow
// Copyright 2026 The Middleware Authors.
package workflow
```

实现文件（非 `doc.go`）：

```go
// Middleware — internal/workflow
//
// Claim and lease semantics for run tasks.
//
// Copyright 2026 The Middleware Authors.
package workflow
```

规则：

- `doc.go` 的第一句必须是 `Package <name> ...`。
- 路径使用仓库相对路径，例如 `internal/workflow`、`cmd/middleware`。
- 不在头部写作者个人姓名或邮箱。

## 3. Python

```python
"""Middleware — harness RuntimeChannel client.

Owns the gRPC stream, Hello/Heartbeat, and reconnect loop.

Copyright 2026 The Middleware Authors.
"""
```

## 4. YAML / TOML / Makefile / SQL / Proto

使用对应语言的注释符：

```yaml
# Middleware — CI workflow.
# Copyright 2026 The Middleware Authors.
```

```sql
-- Middleware — RunTaskQueue schema.
-- Copyright 2026 The Middleware Authors.
```

```proto
// Middleware — RuntimeChannel v1 contract.
// Copyright 2026 The Middleware Authors.
```

## 5. Markdown

文档使用标题 + 元数据表；可选的 HTML 顶部注释用于说明文件归属：

```markdown
<!-- Middleware — implementation plan. -->

# ...
```

## 6. 生成文件

`buf generate`、`go generate` 产出的文件由生成器保留自己的头；不要手工添加或修改。
