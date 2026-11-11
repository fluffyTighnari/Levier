<!-- Middleware — MCP M4 negative and lifecycle scenarios. -->

# MCP M4：负向与生命周期

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-11 |
| 状态 | 完成；附带修复 runtime 停机挂起 |
| 依赖 | M1–M3 |

## 1. 方法

一个真实目录条目 + 一个真实 App 引用，按场景切换：

```text
m4allow    allowed_tools = echo_token / echo_pid
m4missing  env = ${secret:not_configured}
fixture    secret_echo_server.py（echo_token / echo_pid / other_tool）
```

## 2. A：allowlist 过滤与拒绝

直接通过 Harness 客户端打开 runtime（不走模型，结果确定）：

```json
{
  "visible_tools": ["mcp__m4allow__echo_token"],
  "direct_call_other_tool": "TOOL_DENIED"
}
```

- 未授权工具不出现在 `tools[]`
- 直接按名字调用仍被 runtime 拒绝

## 3. B：缺失 secret

目录：

```text
env = {"PROBE_TOKEN": "${secret:not_configured}"}
```

Run 输出：

```text
seq 2 ERROR SECRET_MISSING "secret 'not_configured' is not configured"
seq 5 RUN_STATUS completed
```

- open 在连接前失败
- runtime 没有新增 session（计数 1 → 1）
- Run 继续完成，模型收到错误事件

## 4. C：两个 Run 并发

```text
sessions_before = 1
sessions_after  = 3
```

| Run | 工具 | 结果 |
|---|---|---|
| `01M75C0CQXXB3FZED4QFJ1454G` | `mcp__m4allow__echo_pid` | `82542` |
| `01M75C0CQYW8F51HPS0BNQREK6` | `mcp__m4allow__echo_pid` | `82543` |

- 两个 Run 各自拿到独立 stdio 进程
- 最终回答分别引用自己的 PID
- 结束后 `pgrep -f secret_echo_server.py` = 0

## 5. D：Runtime 不可用

停掉 runtime（socket 已删除）后提交 Run：

```text
seq 2 ERROR MCP_RUNTIME_UNAVAILABLE retryable=true
seq 5 RUN_STATUS completed
```

MCP 不可用不会阻塞 Run；错误对模型可见。

## 6. E：发现并修复停机挂起

### 6.1 现象

第一次执行 D 时，`kill -TERM` 没有让 runtime 退出，Run 仍然成功打开了 session。

根因：

```python
await self._server.wait_closed()
```

Python 3.12 的 `wait_closed()` 会等待**所有活动连接**结束；
Harness 的 JSON-RPC 连接一直挂着，所以关闭流程永远等不到。

### 6.2 修复

`mcp_runtime/src/mcp_runtime/server.py`：

```text
跟踪活动 writer
close():
  server.close()
  for writer in writers: writer.close()
  await wait_for(server.wait_closed(), 2s)
  manager.close_all()
```

### 6.3 验证

保持一个客户端连接，发送 SIGTERM：

```text
stopped=true elapsed_ms=121
mcp runtime stopped
```

新增单测：

```text
test_close_with_connected_client_completes
```

## 7. 汇总

| 场景 | 结果 |
|---|---|
| allowlist 不可见 | 通过 |
| 非授权直呼 → TOOL_DENIED | 通过 |
| 缺失 secret → SECRET_MISSING | 通过 |
| Run 不因 MCP 失败中断 | 通过 |
| 并发 session 隔离 | 通过 |
| 子进程无泄漏 | 通过 |
| runtime 停机 → MCP_RUNTIME_UNAVAILABLE | 通过 |
| runtime SIGTERM 可靠退出 | 修复后通过 |

## 8. 局限

- 并发只测了 2 个 Run。
- 没有覆盖 SSE / streamable_http 的失败语义。
- 未覆盖 runtime 在调用中途崩溃（只测了 open 前不可用）。

## 9. 证据

```text
docs/reports/artifacts/mcp/M4/
  bootstrap.json
  log
  A/ probe.py probe.json summary.txt
  B/ catalog.json db.txt result.txt submit_run.json summary.txt
  C/ catalog.json db.txt result.txt run_1.json run_2.json summary.txt
  D/ db.txt result.txt submit_run.json summary.txt
  E/ runtime_sigterm.log result.txt summary.txt
  middleware.log
  harness.log
  runtime.log
  migrate.log
```

## 10. 下一步

```text
M5 真实第三方/独立 MCP server
M6 make mcp-e2e + 文档
```
