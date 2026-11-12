<!-- Middleware — reaper and startup recovery. -->

# Reaper 与启动恢复

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 状态 | 完成，已合入 |
| 类型 | bug fix |

## 1. 修复内容

### 1.1 过期租约回收

新增 `workflow.RunReaper`：

```text
启动时立即执行一次
之后每 reap_interval 执行一次
调用 ReapExpired(now, reap_limit)
记录 mw_workflow_reaped_total
```

没有它时，worker 崩溃留下的 LEASED 任务只能等人工处理或永久卡住。

### 1.2 启动恢复

```text
RecoverQueued()   启动时重新入队 queued 状态的 Run
RecoverActive()   启动后等待 recovery_delay，
                  把仍处于 assigned/running 且对应 daemon 不在线的 Run requeue
```

`DaemonAvailable` 防止把“daemon 其实还连着”的 Run 误 requeue。

### 1.3 工具调用异步化

`DispatchToolCall`：

```text
有界并发槽位（toolSlots）
超过上限立即返回 TOOL_BUSY
每个调用独立 goroutine，带 WaitGroup
关闭时等待在途调用结束
```

之前工具调用在 daemon 的 envelope 处理路径里同步执行，慢工具会阻塞同一 daemon 的其它消息。

### 1.4 文档

`docs/api/openapi.yaml` 的 `POST /runs` 增加 503 响应：

```text
Dispatch queue unavailable; retry with the same Idempotency-Key
```

## 2. 配置

```yaml
workflow:
  reap_interval: "10s"
  reap_limit: 100
  recovery_delay: "10s"
```

环境变量：

```text
MIDDLEWARE_WORKFLOW_REAP_INTERVAL
MIDDLEWARE_WORKFLOW_REAP_LIMIT
MIDDLEWARE_WORKFLOW_RECOVERY_DELAY
```

## 3. 验证

```text
go test ./...                      全部通过
TestRunReaperRunsImmediately...    通过
make lint                          OK
make build                         OK
make retrofit-e2e                  PASS
  run 01M76VEG3FG5JKXJX6WQDBG7T8
  3 次工具调用，final=5，事件 16/16/16
```

## 4. 证据

```text
docs/reports/artifacts/reaper/
  go_test.log
  reaper_test.log
  lint.log
  build.log
  retrofit_e2e.log
```

## 5. 说明

部分 control/config 侧代码在 P8R.2/P8R.3 的提交中被顺带带入，
本次提交补齐了 runtime 入口、配置样例、OpenAPI 与独立单元测试。
