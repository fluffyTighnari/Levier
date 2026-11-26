# internal

Levier 控制面的 Go 实现。I3 后按职责归并为六个层，而不是继续平铺在
`internal/` 根目录。

```text
contract/       协议与边界
  contract/       HTTP/WS 常量、事件名、错误语义
  protocol/       RuntimeChannel envelope、校验与黄金契约
  roles/          可部署角色集合（all/control/api/...）

platform/       无业务依赖的基础设施
  clock/ db/ errs/ id/ observe/ version/

state/          PostgreSQL 事实来源与执行队列
  cluster/        gateway/runtime/app lease、capacity、outbox
  register/       App、Tool、MCP、model provider、secret 注册
  runstore/       Run / Event / App / Session / checkpoint 持久化
  workflow/       durable RunTaskQueue、worker、lease、reaper

controlplane/   Control 角色与跨副本协作
  control/        Run 生命周期、调度、事件应用
  controlremote/  Runtime Gateway / App Gateway 内部 RPC 端口
  controlservice/ ControlService 与 durable envelope outbox consumer

runtime/        Runtime 接入侧
  connection/     DaemonHub / AppHub / local tool pending
  gateway/        local/global tool invocation 与幂等
  internalapi/    角色间 gRPC listener/client
  ports/          Control 使用的 Runtime/App 端口
  runtimegateway/ Runtime Gateway 角色实现
  daemoncheck/    daemon 自检

app/            App 接入侧
  httpapi/        HTTP API 与 App Bridge WebSocket
  appgateway/     App Gateway 角色实现、连接/执行租约

ops/            运维与可观测表面
  maintenance/ selfcheck/ selfdesc/ status/

testing/        测试支撑
  testutil/      test schema、env、fakedaemon
```

依赖方向：

```text
cmd/levier
  -> app / controlplane / runtime / ops / state / platform / contract

app/httpapi
  -> state/register, state/runstore, state/workflow,
     runtime/connection, contract

controlplane/control
  -> state/register, state/runstore, state/workflow,
     runtime/connection, runtime/ports, contract

controlplane/controlremote
  -> runtime/ports, runtime/internalapi, state/cluster

runtime/gateway
  -> state/register, runtime/connection, platform

state/runstore, state/workflow, state/register
  -> platform, contract

contract, platform
  -> 不依赖更高层
```

规则：

- `contract/` 与 `platform/` 不得反向依赖业务层。
- `state/` 是 PostgreSQL 事实来源；不得依赖 HTTP、Control 或角色实现。
- `controlplane/`、`runtime/`、`app/` 之间通过 `ports` 或内部 RPC 通信。
- `cmd/levier` 只做装配，不承载业务分支。
- 已废弃路径不要继续新增；新增包先判断属于哪个层，再放入对应分组。

验证：`go test ./...`、`make deps-check`、`make structure-check`。PostgreSQL
集成测试需要 `LEVIER_TEST_POSTGRES=1` 与 `LEVIER_TEST_DATABASE_URL`；多副本
故障矩阵使用 `make i3-multireplica`。
