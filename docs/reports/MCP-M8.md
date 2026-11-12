<!-- Middleware — MCP M8 transport drop and recovery. -->

# MCP M8：HTTP transport 断线与恢复

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 状态 | 完成 |
| 范围 | SSE / streamable_http 调用中断线 |
| 结论 | 不支持 in-flight 续传；新会话可恢复 |

## 1. 先明确“续传”指什么

```text
A. 服务端主动消息的流（GET stream）
   streamable_http SDK 支持 Last-Event-ID 自动重连
   SSE 没有自动重连

B. 正在进行的 tools/call
   POST 请求失败后不会自动重放
   MCP 工具不一定幂等，重放有风险
```

本轮验证的是 B：调用进行中断线。

## 2. 方法

`docs/reports/artifacts/mcp/M8/probe.py`：

```text
启动 runtime + HTTP MCP server
open 一个会话
调用 slow(30)
1 秒后杀掉 server
观察调用结果、session 计数
server 恢复后新开会话并调用 echo
```

`slow` 是新增的测试工具（仅 fixture）。

## 3. 结果

两条 transport 表现一致：

```json
{
  "mid_call": {"ok": false, "error_code": "TOOL_EXEC_ERROR"},
  "sessions_after_drop": 1,
  "sessions_after_close": 0,
  "open_while_down": "MCP_CONNECT_FAILED",
  "recovered": {"ok": true, "result": "...recovered..."}
}
```

含义：

```text
调用中断 → TOOL_EXEC_ERROR，retryable=true
会话不会自动消失，等待 close 释放
server 不在 → MCP_CONNECT_FAILED
server 恢复 → 新会话可以正常调用
```

## 4. 产品行为

```text
不自动重放 in-flight 工具调用
Run 收到工具错误，由模型决定是否重试
Harness 在 Run 结束时 close 会话
下一次调用/下一次 Run 重新 open
```

理由：

```text
MCP 工具可能非幂等
自动重放可能产生重复副作用
```

## 5. 文档同步

`docs/MCP.md` 的限制从：

```text
SSE 断线重连 / 事件续传未测
```

改为明确行为：

```text
SSE 无自动重连
streamable_http 的 GET 流可重连，但 tools/call 不重放
传输中断 → TOOL_EXEC_ERROR；新会话可恢复
```

## 6. 局限

```text
没有覆盖服务端通知的续传
没有覆盖 harness 端到端中断线
没有自动重试（刻意不做）
```

## 7. 证据

```text
docs/reports/artifacts/mcp/M8/
  probe.py
  sse.json
  http.json
  sse.err
  http.err
```

## 8. 下一步

可选：

```text
MCP 并发/浸泡
pi-core 通用逻辑抽取评估
```
