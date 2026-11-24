# Levier 操作面清单

> 状态：2026-11-21，按当前工作区代码整理。
>
> 标记：`[主]` 黄金路径；`[高级]` 显式高级命令；`[兼容]` 保留但不出现在主帮助里的入口。
>
> 本文只描述“用户能调用的面”，不展开控制面内部实现。

---

## 1. Levier CLI：主路径

- `levier` — `[主]` 不带参数只打印帮助，不启动服务。
- `levier help` — `[主]` 打印主帮助。
- `levier version [--json]` — `[主]` 打印版本、commit、构建时间、Go 版本和平台。
- `levier up [--file PATH] [--server URL] [--profile CSV] [--build] [--all] [--wait] [--dry-run] [--timeout DUR]` — `[主]` 启动本地 Compose 栈核心服务并等待 `/readyz`，默认不启动示例 agent 容器。
- `levier down [--file PATH] [--volumes]` — `[主]` 停止本地栈；`--volumes` 同时删除命名卷。
- `levier logs [--file PATH] [--follow]` — `[主]` 查看 Compose 日志；`--follow` 持续跟随。
- `levier ps [--file PATH]` — `[主]` 查看 Compose 服务状态。
- `levier status [--server URL] [--json] [--timeout DUR]` — `[主]` 查询 `/status`，显示健康、就绪、daemon、App 和 Run 概览。
- `levier init [name] [--name NAME] [--server URL] [--admin-token TOKEN] [--runtime NAME] [--mcp-ref CSV] [--rotate] [--force] [--json] [--timeout DUR]` — `[主]` 创建/复用当前项目的 App，写默认凭据、用户 profile 和项目 `.levier.json`。
- `levier dev [--file PATH] [--python PY] [--server URL] [--credentials PATH] [--mcp-ref CSV]` — `[主]` 启动项目 App，默认查找 `app.py`/`levier_app.py`，自动注入凭据、server 和 MCP 引用。
- `levier model connect --url URL --key KEY --model MODEL [--name NAME] [--adapter auto|openai_compatible|anthropic] [--models CSV] [--default] [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[主]` 从 URL 推断 provider/adapter/secret_ref，保存 provider 元数据并把 key 写入 managed secret 文件。
- `levier model list [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[主]` 列出已连接模型，不显示 secret_ref。
- `levier model check --name NAME [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[主]` 检查 provider 和 secret 是否可用。
- `levier model remove --name NAME --yes [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[主]` 删除 provider 元数据。
- `levier run "prompt" [--runtime NAME] [--provider NAME] [--model NAME] [--max-steps N] [--run-ms N] [--tool-ms N] [--no-watch] [--watch] [--with-app] [--app PATH] [--python PY] [--interval DUR] [--watch-timeout DUR] [--timeout DUR] [--credentials PATH] [--server URL] [--json]` — `[主]` 用 profile/项目默认凭据提交 Run 并默认观测到终态；prompt 前后都可以放 flag。
- `levier run ... --with-app` — `[主]` 在 Run 期间临时启动 `app.py`；如果已有 App 连接则不重复启动。
- `levier runtime init [--file PATH] [--force]` — `[主]` 生成最小 `runtime.py` 模板。
- `levier runtime start [--file PATH] [--name NAME] [--token TOKEN] [--grpc ADDR] [--python PY] [--config PATH] [--log-level LEVEL]` — `[主]` 启动自定义 Runtime，默认查找 `runtime.py`/`levier_runtime.py`/`my_runtime.py`，自动注入地址和 token。

---

## 2. Levier CLI：高级与兼容面

- `levier setup [--server URL] [--admin-token TOKEN] [--credentials PATH] [--runtime NAME] [--grpc ADDR] [--runtime-token TOKEN] [--runtime-config PATH] [--mcp-socket PATH] [--control-config PATH] [--stack-file PATH] [--reset] [--show] [--json]` — `[高级]` 写用户级 profile；`--show` 只显示不写入，token 只在显式传入时保存。
- `levier serve [-config PATH]` — `[高级]` 启动控制面；`-config`/`--config` 指定控制面 YAML。
- `levier doctor [--config PATH] [--server URL] [--credentials PATH] [--json] [--timeout DUR]` — `[高级]` 检查配置、HTTP、数据库、迁移、daemon 和可选 App 凭据。
- `levier app init --name NAME [--credentials-out PATH] [--server URL] [--admin-token TOKEN] [--rotate] [--yes] [--force] [--mcp-ref CSV] [--dry-run] [--json] [--timeout DUR]` — `[兼容]` 低层 App 创建命令，显式控制凭据输出路径。
- `levier app show [--credentials PATH] [--server URL] [--json] [--timeout DUR]` — `[兼容]` 查看服务端 App 元数据和本地凭据摘要。
- `levier app check [--credentials PATH] [--server URL] [--json] [--timeout DUR]` — `[兼容]` 检查本地凭据、服务端 App、key 和 WebSocket token。
- `levier app list [--server URL] [--admin-token TOKEN] [--json] [--timeout DUR]` — `[兼容]` 管理员列出 App。
- `levier app rotate-key [--credentials PATH] [--server URL] [--admin-token TOKEN] [--yes] [--json] [--timeout DUR]` — `[兼容]` 轮换 App key 并原子写回凭据文件。
- `levier mcp list [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[高级]` 列出 MCP Server。
- `levier mcp add --name NAME --config-file PATH [--transport stdio|sse|streamable_http] [--allowed-tools CSV] [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[高级]` 注册 MCP Server 及其 allowed_tools。
- `levier mcp remove --name NAME --yes [--force] [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[高级]` 删除 MCP Server；`--force` 同时清理 App 引用。
- `levier run submit --runtime NAME --prompt TEXT [--credentials PATH] [--server URL] [--provider NAME] [--model NAME] [--max-steps N] [--run-ms N] [--tool-ms N] [--idempotency-key KEY] [--request-id ID] [--watch] [--interval DUR] [--watch-timeout DUR] [--timeout DUR] [--json]` — `[兼容]` 低层提交 Run，可显式控制所有超时和幂等键。
- `levier run watch [run_id] [--run-id ID] [--credentials PATH] [--server URL] [--interval DUR] [--timeout DUR] [--request-timeout DUR] [--json]` — `[兼容]` 轮询 Run 到终态，支持位置参数 run id。
- `levier run show [run_id] [--run-id ID] [--events] [--credentials PATH] [--server URL] [--timeout DUR] [--json]` — `[兼容]` 查看 Run 详情；`--events` 同时列出事件。
- `levier run cancel [run_id] [--run-id ID] [--yes] [--credentials PATH] [--server URL] [--timeout DUR] [--json]` — `[兼容]` 取消 Run；非交互环境需要 `--yes`。
- `levier runtime list [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[高级]` 列出已连接 Runtime、实例、容量和心跳。
- `levier stack up [--file PATH] [--project-directory PATH] [--project-name NAME] [--profile CSV] [--build] [--detach] [--dry-run]` — `[兼容]` 直接包装 `docker compose up`。
- `levier stack down [--file PATH] [--project-directory PATH] [--project-name NAME] [--volumes] [--remove-orphans] [--dry-run]` — `[兼容]` 直接包装 `docker compose down`。
- `levier stack ps [--file PATH] [--project-directory PATH] [--project-name NAME] [--dry-run]` — `[兼容]` 直接包装 `docker compose ps`。
- `levier stack logs [--file PATH] [--project-directory PATH] [--project-name NAME] [--follow] [--dry-run]` — `[兼容]` 直接包装 `docker compose logs`。
- `levier ops runs list [--status CSV] [--app-id ID] [--daemon-id ID] [--session-id ID] [--cursor C] [--limit N] [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[兼容]` 管理员分页查看 Run。
- `levier ops runs show --run-id ID [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[兼容]` 管理员查看单个 Run。
- `levier ops runs cancel --run-id ID [--reason TEXT] [--yes] [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[兼容]` 管理员取消 Run。
- `levier ops daemons [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[兼容]` 查看 daemon 列表。
- `levier ops tasks [--status CSV] [--type CSV] [--run-id ID] [--limit N] [--offset N] [--server URL] [--admin-token TOKEN] [--timeout DUR] [--json]` — `[兼容]` 查看 workflow task，默认 `--status DEAD`。
- `levier migrate status [--config PATH] [--json] [--timeout DUR]` — `[高级]` 查看数据库迁移版本和 pending 数。
- `levier migrate up [--config PATH] [--json] [--timeout DUR]` — `[高级]` 执行数据库迁移。
- `levier maintenance status [--config PATH] [--json] [--timeout DUR]` — `[高级]` 查看 retention/数据维护状态。
- `levier maintenance prune [--config PATH] [--yes] [--dry-run] [--json] [--timeout DUR]` — `[高级]` 清理过期数据；默认 dry-run，真删需要 `--yes`。
- `levier config check [--config PATH] [--production] [--json]` — `[高级]` 校验控制面 YAML；`--production` 启用生产 secret 要求。
- `levier config print [--config PATH] [--json]` — `[高级]` 打印脱敏后的控制面配置。
- `levier admin <ops|migrate|maintenance|config|daemon|describe|selfcheck>` — `[高级]` 将上述运维命令收进 `admin` 命名空间。
- `levier describe [--server URL] [--json] [--timeout DUR]` — `[高级]` 读取 `/meta` 并显示服务能力和限制。
- `levier selfcheck [--config PATH] [--server URL] [--grpc ADDR] [--admin-token TOKEN] [--credentials PATH] [--daemon-id ID] [--daemon-token TOKEN] [--mcp-socket PATH] [--runtime NAME] [--timeout DUR] [--deep] [--json]` — `[高级]` 逐跳自检；`--deep` 会做主动 daemon/WS/tool/MCP 检查。
- `levier daemon check [--grpc ADDR] [--server URL] [--daemon-id ID] [--token TOKEN] [--admin-token TOKEN] [--runtime NAME] [--timeout DUR] [--json]` — `[兼容]` 用临时 App/Run 检查 RuntimeChannel 协议。
- 顶层兼容别名：`levier ops|migrate|maintenance|config|daemon|describe|selfcheck|stack|app|provider|mcp|run|runtime` — `[兼容]` 旧入口仍可直接使用，内部转调对应命令。

退出码：`0` 成功、`1` 运行失败、`2` 用法/配置错误、`3` 认证失败、`4` 网络或上游可重试失败、`5` 拒绝/超时。

---

## 3. App SDK：高层 `levier`

- `App(tools=(), *, mcp=None, runtime=None, credentials=None, server=None, logger=None, on_event=None)` — 创建 App；默认从项目 `.levier.json` / 用户 profile 读取凭据、server、runtime 和 MCP 引用。
- `App.tool(function=None, *, name=None, description=None, schema=None, timeout_ms=10000)` — 装饰器注册工具，支持 `@app.tool`、`@app.tool("name")`、`@app.tool(name=...)`。
- `App.add(item)` — 注册一个 `levier.Tool` 或普通函数；重复注册同名工具会报错。
- `App.serve()` — 连接 Levier 并阻塞服务工具调用。
- `App.stop()` — 请求停止 `serve()` 或一次性 `run()`。
- `App.run(prompt=None, *, runtime=None, provider="", model="", max_steps=8, run_ms=60000, tool_ms=10000, timeout=300.0, print_output=True)` — 无 prompt 时等价 `serve()`；有 prompt 时临时起连接、提交 Run、等待并打印最终消息。
- `App.submit_run(prompt, *, runtime=None, **options)` — 提交 Run；`options` 可传 `runtime_type/session_id/provider/model/max_steps/run_ms/tool_ms/idempotency_key`。
- `App.wait_run(run_id, *, timeout=300.0)` — 轮询 Run 到终态或 `waiting`。
- `App.get_run(run_id)` — 获取单个 Run。
- `App.get_events(run_id)` — 获取 Run 事件列表。
- `App.cancel_run(run_id)` — 取消 Run。
- `App.resume_run(run_id)` — 恢复 waiting Run。
- `App.list_model_providers()` — 获取当前 App 可见的模型目录。
- `App.tools_manifest()` — 获取已注册工具的 manifest。
- `levier.tool(function=None, *, name=None, description=None, schema=None, timeout_ms=10000)` — 把普通函数转成 `Tool`，从类型注解和 docstring 自动生成 schema。
- `Tool.invoke(arguments)` — 用 `arguments` 字典调用 handler。
- `Tool.__call__(**arguments)` — 直接按关键字参数调用 handler。
- `schema_from_function(function)` — 从 Python 函数签名生成 JSON Schema。
- `from levier import Context, RunContext, Runtime, Toolbox` — 懒加载 Runtime SDK 的对应对象，App-only 安装不需要立即导入 gRPC。

---

## 4. App SDK：低层 `levier_app`

- `LevierApp(credentials, *, server=None, transport=None, logger=None, on_event=None, mcp_refs=None)` — 低层适配器构造，显式注入凭据和 transport。
- `LevierApp.from_credentials(path=None, *, server=None, transport=None, logger=None, on_event=None, mcp_refs=None)` — 从凭据文件或项目/profile 默认值创建 App。
- `LevierApp.tool(name, *, description="", schema=None, timeout_ms=10000)` — 低层工具注册，schema 需要显式提供。
- `LevierApp.register(tool)` — 注册一个 `levier.Tool` 并复用其 name/description/schema/timeout。
- `LevierApp.tools_manifest()` — 返回工具 manifest。
- `LevierApp.run(*, max_calls=0, idle_timeout=0.0, ping_interval=20.0, reconnect_initial=0.5, reconnect_max=30.0, handle_signals=True)` — 启动 WebSocket 服务循环。
- `LevierApp.stop()` — 请求优雅停止。
- `LevierApp.wait_connected(timeout=10.0)` — 等待第一次 WebSocket 握手。
- `LevierApp.run_cli(register=None, *, argv=None, prog="levier-app")` — 标准 App CLI 外壳，自动处理 argparse、凭据、日志和信号。
- `LevierApp.submit_run(prompt, *, runtime, runtime_type="", session_id="", provider="", model="", max_steps=8, run_ms=60000, tool_ms=10000, idempotency_key="")` — 低层 Run 提交。
- `LevierApp.wait_run(run_id, *, timeout=300.0, interval=0.5)` — 轮询等待终态或 `waiting`。
- `LevierApp.get_run(run_id)` — 查询 Run。
- `LevierApp.get_events(run_id, *, after_seq=0, limit=1000)` — 查询 Run 事件。
- `LevierApp.cancel_run(run_id)` — 取消 Run。
- `LevierApp.resume_run(run_id)` — 恢复 waiting Run。
- `Credentials` / `Secret` — 凭据数据类和不会在 repr/str 中泄漏的 key 包装。
- `load_credentials` / `save_credentials` / `default_credentials_path` / `resolve_server` / `credential_fingerprint` — 凭据读写、默认路径、server 解析和 key 指纹。
- `CredentialsError` / `LevierError` / `HTTPError` / `ProtocolError` / `TransportError` / `AppInactiveError` / `ToolExecutionError` — SDK 结构化错误类型。
- `WsToken` / `ToolSpec` / `PROTOCOL_VERSION` / `SDK_VERSION` — WebSocket token、工具描述和版本常量。
- `python app.py [--credentials PATH] [--server URL] [--mcp-ref CSV] [--max-calls N] [--idle-timeout SEC] [--ping-interval SEC] [--reconnect-initial SEC] [--reconnect-max SEC] [--log-level LEVEL] [--version]` — `LevierApp.run_cli` 提供的低层 App CLI。

---

## 5. Runtime SDK：入口与配置

- `Runtime(config=None, *, logger=None)` — 创建 Runtime；不传 config 时从 profile/env 推导。
- `Runtime.from_config(path=None, *, environ=None, logger=None)` — 读取 YAML；空文件或未配置时使用 profile 默认值。
- `Runtime.start(run, *, config=None, environ=None, handle_signals=True)` — 一行启动 Runtime，不需要 YAML 或显式 runtime token。
- `Runtime.run(run, *, handle_signals=True)` — 同步启动并阻塞。
- `Runtime.serve(run, *, handle_signals=True)` — 异步启动并阻塞。
- `Runtime.stop()` — 请求优雅停止。
- `RuntimeConfig(runtime_name, instance_id, middleware, runtime_type="normal", max_concurrent_runs=4, labels={}, log_level="info")` — Runtime 身份、容量和日志配置。
- `MiddlewareConfig(address, auth_token="", auth_token_file="", tls=False)` — 控制面 gRPC 连接配置。
- `load_runtime_config(path=None, *, environ=None)` — 加载 YAML/profile/env 并校验。
- `ConfigError` / `RuntimeUsageError` — 配置错误和 Runtime 使用错误。
- `RunFunction = Callable[[RunContext], Awaitable[str]]` — `run(ctx)` 的返回类型约定，终态为 `completed/failed/cancelled`，longterm Run 也可以返回 `waiting`。

---

## 6. Runtime SDK：`RunContext`

- `ctx.runtime_name` / `ctx.run_id` / `ctx.app_id` / `ctx.trace_id` / `ctx.attempt` — 当前 Run 的身份、追踪和 attempt 信息。
- `ctx.input` — 输入消息列表。
- `ctx.params` — 提交 Run 时传入的 Runtime 自定义参数。
- `ctx.model` — 控制面解析后的模型配置（provider/model/api_key/base_url/params），可能为 `None`。
- `ctx.tools` — 控制面下发的本地/global 工具描述列表。
- `ctx.mcp_bindings` — 控制面下发的 MCP 连接描述列表。
- `ctx.timeouts` — Run 超时和工具超时。
- `ctx.session_id` / `ctx.resume_from` — 可选 session 和 checkpoint 状态。
- `ctx.cancelled` / `ctx.cancel_event` — 取消状态和 asyncio 事件。
- `ctx.started_at` / `ctx.outcome` / `ctx.task` — 启动时间、当前结果和内部 task。
- `ctx.logger` — 自动带 run/app/runtime/attempt/trace 字段的 logger adapter。
- `await ctx.toolbox()` — 打开并返回本地+MCP 的统一 `Toolbox`，按 Run 缓存。
- `await ctx.chat(messages=None, *, tools=None, on_delta=None)` — 用 `ctx.model` 调模型；`tools=None` 时自动使用 toolbox。
- `await ctx.invoke(call, *, toolbox=None)` — 解析并执行一个模型 tool call。
- `await ctx.invoke_all(response, *, toolbox=None)` — 执行响应中的所有 tool calls，返回 `ToolResult` 列表。
- `await ctx.reply(text, *, role="assistant")` — 发一条完成的 assistant 消息。
- `await ctx.emit(event_type, data, *, ephemeral=False)` — 直接发事件，SDK 负责 seq 和 envelope。
- `await ctx.call_tool(name, arguments=None, *, timeout_ms=None, emit_events=True)` — 走 SDK 工具桥调用本地/global 工具。
- `await ctx.error(code, message, *, retryable=False, data=None)` — 发结构化 `ERROR` 事件。
- `await ctx.checkpoint(state)` — 保存 bytes 或 mapping 形式的 checkpoint，需要 `session_id`。
- `await ctx.wait(state)` — 保存 checkpoint 并返回 `waiting`，用于 longterm Run 暂停等待外部恢复。
- `await ctx.close_toolbox()` — 关闭该 Run 打开的 MCP session。

---

## 7. Runtime SDK：Toolbox、MCP、ReAct

- `await Toolbox.open(ctx)` — 聚合 `ctx.tools`、打开 `ctx.mcp_bindings`，返回可迭代的 `Toolbox`。
- `Toolbox.find(name)` — 按名字查找工具。
- `Toolbox.local()` — 只取本地/global 工具。
- `Toolbox.mcp()` — 只取 MCP 工具。
- `Toolbox.specs()` — 转成 LLM 侧工具描述列表。
- `await Toolbox.call(name, arguments=None)` — 统一调用入口，返回 `ToolResult`。
- `await Toolbox.close()` — 关闭所有 MCP session。
- `await Tool.call(arguments=None)` — 单个工具调用。
- `McpRuntimeClient(socket_path, *, default_timeout=30.0)` — MCP Runtime JSON-RPC 客户端，支持 Unix socket 和 `tcp://host:port`。
- `await McpRuntimeClient.health()` — 查询 MCP Runtime 健康。
- `await McpRuntimeClient.open(binding)` — 打开一个 MCP binding，返回 session 和 namespaced tools。
- `await McpRuntimeClient.call(session_id, tool_name, arguments_json, timeout_ms)` — 调用一个 MCP 工具。
- `await McpRuntimeClient.close_session(session_id)` — 关闭单个 session。
- `await McpRuntimeClient.close()` — 关闭客户端。
- `ProviderRouter(*, anthropic=None, openai=None)` — 按模型 provider 路由到 Anthropic 或 OpenAI-compatible 实现。
- `await ProviderRouter.complete(messages, tools, model, *, on_delta=None)` — 调用一次模型。
- `ReactAgent(provider, tools=None, *, default_step_timeout_ms=120000, default_tool_timeout_ms=60000)` — 可选 ReAct 循环；`tools=None` 时自动使用 `ctx.toolbox()`。
- `await ReactAgent.run(ctx)` / `ReactAgent(ctx)` — 执行一次 ReAct Run。
- `capture(...)` / `CheckpointState` / `CheckpointError` — ReAct checkpoint 编解码。
- `ToolProvider` / `ToolResult` — 自定义 ReactAgent 工具源的低层接口。
- `Message` / `ModelConfig` / `ToolSpec` / `McpBinding` / `Timeouts` / `ToolResult` — Runtime SDK 公开的 plain dataclass 类型。

---

## 8. Runtime 进程 CLI

- `levier-echo-runtime --config PATH --check --print-config --version --log-level LEVEL` — 内置最小 Echo Runtime。
- `levier-react-runtime --config PATH --check --print-config --version --log-level LEVEL` — 内置 ReAct Runtime；无 `--mcp-socket`，MCP 走 `ctx.toolbox()`。
- `python runtime.py` — 使用 `Runtime.start(run)` 的零配置启动方式。
- `levier runtime start [--file PATH] [--name NAME] [--token TOKEN] [--grpc ADDR] [--python PY] [--config PATH] [--log-level LEVEL]` — 由 Levier CLI 启动自定义 Runtime。
- `levier-mcp-runtime --socket PATH --addr HOST:PORT --stdio --secrets-file PATH --log-level LEVEL` — 独立 MCP Runtime，可同时监听 Unix socket 和 TCP，或走 stdio。

---

## 9. 控制面 HTTP / WebSocket 入口

- `GET /healthz` — 存活检查。
- `GET /readyz` — 就绪检查，依赖 PostgreSQL。
- `GET /meta` — 服务能力、版本和限制。
- `GET /status` — 健康、daemon、App、Run 汇总。
- `GET /metrics` — Prometheus 指标。
- `POST /api/v1/runs` — App key 提交 Run，支持 runtime_name/input/model/params/timeouts。
- `GET /api/v1/runs/{id}` — 查询 Run。
- `GET /api/v1/runs/{id}/events?after_attempt=&after_seq=&limit=` — 按游标补拉事件。
- `POST /api/v1/runs/{id}/cancel` — 取消 Run。
- `POST /api/v1/runs/{id}/resume` — 恢复 waiting longterm Run。
- `POST /api/v1/ws-token` — App key 换取短期 WebSocket token。
- `GET /api/v1/ws` — App WebSocket 连接。
- `POST /api/v1/apps` — 更新 App 元信息。
- `GET /api/v1/apps/{app_id}` — 查询当前 App。
- `GET /api/v1/model-providers` — App 可读的模型目录。
- `GET /api/v1/sessions/{id}` — 查询 session。
- `GET /api/v1/sessions/{id}/events` — 查询 session 事件。
- `POST /admin/apps` — 创建 App 并一次性返回 key。
- `GET /admin/apps` — 管理员列出 App。
- `POST /admin/apps/{id}/rotate-key` — 轮换 App key。
- `GET /admin/runs` — 管理员分页查询 Run。
- `GET /admin/runs/{id}` — 管理员查看单个 Run。
- `POST /admin/runs/{id}/cancel` — 管理员取消 Run。
- `POST /admin/runs/{id}/resume` — 管理员恢复 waiting Run。
- `GET /admin/daemons` — 管理员 daemon 视图。
- `GET /admin/tasks` — 管理员 workflow task 视图。
- `GET /admin/mcp` / `POST /admin/mcp` — 列出/写入 MCP Server。
- `DELETE /admin/mcp/{name}?force=` — 删除 MCP Server。
- `GET /admin/model-providers` / `POST /admin/model-providers` — 列出/写入 provider。
- `GET /admin/model-providers/{name}` / `DELETE /admin/model-providers/{name}` — 查询/删除 provider。
- `POST /admin/model-providers/{name}/check` — 检查 provider 和 secret。
- `PUT /admin/model-providers/{name}/secret` — 把 provider key 写入 managed secret 文件。

---

## 10. 配置与环境变量速查

- `~/.config/levier/config.yaml` 或 `%APPDATA%\levier\config.yaml` — 用户级 profile，保存 server、凭据路径、runtime、token 等。
- `.levier.json` — `levier init` 生成的当前项目配置，保存 app_name/credentials/server/runtime/mcp_refs。
- `LEVIER_HOME` — 覆盖 profile 和数据目录。
- `LEVIER_SERVER` — 默认 HTTP 地址。
- `LEVIER_ADMIN_TOKEN` — 管理命令 token。
- `LEVIER_CREDENTIALS` / `LEVIER_APP_CREDENTIALS` — App 凭据文件路径。
- `LEVIER_APP_NAME` — 默认 App 名。
- `LEVIER_APP_MCP_REFS` — 逗号分隔的 MCP 引用。
- `LEVIER_MODEL_API_KEY` — `model connect` 的 key 环境变量替代。
- `LEVIER_SECRET_<REF>` — 控制面读取的模型 secret。
- `LEVIER_SECRETS_FILE` — 控制面只读 secret JSON 文件。
- `LEVIER_SECRETS_WRITE_FILE` — 控制面 managed secret JSON 文件。
- `LEVIER_RUNTIME_CONFIG` — Runtime YAML 路径。
- `LEVIER_MIDDLEWARE_ADDRESS` — Runtime 连接控制面的 gRPC 地址。
- `LEVIER_RUNTIME_NAME` — Runtime 路由名。
- `LEVIER_RUNTIME_INSTANCE_ID` — Runtime 实例 ID。
- `LEVIER_RUNTIME_TOKEN` / `LEVIER_RUNTIME_TOKEN_FILE` — Runtime 认证 token 或其文件。
- `LEVIER_RUNTIME_TYPE` — Runtime 类型。
- `LEVIER_RUNTIME_MAX_CONCURRENT_RUNS` — Runtime 容量。
- `LEVIER_RUNTIME_LABELS` — Runtime 标签。
- `LEVIER_RUNTIME_LOG_LEVEL` — Runtime 日志级别。
- `LEVIER_MCP_SOCKET` / `LEVIER_MCP_ADDR` / `LEVIER_MCP_RUNTIME_SOCKET` — MCP Runtime 的 Unix socket 或 TCP 地址。
- `LEVIER_MCP_SECRETS_FILE` — MCP Runtime 的 secret JSON 文件。
- `LEVIER_STACK_FILE` — `levier up/down/logs/ps` 默认 Compose 文件。
- `LEVIER_PYTHON` — `levier dev` / `levier runtime start` 使用的 Python。
- `LEVIER_CONFIG` — `levier serve` 默认控制面 YAML。
- `LEVIER_GRPC` / `LEVIER_DAEMON_ID` / `LEVIER_DAEMON_TOKEN` — `daemon check` / `selfcheck` 的默认值。

---

## 11. 控制面部署配置

- `levier.example.yaml` — 控制面 YAML，包含 `http`、`grpc`、`database`、`workflow`、`retention`、`log` 六组配置。
- `LEVIER_DATABASE_URL` / `LEVIER_DATABASE_URL_FILE` — PostgreSQL DSN 或其 0600 文件。
- `LEVIER_HTTP_ADMIN_TOKEN` / `LEVIER_HTTP_ADMIN_TOKEN_FILE` — 控制面 Admin token 或其文件。
- `LEVIER_GRPC_DAEMON_TOKENS_FILE` — daemon token JSON 文件。
- `LEVIER_ENVIRONMENT` — `development` / `production`；生产模式会拒绝示例默认 token。
- `LEVIER_LOG_LEVEL` / `LEVIER_LOG_FORMAT` — 控制面日志级别和格式。
- `LEVIER_RETENTION_ENABLED` / `LEVIER_RETENTION_RUN_DAYS` / `LEVIER_RETENTION_SESSION_DAYS` / `LEVIER_RETENTION_BATCH_SIZE` — retention 清理开关和参数。
