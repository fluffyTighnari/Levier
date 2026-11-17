<!-- Levier — protobuf contract workspace. -->

# Protobuf contracts

## 单元卡

| 项 | 内容 |
|---|---|
| 职责 | gRPC RuntimeChannel 契约源 |
| 入口 | `proto/middleware/runtime/v1/runtime.proto` |
| 输入 / 输出 | proto 定义；生成 Go / Python stub |
| 依赖 | 无运行时依赖 |
| 不负责 | 生成物手改、业务逻辑 |
| 测试 | `buf lint`、`make proto-lint`、`make proto-check` |

- `proto/middleware/runtime/v1/runtime.proto` — RuntimeChannel v1.
- `buf.yaml` — module, lint, and breaking-change policy.

Run:

```sh
buf lint
buf build
```

Generated Go/Python stubs are produced in P1-09; do not edit generated files by hand.
