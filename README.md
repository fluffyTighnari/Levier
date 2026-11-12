<!-- Middleware-Harness_Scheduler — repository landing page. -->

# Middleware-Harness_Scheduler

Middleware 是 Agent Run 的控制面与工具/事件中介层：

```text
App         业务函数 + 工具元数据 + 一条 WebSocket
Middleware  注册、调度、租约、工具路由、事件持久化
Harness     单次 Run 的 Agent 循环（当前 Python）
MCP Runtime 独立的 MCP 连接与会话进程（可选）
PostgreSQL  Run、事件、任务、App、工具、会话
```

当前主线是**单机 MVP**：

```text
单 Middleware + 单 PostgreSQL + 单 Harness + 单 App
runtime = normal
不追求并发/HA/多 Daemon
```

## 1. 现在能做什么

```text
App 注册与 App key 认证
Run 创建、排队、调度到 Harness、终态收敛
ReAct 循环、工具调用、usage 归一化
local / global / MCP 三种工具路径
事件持久化、WebSocket 回放与补拉
取消、Run/Step/Tool 超时
Harness 断线重排、MW 重启恢复、租约回收
```

## 2. 明确未完成

```text
sandbox runtime           未实现
longterm runtime          checkpoint 协议在 Harness 侧已有，MW 侧未闭环
多 Daemon / 多 MW / HA     不在 MVP 范围
压测、长时间浸泡           不在 MVP 范围
```

MVP 的调度语义是 **at-least-once**：Harness 崩溃后 Run 会从头重跑，
local 工具可能重复执行。接入 App 时，涉及副作用的工具应按 `invocation_id`
做幂等，或接受重复。

## 3. 五分钟启动

前置：

```text
Go 1.26+
Python 3.12+
uv
PostgreSQL 16+
curl
```

准备环境：

```sh
cp -n .env.example .env
# 编辑 .env：数据库 URL、MW_ADMIN_TOKEN；react 模式再加 DEEPSEEK_API_KEY

make build
cd harness && uv sync --locked && cd ..
```

启动 PostgreSQL。用本机 Homebrew 时注意端口，例如：

```sh
pg_ctl -D /opt/homebrew/var/postgresql@16 -o "-p 5433" start
# 并让 MIDDLEWARE_DATABASE_URL 指向 :5433
```

跑不需要模型 key 的 echo demo：

```sh
make mvp-demo
```

跑真实 DeepSeek + counter 工具：

```sh
make mvp-demo-react
```

成功输出类似：

```text
mode: react
run_id: 01M...
status: completed
events: 16
tool_calls: begin=3 end=3
errors: 0
final_message: The final counter value is **5**...
mvp-demo: PASS
```

日志与凭据临时目录在 `data/mvp-demo/`（已 gitignore，凭据在退出时删除）。

## 4. 测试

```sh
make lint
make build
make agent-core-test
make harness-test
make mcp-runtime-test
make counter-agent-test

# 真实 PostgreSQL；不设置会跳过 DB 集成用例
set -a; . ./.env.local; set +a
MIDDLEWARE_TEST_POSTGRES=1 go test -count=1 ./...
MIDDLEWARE_TEST_POSTGRES=1 go test -race -count=1 ./...
```

端到端回归：

```sh
make retrofit-e2e   # 真实 Harness + DeepSeek + local tools
make mcp-e2e        # MCP runtime + 凭据解析
```

## 5. 目录

```text
cmd/              middleware / migrate / daemon-check
internal/         控制面、存储、调度、网关、HTTP/WS
harness/          Python Harness
agent_core/       语言无关的 Agent 循环与 conformance fixtures
mcp_runtime/      独立 MCP 连接进程
examples/         counter_agent 等接入示例
scripts/          e2e、demo、护栏脚本
docs/             设计、契约、接入文档、报告
deploy/           容器镜像定义
```

## 6. 文档入口

| 文档 | 内容 |
|---|---|
| `docs/MVP.md` | 单机 MVP 的边界、启动、验收与限制 |
| `docs/ARCHITECTURE.md` | 一页架构与依赖方向 |
| `docs/APP_ONBOARDING.md` | App 接入流程 |
| `docs/DAEMON_CONTRACT.md` | Daemon 协议 |
| `docs/MCP.md` | MCP runtime |
| `docs/AGENT_CORE_CONTRACT.md` | Agent Core v1 契约 |
| `docs/reports/` | 每一步的实现与验证报告 |
