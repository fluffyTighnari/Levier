# cmd

职责：Levier 的 Go 可执行入口与 wiring。

入口：

```text
cmd/levier/           控制面服务
cmd/migrate/          数据库迁移
cmd/mw-daemon-check/  Daemon 协议自检
```

输入 / 输出：

```text
输入：YAML 配置、环境变量、HTTP / gRPC 请求
输出：HTTP / gRPC 服务、迁移结果、自检 PASS/FAIL
```

依赖：`internal/*`。

不负责：业务逻辑、协议定义、存储查询、Python 执行侧。

测试：`go build ./...`、`go test ./...`、`make build`。
