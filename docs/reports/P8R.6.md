<!-- Middleware — P8R.6 daemon onboarding kit. -->

# P8R.6：Daemon 接入套件

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.6 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 接入文档

新增 `docs/DAEMON_CONTRACT.md`：

```text
传输与身份：gRPC Channel + 预分配 daemon_id/token
Hello：runtime_type / capacity / strategies / labels / running_runs
TaskAssign：字段与 Ack
事件：seq / attempt / lease_epoch / terminal 规则
工具桥：ToolCallRequest/Result
Cancel / Reconcile / Heartbeat
容量、策略、错误语义
常见错误
```

## 2. 自检命令

```sh
make daemon-check
# 或
go run ./cmd/mw-daemon-check --grpc 127.0.0.1:9090 --http http://127.0.0.1:8080
```

实现复用 `internal/testutil/fakedaemon` 作为参考客户端。

检查内容：

```text
Hello（能收到 TaskAssign 即通过）
TaskAssign 的 run_id / strategy
Ack accepted
发 RUN_STATUS running + MESSAGE_COMPLETED + RUN_STATUS completed
第二个 Run：Cancel 信封 + RUN_STATUS cancelled
```

## 3. 实测

```json
{
  "ok": true,
  "checks": {
    "hello": "ok",
    "task_assign": "01M76WBG63GY4570KPNEXAD6DY",
    "ack": "accepted",
    "events": "running/completed",
    "completed": "ok",
    "cancel": "daemon-check",
    "cancelled": "ok"
  }
}
```

## 4. 前提

```text
daemon_id 必须在 MW 配置中预分配
同一 daemon_id 不能有其他 daemon 同时连接
用于检查时最好没有真实 Harness 竞争 echo 策略
```

## 5. 验证

```text
go test ./...          全部通过
make lint              OK
make build             OK
dependency check       OK
size report            无文件 > 450 行
```

## 6. 局限

```text
只验证 echo 策略
没有覆盖 ToolCallRequest 桥
没有覆盖 Reconcile
单 daemon、单实例
```

## 7. 证据

```text
docs/reports/artifacts/P8R.6/
  daemon_check.log
  go_test.log
  lint.log
  build.log
  deps.txt
  size.txt
  middleware.log
  migrate.log
```

## 8. 下一步

P8R.7：清理与架构文档。
