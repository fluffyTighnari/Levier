# internal

职责：Levier 控制面的全部 Go 实现。

关键包：

```text
httpapi     HTTP + WebSocket 适配
control     Run 状态机、调度、租约、事件投影
workflow    PostgreSQL RunTaskQueue 与 worker
gateway     local / global 工具路由与幂等
connection  DaemonHub / AppHub / 在线状态
register    App、工具、MCP 目录与 ACL
runstore    Run / Event / App / Session 持久化
observe     指标、日志、脱敏
protocol    gRPC 信封与校验
contract    事件字段与常量
config      配置加载与校验
db          PostgreSQL 连接与迁移
id          ULID
errs        错误码与 HTTP 映射
```

输入 / 输出：

```text
输入：cmd/levier 的装配、HTTP/WS/gRPC 请求、PostgreSQL
输出：调度决策、工具调用、事件、指标、日志
```

不负责：Python 执行侧、参考 App、生成代码、部署文件。

测试：`go test ./...`；真实 PostgreSQL 集成测试见 `docs/MVP.md`。
