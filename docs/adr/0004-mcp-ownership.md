# ADR-0004：MCP Runtime 独立进程与连接归属

| 项目 | 内容 |
|---|---|
| 状态 | 提议（P3-29 设计修订） |
| 日期 | 2026-11-06 |
| 决策者 | middleware 项目 |
| 关联 | `docs/DESIGN.md`、`docs/adr/0002-harness.md`、`docs/adr/0003-scope.md` |
| 修订 | ADR-0002 §9.2–§9.4 的 MCP 归属描述；不改变“总控不持有连接”的结论 |

---

## 1. 背景

ADR-0002 曾把 MCP 连接放在 Harness 内部，由 `McpToolProvider` 负责，并给出“Python in-process、非 Python 本地 sidecar”的演进路径。P3-29 尚未实现，现对该边界做一次明确修订：

- `McpRuntime` 是一个**独立进程/组件**，不是 Harness 进程里的一个类。
- Harness 与 Middleware 最终是独立交付物；MCP Runtime 也不应放进 `harness/` 目录。
- 多个 Harness 不应各自实现一套 MCP 客户端；共享的是 Runtime 实现和本地契约。
- 总控仍然只存目录、ACL、`mcp_bindings`，不持有 MCP 连接、不代理 MCP 工具结果。

---

## 2. 决策

### 2.1 目录与交付形态

新增仓库顶层目录：

```text
mcp_runtime/
  README.md
  pyproject.toml          # 独立 Python 项目（P3-29 实现）
  src/mcp_runtime/
    __main__.py           # 进程入口
    server.py             # 本地 RPC 服务
    sessions.py           # MCP session 生命周期
    transports/           # stdio / sse / streamable_http
```

职责：

| 组件 | 职责 |
|---|---|
| `mcp_runtime` 进程 | 建立 MCP 连接、管理 stdio 子进程、`tools/list`、`tools/call`、session 生命周期 |
| Harness | 通过 `McpRuntimeClient` 调用 Runtime；把发现的工具合并进 LLM 工具清单；发 `TOOL_CALL_BEGIN/END` 事件 |
| Middleware Register/Control | 只存 MCP 目录、App 引用、ACL、`auth_ref`；组装 `TaskAssign.mcp_bindings` |
| Middleware Tool Gateway | 只处理 local/global 工具；不接触 MCP |

`mcp_runtime/` 不属于 `harness/`，也不属于 Go middleware。它可以和 Harness 同 pod 部署，但生命周期、配置、依赖和发布物独立。

### 2.2 本地契约

v1 使用**本地 Unix domain socket + JSON-RPC 2.0**：

```text
Harness McpRuntimeClient  ── Unix socket ──►  mcp_runtime 进程  ──► MCP servers
```

- 默认 socket 路径由 Runtime 配置决定（例如 `/run/middleware/mcp.sock`）。
- 同一 host/pod 内的多个 Harness 可以连接同一个 Runtime。
- 单客户端调试模式可以支持 stdio，但生产默认是 Unix socket。
- 契约不依赖 Python；后续 Go/Rust Harness 使用同一 JSON-RPC 方法。

方法：

```text
mcp.health() -> {status, version}
mcp.open(binding) -> {session_id, tools[]}
mcp.call(session_id, tool_name, arguments_json, timeout_ms) -> {ok, result_json, error}
mcp.close(session_id) -> {}
```

约束：

- Runtime 不在 RPC 中返回 socket、进程句柄或连接对象。
- `tools[].name` 已按 `mcp__<server_name>__<tool_name>` 命名空间化。
- `allowed_tools` 在 Runtime 侧强制过滤；未授权工具不出现在 `tools[]`，调用时返回 `TOOL_DENIED`。
- 每个 Run 至少一个独立 session；不跨 Run 复用 session，避免状态与凭据串扰。
- session 关闭必须回收 stdio 子进程、HTTP 连接和临时文件。

### 2.3 TaskAssign 与凭据

`TaskAssign.mcp_bindings` 只传连接描述和凭据材料，不传连接对象：

```text
server_name
transport
command / args / env       # stdio
url / headers              # sse / streamable_http
allowed_tools[]
namespace
```

v1 保持现有 DESIGN 语义：

- 注册表只存 `auth_ref`。
- `AssembleTaskMaterials` 在 Middleware 侧解析 `${secret:*}`。
- `TaskAssign.mcp_bindings` 中是**已解析凭据**，明文过 gRPC。
- Runtime 只在内存使用凭据，不写日志、不入 checkpoint、不回传事件。

> 如果后续要求 TaskAssign 只传“凭据引用”，需要新增凭据解析协议或短期凭据代理；这是 v2 变更，不在本 ADR 的 v1 范围内。

### 2.4 审计与事件

- Runtime 返回结构化结果和耗时元数据。
- Harness 负责发 `TOOL_CALL_BEGIN` / `TOOL_CALL_END`，`source=mcp`。
- Middleware 只接收事件并持久化，不参与 MCP 调用。
- v1 不做中央 MCP 审计/限流/配额；需要时由未来 MCP Hub 或 Runtime 自身日志承担。

### 2.5 生命周期与失败

- Runtime 可以先于 Harness 启动，也可以由 Harness 作为子进程拉起；两种方式使用同一 socket 契约。
- Runtime 崩溃或 socket 不可用：Harness 将 MCP 工具标记为不可用；调用返回 `MCP_RUNTIME_UNAVAILABLE`，不阻塞其他工具。
- 单个 MCP server 连接失败：该 server 的工具不可用，其他 server 继续。
- Runtime 优雅退出时关闭所有 session 和 stdio 子进程。

---

## 3. 不采用方案

| 方案 | 结论 | 原因 |
|---|---|---|
| MCP 客户端作为 Harness 内部类 | 拒绝 | 无法跨语言共享；进程内实现容易演变成每套 Harness 各写一份 |
| MCP 连接放进 Middleware Tool Gateway | 拒绝 | 会把控制面变成数据面；stdio 进程和凭据应留在执行侧 |
| 每个 Harness 各自实现 MCP | 拒绝 | 行为不一致、维护成本高 |
| v1 引入远程 MCP Hub | 拒绝 | 当前没有跨 App/Run 共享连接、总控级审计等需求；保留 v2 扩展点 |

---

## 4. 后果

正面：

- MCP 实现与 Harness/Middleware 解耦，可独立升级、扩缩容和测试。
- 多语言 Harness 共享同一 Runtime，不重复实现 MCP 协议。
- Middleware 保持控制面，不持有 MCP 连接。
- stdio/HTTP 的放置与 sandbox 边界更清晰。

代价：

- 多一个进程和本地 RPC，部署与健康检查更复杂。
- 需要定义并维护 JSON-RPC 契约。
- 凭据仍会到达 Runtime 进程内存；v1 依赖可信网络和进程隔离。

---

## 5. 对 ADR-0002 的修订

- ADR-0002 §9.2 的“连接归属 Harness”修订为“连接归属执行层独立 `mcp_runtime` 进程”。
- §9.3 的 `McpToolProvider` 类接口修订为 `McpRuntimeClient` + Runtime JSON-RPC 契约。
- §9.4 的“Python in-process”修订为“v1 独立进程；单客户端可用 stdio，生产默认 Unix socket”。
- “总控只存目录/ACL/bindings，不持有连接”的结论不变。

---

## 6. 验收影响

P3-29 必须覆盖：

1. `mcp_runtime/` 独立进程可启动、健康检查、优雅退出。
2. Unix socket JSON-RPC 契约：`open/list/call/close`。
3. Harness `McpRuntimeClient` 不导入 MCP SDK。
4. stdio、SSE、streamable_http 至少各一条连接/失败测试。
5. `allowed_tools` 白名单、命名空间、session 隔离。
6. Runtime 崩溃/不可达时，MCP 工具失败但不阻塞 Run。
7. Middleware 代码中不存在 MCP 连接或 MCP session。

---

## 5. 补充（2026-11-11）：凭据解析归属

实现 MCP 链路时明确一条：**总控不解析 MCP 凭据**。

- 目录 API：`POST /admin/mcp`、`GET /admin/mcp`、`DELETE /admin/mcp/{name}`。
- `config_json` 中的凭据写成 `${secret:name}` 占位符。
- Control 组装 `TaskAssign.mcp_bindings` 时保留占位符，不做替换。
- `mcp_runtime` 在建立连接前解析：环境变量 `MIDDLEWARE_SECRET_<NAME>`，或 `--secrets-file` 指定的 0600 JSON。
- 缺少 secret 时返回 `SECRET_MISSING`，Harness 收到错误事件，不启动该 server。
- 这样 Harness 只转发描述，不接触 MCP 明文凭据；总控仍不持有连接。
