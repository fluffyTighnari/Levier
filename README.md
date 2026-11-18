<!-- Levier — usage-oriented landing page. -->

# Levier

Levier 是一个 **Agent Run 控制面**：你把业务能力声明成工具，把一次智能任务作为 Run 提交进来；调度、模型循环、工具路由、超时、取消和事件持久化都由 Levier 负责。

```text
你的 App      业务函数 + 工具声明 + 一条 WebSocket
Levier    注册、调度、租约、工具路由、事件持久化
Harness       单次 Run 的 Agent 循环（当前 Python）
MCP Runtime   独立的 MCP 连接/凭据进程（可选）
PostgreSQL    App、工具、Run、事件、任务、调用记录
```

> **仓库导航：`docs/MAP.md`；文档索引：`docs/README.md`。**
>
> **当前状态：单机 MVP。**
> 只支持 `runtime=normal`，单 Levier + 单 PostgreSQL + 单 Harness + 单 App。
> 调度语义是 **at-least-once**：Harness 崩溃后 Run 会从头重跑，本地工具可能重复执行。
> `sandbox`、`longterm`、多实例 / HA、工具 exactly-once 均不在当前范围内。

## 目录

- [1. 先判断要不要接入](#1-先判断要不要接入)
- [2. 架构与一次 Run](#2-架构与一次-run)
- [3. 五分钟跑通](#3-五分钟跑通)
- [4. 接入前准备](#4-接入前准备)
- [5. 改造现有 App（重点）](#5-改造现有-app重点)
- [6. App 侧协议速查](#6-app-侧协议速查)
- [7. 配置参考](#7-配置参考)
- [8. 部署](#8-部署)
- [9. 运维与排障](#9-运维与排障)
- [10. 测试与回归](#10-测试与回归)
- [11. 目录与文档](#11-目录与文档)
- [12. 当前边界](#12-当前边界)

---

## 1. 先判断要不要接入

Levier 不是模型 SDK 的替代品，它解决的是 **“谁来跑循环、谁来调度、工具怎么回到业务代码、Run 怎么被观测和收敛”**。

| 你的现状 | 建议 |
|---|---|
| 已经有一个稳定的进程内 Agent 循环，只需要单机演示 | 不必接入 |
| 想把模型调用、ReAct 循环从业务进程里拿出去 | 接入 |
| 多个业务或租户需要统一调度、隔离工具和事件 | 接入 |
| 需要 Run 可取消、可超时、可回放、可审计 | 接入 |
| 只是想把几个函数包装成模型可调用的工具 | 直接用模型 SDK 更轻 |

接入后，**你仍然负责**：业务函数、业务数据、租户隔离、用户鉴权、前端交互、副作用补偿。

**Levier 负责**：App/工具注册、Run 排队与调度、模型的工具调用路由、事件落库与补拉、租约与故障恢复。

---

## 2. 架构与一次 Run

```text
App 后端 ── 1. 提交 Run（HTTP）──▶ Levier ── 2. 调度（gRPC）──▶ Harness
   ▲                                    │                           │
   │                                    │                           ├─ 3. 调模型
   │◀── 5. local_tool_call（WS）─────────┘                           └─ 4. 返回 tool_call
   ├─ 6. 执行业务函数
   └── 7. local_tool_result（WS）──▶ Levier ── 8. 回填──────────▶ Harness
                                                                     │
                                                             9. Run 终态
```

### 2.1 组件职责

| 组件 | 负责 | 不负责 |
|---|---|---|
| App | 业务函数、工具声明、一条 WebSocket、提交 Run | 不跑模型循环，不决定工具调用顺序 |
| Levier | 注册表、调度、租约、工具路由、幂等、事件持久化 | 不执行 App 业务代码，不代理 MCP 调用 |
| Harness | 单次 Run 的 ReAct 循环、模型适配、工具桥 | 不接触 App 明文密钥，不持久化业务数据 |
| MCP Runtime | MCP 连接、会话、stdio 子进程、凭据解析 | 不直连 Levier，不写 Run 事件 |
| PostgreSQL | App、工具、Run、事件、任务、调用记录 | 不承担业务数据存储 |

### 2.2 一次带工具调用的 Run

1. App 用 App key 提交 Run。
2. Levier 在一个事务里创建 Run 和调度/超时任务，返回 `202` 和 `run_id`。
3. 调度器把 Run 分配给一台声明了对应策略的 Harness。
4. Harness 调用模型。
5. 模型返回工具调用，Harness 把请求交回 Levier。
6. Levier 按 `app_id` 把调用路由到该 App 的 WebSocket。
7. App 执行原有业务函数，把结果回给 Levier。
8. Levier 落库并回填给 Harness，模型进入下一轮。
9. Run 到达终态，事件可查询、可回放，也可通过 WebSocket 实时推送。

### 2.3 三种工具路径

| 路径 | 谁执行工具 | 典型场景 | 是否经过 Gateway | 是否记录调用 |
|---|---|---|---|---|
| `local` | App，经 WebSocket | 业务系统内部函数、数据库读写、租户内操作 | 是 | `tool_invocations` |
| `global` | Levier 的 http/webhook executor | 通用 HTTP 接口、跨 App 的公共能力 | 是 | `tool_invocations` |
| `MCP` | 独立 `mcp_runtime` 连接的 MCP Server | GitHub、文件系统、第三方 MCP 工具 | 否 | 只有 Harness 事件 |

大多数业务接入只需要 `local`。已有 MCP Server 时优先走 MCP，不必为它改 App 代码。

---

## 3. 五分钟跑通

### 3.1 Docker Compose（推荐）

前置：Docker Desktop / Docker Engine + Compose v2。

```sh
cp -n .env.example .env
# 至少检查 .env：
#   LEVIER_DATABASE_URL 由 compose 覆盖，本地可保持默认
#   LEVIER_ADMIN_TOKEN          管理凭据，仅用于创建/轮换 App
#   DEEPSEEK_API_KEY        只在跑真实模型时需要

docker compose up --build -d
docker compose ps
```

> Windows PowerShell 下把 `cp -n .env.example .env` 换成
> `Copy-Item .env.example .env -ErrorAction SilentlyContinue` 即可；`docker compose` 命令不变。

首次启动会依次完成：

```text
postgres 就绪
→ levier 执行 migrate up，然后 serve
  （serve 启动时检查 pending migration）
→ mcp-runtime 创建 /run/levier/mcp.sock
→ harness 连接 levier，注册为 daemon-local-1
→ agent 等待 /readyz，bootstrap App，注册工具，建立 WebSocket
```

跑一次不依赖模型的端到端 smoke：

```sh
docker compose --profile smoke run --rm smoke
```

成功时最后一行是：

```text
docker-smoke: PASS
```

它覆盖了：Levier 健康检查、Harness 注册、App bootstrap、App WebSocket、MCP runtime socket、提交一个 echo Run 并到达 `completed`。

跑真实模型 + 本地工具：

```sh
# 在 .env 里设置 DEEPSEEK_API_KEY 后
docker compose up --build -d

docker compose exec agent python3 /app/examples/counter_agent/submit_run.py \
  --credentials /data/credentials.json \
  --mw-url http://levier:8080 \
  --strategy react \
  --provider deepseek \
  --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter."
```

查看日志与清理：

```sh
docker compose logs -f
docker compose down          # 保留数据卷
docker compose down -v       # 删除 PostgreSQL、凭据和 socket 卷
```

### 3.2 本地开发

前置：Go 1.26+、Python 3.12+、`uv`、PostgreSQL 16+、`curl`。
Windows 建议使用 WSL / Git Bash 执行 Makefile，或直接用 Docker Compose。

```sh
cp -n .env.example .env
# 编辑 .env：数据库 DSN、LEVIER_ADMIN_TOKEN
# 跑 react 模式再加 DEEPSEEK_API_KEY / DEEPSEEK_BASE_URL / DEEPSEEK_MODEL

make build
(cd execution/harness && uv sync --locked)
```

启动 PostgreSQL（示例为本机 Homebrew，端口 5433；请让 `.env` 的 DSN 指向实际端口）：

```sh
pg_ctl -D /opt/homebrew/var/postgresql@16 -o "-p 5433" start
```

跑不需要模型 key 的 echo 链路：

```sh
make mvp-demo
```

跑真实 DeepSeek + 本地 counter 工具：

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

临时日志、凭据和 summary 在 `data/mvp-demo/`，已 gitignore；凭据在退出时删除。

故障矩阵输出在 `data/fault-matrix/`。

### 3.3 手动逐组件启动（理解链路时用）

先加载本地环境（Windows PowerShell 请改用 `$env:NAME="value"` 或直接在 `.env` 中配置后交给 Compose）：

```sh
set -a; . ./.env; set +a
```

然后按顺序启动。下面标了“启动”的长期运行命令建议各开一个终端（或自行放到后台）：

```sh
# 1. 迁移数据库
go run ./cmd/levier migrate up -config levier.example.yaml

# 2. 启动 Levier（HTTP :8080，gRPC :9090）
bin/levier serve -config levier.example.yaml

# 3. 启动 Harness
(cd execution/harness && PYTHONPATH=src:src/gen .venv/bin/python -m levier_harness \
  --config levier.example.yaml)

# 4. 可选：启动 MCP runtime
(cd execution/mcp_runtime && uv sync --locked && \
  PYTHONPATH=src .venv/bin/python -m levier_mcp_runtime \
  --socket /tmp/levier-mcp.sock)

# 5. 启动参考 App（bootstrap + 注册工具 + 连 WS）
python3 examples/counter_agent/bootstrap_app.py \
  --credentials-out ~/.config/middleware/counter.json
python3 examples/counter_agent/serve.py \
  --credentials ~/.config/middleware/counter.json \
  --mw-url http://127.0.0.1:8080

# 6. 提交 Run
python3 examples/counter_agent/submit_run.py \
  --credentials ~/.config/middleware/counter.json \
  --strategy react \
  --provider deepseek --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter."
```

### 3.4 确认链路健康

```sh
curl -s http://127.0.0.1:8080/healthz      # {"status":"ok"}
curl -s http://127.0.0.1:8080/readyz       # {"status":"ready","database":"up"}
curl -s http://127.0.0.1:8080/metrics | grep -E 'levier_daemon_capacity|levier_ws_connections'
```

预期能在指标里看到：

```text
levier_daemon_capacity{daemon="daemon-local-1",...}
levier_ws_connections{app="app_..."} 1
```

前者说明 Harness 已注册，后者说明 App 的长连接在线。

### 3.5 日常 CLI

常用命令：

```sh
levier status
levier doctor --credentials /path/to/app.json
levier app show --credentials /path/to/app.json
levier run submit --credentials /path/to/app.json --strategy echo --prompt "hello" --watch
```

完整命令、参数、环境变量、JSON 字段和退出码见
[`docs/CLI_REFERENCE.md`](docs/CLI_REFERENCE.md)。

从 release 安装 CLI 见
[`docs/INSTALL.md`](docs/INSTALL.md)。

---

## 4. 接入前准备

### 4.1 三个凭据各管什么

| 凭据 | 谁持有 | 用途 | 生命周期 |
|---|---|---|---|
| `LEVIER_ADMIN_TOKEN` | 运维 / 部署系统 | 创建 App、轮换 App key、注册 MCP | 只用于 bootstrap，不进运行时 |
| `APP_ID` + `APP_KEY` | 你的 App 服务 | 验证身份、注册工具、提交 Run、换 ws_token | 长期，`APP_KEY` 只在创建/轮换时返回一次 |
| `ws_token` | 你的 App 进程内存 | 建立 App WebSocket | 短期，默认 10 分钟 |

接入前你需要确定：

```text
LEVIER_URL        Levier 的 HTTP 地址（非秘密）
APP_NAME      App 自己选定的全局唯一名称
```

运行时只需要：

```text
APP_ID
APP_KEY
LEVIER_URL
```

### 4.2 接入的最小能力

任何语言都可以接入，只要能做到：

```text
发 HTTP 请求、维持一条 WebSocket
解析和生成 JSON
安全保存 APP_ID / APP_KEY
把 arguments_json / result_json 当字符串处理，而不是对象
常驻运行，不因一次工具调用结束就退出
```

不需要 SDK、生成代码或特定目录结构。

### 4.3 选择接入粒度

- **一个产品一个 App**：默认选择，工具名在 App 内唯一。
- **一个租户一个 App**：租户之间工具、凭据、事件天然隔离，推荐给多租户 SaaS。
- **不要用同一个 `app_id` 起多个 App 实例**：Levier 会把工具调用广播到该 App 的所有连接，只有一个实例能正确回包。需要多实例时，先设计连接归属（例如每个实例独立 App，或只让 leader 持有连接）。
- 工具数量建议按业务域拆分，单 App 保持在模型容易选择的规模；工具太多时优先拆成多个 App 或按场景分组。

---

## 5. 改造现有 App（重点）

这一节只讲 **怎么把已有代码接进来**。完整协议细节见 `docs/APP_ONBOARDING.md`，这里给的是改造顺序和决策依据。

### 5.1 改造的核心：把循环拿走，把业务留下

```text
改造前
用户请求 → 你的 App
             ├─ 拼 prompt
             ├─ 自己调模型
             ├─ 解析 tool_call
             ├─ 自己执行函数
             ├─ 把结果塞回上下文
             └─ 循环到模型停止

改造后
用户请求 → 你的 App 后端 ──提交 Run──▶ Levier ──▶ Harness ──▶ 模型
             ▲                                         │
             │◀──── local_tool_call（WebSocket）────────┘
             └─ 执行原有业务函数 ──▶ local_tool_result
```

代码层面的处理原则：

| 原有代码 | 怎么处理 |
|---|---|
| 业务 service / domain 函数 | **保留**，它就是工具的真实实现 |
| 模型 SDK 调用、API key 管理 | **移除**，模型调用和 key 都在 Harness / 部署环境 |
| ReAct / function-calling 循环 | **移除**，由 Harness 的 `react` 策略承担 |
| prompt 拼装、system message | **保留**，放到提交 Run 的 `input` 里 |
| 工具 schema / function 定义 | **保留**，改成 Levier 的工具注册格式 |
| 工具执行的 switch / router | **保留**，改成从 WebSocket 消息分发 |
| 对话历史 | 当前 MVP 由 `input` 显式传入；`longterm` 续跑未闭环 |
| 前端流式输出 | 改为订阅 WebSocket 事件，或轮询 Run 状态 |
| 用户鉴权、租户校验 | **保留在 App 入口**；Levier 只认 App key |

### 5.2 改造步骤

#### 第 1 步：冻结基线，保留可回滚路径

先把现有 Agent 路径保留为 feature flag 或独立入口，记录几条基线输入和期望结果。

接入后用它做 A/B 对照；回滚时只需要把提交入口切回旧路径并停掉 WebSocket 服务进程。

#### 第 2 步：盘点并切分工具

把现有业务能力分成“适合模型调用”和“不适合模型调用”两类。

适合：

```text
查询类：查订单、查库存、查用户资料、检索知识库
计算类：统计、换算、规则校验
操作类：创建工单、更新状态、发送通知
组合类：内部服务已有的原子能力
```

不适合，或需要先改造：

```text
需要人类确认的支付/删除/审批 → 拆成 prepare + confirm 两步
超过工具超时的长任务         → 先返回 job_id，再提供查询工具
返回巨大对象                → 分页、摘要、只返回必要字段
需要模型推理的步骤           → 留在 Harness，不要塞进工具
```

每个工具至少声明：

```text
name           App 内唯一，建议 snake_case
description    给模型看：做什么、什么时候用、参数含义、返回什么、不要用于什么
schema         JSON Schema，严格限制类型、枚举和必填项
timeout_ms     该工具的截止时间，必须 ≤ 240000
```

工具名一旦上线就当作稳定契约；改名等同于新工具。

#### 第 3 步：加一层工具适配层

不要改业务函数本身，在它们外面加一层“协议适配”：

```text
收到 local_tool_call
  → 把 arguments_json 解析一次（它已经是字符串）
  → 校验 tool_name 是否属于自己注册过的工具
  → 校验参数类型、范围、枚举
  → 调用原有业务函数
  → 把返回值序列化成 JSON 字符串
  → 按原 msg_id / invocation_id 回包
```

适配层要遵守：

- **参数校验自己做**。模型给的参数不可信，不要依赖模型遵守 schema。
- **结果只返回必要信息**。不要把整张表、整个对象、堆栈或密钥返回给模型。
- **错误用稳定错误码**。建议 `code` 稳定、`message` 可读、`retryable` 明确；不要回显内部异常。
- **不要在工具里再调模型**，也不要再提交一个 Run；这会造成嵌套循环。
- **不要在工具里做超长阻塞**。超过 `timeout_ms` 会返回 `TOOL_TIMEOUT`，模型只能看到失败。
- **并发要隔离**。一条连接可以顺序处理，也可以并发处理；但不要让一个慢工具阻塞心跳、事件和后续调用。
- **重复回包会被忽略**，但不要主动重发成功结果；重发时 `correlation_id` 必须仍是原 `msg_id`。

#### 第 4 步：一次性注册 App 和工具

**只做一次**（部署初始化或运维脚本）：

```http
POST {LEVIER_URL}/admin/apps
Authorization: Bearer {LEVIER_ADMIN_TOKEN}
Content-Type: application/json

{"name": "{APP_NAME}", "multiagent": false}
```

- `201`：新建成功，响应里的 `api_key` 只返回这一次。
- `200`：App 已存在，`api_key` 为空；需要轮换时调用

```http
POST {LEVIER_URL}/admin/apps/{APP_ID}/rotate-key
Authorization: Bearer {LEVIER_ADMIN_TOKEN}
```

把 `app_id` / `app_name` / `api_key` 写进 0600 凭据文件。**Admin token 不要进入 App 的长期运行环境。**

**每次启动**（或每次工具变更发布时）注册工具：

```http
POST {LEVIER_URL}/api/v1/apps
Authorization: Bearer {APP_KEY}
Content-Type: application/json

{
  "name": "{APP_NAME}",
  "multiagent": false,
  "tools": [
    {
      "name": "counter_add",
      "description": "Add an integer delta to the counter.",
      "schema": {
        "type": "object",
        "properties": {"delta": {"type": "integer"}},
        "required": ["delta"],
        "additionalProperties": false
      },
      "timeout_ms": 10000
    }
  ]
}
```

规则：

```text
tools 是全量替换：新增工具必须提交完整列表
"tools": [] 清空；不传 tools 不修改
schema 为空会补成 {}
重名会提升 version
```

不要在每次用户请求时注册工具，也不要把注册当成健康检查。

#### 第 5 步：建立并维持 App WebSocket

启动顺序建议固定为：

```text
读取凭据
→ 验证 App key（GET /api/v1/apps/{app_id}，200 且 status=active）
→ 注册工具
→ 换取短期 ws_token
→ 建立 WebSocket
→ 进入消息循环
```

换 token：

```http
POST {LEVIER_URL}/api/v1/ws-token
Authorization: Bearer {APP_KEY}
```

连接：

```text
GET {LEVIER_URL}{ws_url}?app_id={APP_ID}&token={ws_token}
Upgrade: websocket
```

连接管理的要点：

- `ws_token` 短期有效，只放内存；重连时重新换，不要缓存到磁盘。
- 断线要退避重连，不要忙等；重连后继续服务**新**调用。
- 订阅事件是可选能力，**不是接收工具调用的前提**。
- 优雅退出时停止接收新调用，让在途调用完成或按超时失败，再关闭连接。
- 不要让 WebSocket 读循环被慢业务阻塞；用队列/线程池把执行与读分开。
- 一个 `app_id` 只对应一个逻辑连接（多实例问题见 4.3）。

#### 第 6 步：把用户入口改成提交 Run

提交方应该是 **App 后端**，不要把 App key 放进浏览器或移动端。

```http
POST {LEVIER_URL}/api/v1/runs
Authorization: Bearer {APP_KEY}
Content-Type: application/json
Idempotency-Key: {可选但建议}
```

```json
{
  "runtime": "normal",
  "strategy": {"name": "react", "params": {"max_steps": 8}},
  "input": [
    {"role": "system", "content": "你是客服助手，只能操作当前租户的数据。"},
    {"role": "user", "content": "帮我把订单 A-100 的收货地址改到杭州。"}
  ],
  "model": {"provider": "deepseek", "model": "deepseek-flash"},
  "timeouts": {"run_ms": 60000, "tool_ms": 10000}
}
```

响应：

```json
{
  "run_id": "01...",
  "status": "queued",
  "ws_url": "/api/v1/ws",
  "ws_token": "..."
}
```

字段选择建议：

| 字段 | 怎么选 |
|---|---|
| `runtime` | MVP 固定 `normal` |
| `strategy.name` | `echo` 只用于连通性测试；真实任务用 `react` |
| `strategy.params.max_steps` | 限制循环步数，避免模型反复试错 |
| `input` | 把原有 system prompt 和对话历史原样放进 messages |
| `model` | 由后端选择；模型凭据在 Harness 部署环境，不在请求里 |
| `timeouts.run_ms` | 整个 Run 的预算，默认 30 分钟 |
| `timeouts.tool_ms` | 单次工具调用上限，默认 60 秒，最大 240 秒 |
| `Idempotency-Key` | 同一次用户操作重试时复用；内容不同会 `409` |

工具清单**不要**在 Run 里传，Levier 会按 `app_id` 从注册表组装。

调用方只需要拿到 `run_id`；`ws_token` 可以交给同一个后端进程用于订阅事件，也可以不下发。

#### 第 7 步：消费结果并做终态处理

两种方式可以并存：

**轮询**

```http
GET {LEVIER_URL}/api/v1/runs/{run_id}
GET {LEVIER_URL}/api/v1/runs/{run_id}/events?after_attempt=&after_seq=&limit=
```

**WebSocket 订阅**

```json
{"type": "subscribe", "run_id": "01...", "last_attempt": 0, "last_seq": 0}
```

订阅后服务端会先补历史事件，再回 `caught_up`，之后实时推 `event`。断线重连时用上次的 `attempt` / `seq` 续传。

终态：

```text
completed / failed / cancelled / timeout
```

最终回答在 `MESSAGE_COMPLETED` 事件的 `data.content` 里；`MESSAGE_DELTA` 是流式片段，不落库、不参与回放。

需要取消时：

```http
POST {LEVIER_URL}/api/v1/runs/{run_id}/cancel
```

`failed` / `timeout` / `cancelled` 都要有明确的业务兜底，不要把“没有收到 completed”当成成功。

#### 第 8 步：灰度、对照、回滚

1. 先用 `echo` 策略验证注册、连接、提交、终态，不消耗模型额度。
2. 再用真实模型跑固定脚本，对比改造前的工具调用顺序和结果。
3. 按租户或用户灰度切流，保留旧路径。
4. 观察 `TOOL_CALL_BEGIN` / `TOOL_CALL_END` 是否成对、`ERROR` 是否为 0、Run 是否稳定到达 `completed`。
5. 回滚：切回旧入口，停止 App 的 WebSocket 服务进程；业务函数不需要回滚。

### 5.3 工具设计规则

- **一个工具只做一件事**。不要把“查订单 + 改地址 + 发短信”塞进一个工具。
- **description 是给模型看的**。写清楚使用时机、参数单位、返回结构、禁止场景。
- **schema 要严格**。`type: object`、显式 `required`、`additionalProperties: false`、能枚举就枚举。
- **参数用业务语义命名**。`order_id` 比 `id` 好，`delta` 比 `v` 好。
- **结果要小**。返回模型决策所需字段；列表分页；大文本摘要。
- **超时要真实**。`timeout_ms` 覆盖 P99，且不要超过 240 秒。
- **写操作要可补偿**。删除、支付、通知等副作用必须能撤销或去重。
- **工具名稳定**。改名、改语义都按新工具发布，避免历史 Run 回放时语义漂移。

### 5.4 幂等与副作用

当前语义是 **at-least-once**：

```text
同一次调用被重复投递 / App 重复回包
  → Levier 按 invocation_id 去重

Harness 崩溃后 Run 从头重跑
  → 新 attempt、新的工具调用、新的 invocation_id
  → 业务副作用可能再次发生
```

所以有副作用的工具需要二选一：

```text
A. 用业务幂等键去重（例如 order_id + action + 业务版本号）
B. 接受重复，并提供人工或自动补偿
```

只按 `invocation_id` 做幂等，只能覆盖同一次调用的重复投递，**不能覆盖跨 attempt 的重跑**。这一点在改造支付、扣库存、发消息类工具时尤其重要。

### 5.5 用户与租户上下文

`local_tool_call` 只带 `tool_name`、`arguments_json`、`invocation_id`、`msg_id`、`timeout_ms`，**不带 `run_id` 或用户身份**。因此：

| 方案 | 适用 | 注意 |
|---|---|---|
| 一个租户一个 App | 多租户 SaaS，推荐 | 隔离最清晰，工具和事件天然分开 |
| 业务标识放进工具参数 | 单 App 多用户 | 模型能看到这些字段，注意隐私 |
| App 内维护“当前用户”映射 | 单用户/单会话实例 | 并发和多实例下容易串号，不推荐 |
| 用 Run 的提交方做映射 | 后端自己提交 | 需要自己维护 run 与用户的关联表 |

不要把用户 token、cookie、内部密钥放进工具参数交给模型。

### 5.6 按现有 App 形态选择改造方式

| 现有形态 | 推荐做法 |
|---|---|
| 传统 REST 服务 | 把 controller 里的业务调用抽到 service 层；工具体直接调 service，不要为了复用再绕一次公网 HTTP |
| 已有 Chat / WebSocket 服务 | 新增一条到 Levier 的 WebSocket，不要复用面向用户的连接；消息类型做命名空间隔离 |
| CLI / 批处理 | 把“一次执行”封装成工具；服务进程必须常驻以维持 WebSocket，不能执行完就退出 |
| 多租户 SaaS | 每租户一个 App，或至少每租户独立凭据；不要用一个 App 承载所有租户 |
| 长耗时任务 | 工具立即返回 `job_id`，另提供查询工具；不要让调用阻塞到超时 |
| 有状态单例 | 单实例部署；多实例会导致状态不一致，且调用会广播到所有连接 |
| 已有 MCP Server | 不用改 App；在 Levier 注册 MCP，App 通过 `mcp_refs` 引用 |
| 已有大量函数工具 | 先按业务域拆 App，再控制单 App 工具数量；不要一次把几百个函数都交给模型 |

### 5.7 明确不要做的事

```text
不要在工具里再调模型，或再提交一个 Run
不要把 arguments_json 当对象再解析一次
不要回包后断开连接
不要用新的 msg_id 回包
不要以为必须订阅 Run 才能收到工具调用
不要在 Run 请求里传工具清单
不要用同一个 app_id 起多个实例同时服务
不要把 App key / Admin token 写进前端、日志、git
不要在工具结果里返回密钥、堆栈或超大对象
不要把长任务硬扛到工具超时
```

### 5.8 改造验收清单

```text
[ ] GET /healthz、/readyz 正常
[ ] 用 App key 验证身份返回 200 且 status=active
[ ] 工具注册成功，Run 中模型能调用到
[ ] levier_ws_connections{app="..."} 为 1
[ ] echo Run 能 completed
[ ] react Run 的 TOOL_CALL_BEGIN 与 TOOL_CALL_END 成对
[ ] 工具结果能被模型引用，最终回答正确
[ ] 有副作用工具在重复调用下不会产生重复业务结果
[ ] 断线重连后新 Run 正常
[ ] 取消和超时都能收敛到终态
[ ] 日志、事件、错误信息中没有 App key / ws_token / 模型 key
```

---

## 6. App 侧协议速查

### 6.1 HTTP 接口

| 方法 | 路径 | 认证 | 用途 |
|---|---|---|---|
| `GET` | `/healthz` | 无 | 存活检查 |
| `GET` | `/readyz` | 无 | 就绪检查（含数据库） |
| `POST` | `/admin/apps` | Admin token | 创建 App，返回一次性 key |
| `POST` | `/admin/apps/{app_id}/rotate-key` | Admin token | 轮换 App key |
| `GET` | `/api/v1/apps/{app_id}` | App key | 验证身份与状态 |
| `POST` | `/api/v1/apps` | App key | 全量更新工具 / MCP 引用 |
| `POST` | `/api/v1/ws-token` | App key | 换取短期 WebSocket token |
| `GET` | `/api/v1/ws` | ws_token | App WebSocket |
| `POST` | `/api/v1/runs` | App key | 提交 Run |
| `GET` | `/api/v1/runs/{run_id}` | App key | 查询 Run |
| `GET` | `/api/v1/runs/{run_id}/events` | App key | 回放事件 |
| `POST` | `/api/v1/runs/{run_id}/cancel` | App key | 取消 Run |
| `GET` | `/api/v1/sessions/{session_id}` | App key | 查询长期会话（MVP 未闭环） |
| `GET` | `/api/v1/sessions/{session_id}/events` | App key | 回放会话事件（MVP 未闭环） |

所有错误都返回统一信封，应读取 `error.code`，不要只看 HTTP 状态码：

```json
{
  "error": {
    "code": "TOOL_TIMEOUT",
    "message": "local tool timed out",
    "retryable": true,
    "request_id": "01..."
  }
}
```

### 6.2 WebSocket 消息

客户端可发送：

| `type` | 作用 |
|---|---|
| `hello` | 声明协议版本（`protocol: 1`），可选 |
| `ping` | 保活，服务端回 `pong` |
| `subscribe` | 订阅某个 Run 的事件，可带 `last_attempt` / `last_seq` 续传 |
| `local_tool_result` | 回传工具执行结果 |

服务端会发送：

| `type` | 作用 |
|---|---|
| `local_tool_call` | 请求 App 执行本地工具 |
| `event` | Run 事件 |
| `run_completed` | Run 到达终态 |
| `caught_up` | 历史事件补发完成 |
| `pong` | 保活响应 |
| `error` | 协议或路由错误 |

工具调用与结果：

```json
{
  "type": "local_tool_call",
  "msg_id": "01...",
  "invocation_id": "01...",
  "tool_name": "counter_add",
  "arguments_json": "{\"delta\": 2}",
  "timeout_ms": 10000
}
```

```json
{
  "type": "local_tool_result",
  "correlation_id": "01...",
  "invocation_id": "01...",
  "ok": true,
  "result_json": "{\"value\": 2}"
}
```

```json
{
  "type": "local_tool_result",
  "correlation_id": "01...",
  "invocation_id": "01...",
  "ok": false,
  "error": {
    "code": "TOOL_EXEC_ERROR",
    "message": "delta must be an integer",
    "retryable": false
  }
}
```

强制规则：

```text
arguments_json / result_json 是字符串，不是 JSON 对象
correlation_id 必须等于收到的 msg_id
ok=true 时不能带 error
ok=false 时必须带 error
同一结果重复发送会被静默忽略
```

### 6.3 事件与终态

```json
{
  "type": "event",
  "run_id": "01...",
  "attempt": 1,
  "seq": 5,
  "source": "daemon",
  "event_type": "TOOL_CALL_END",
  "data": {"tool_name": "counter_add", "ok": true, "result_json": "{\"value\": 2}"},
  "ts_ms": 1794368388919
}
```

常见事件类型：

```text
RUN_STATUS         Run 状态变化
MESSAGE_DELTA      流式文本片段（ephemeral，不落库）
MESSAGE_COMPLETED  模型最终回答
TOOL_CALL_BEGIN    工具调用开始
TOOL_CALL_END      工具调用结束
USAGE              token 用量
ERROR              错误
```

补拉游标是 `(attempt, seq)`。只有持久事件消耗 `seq`；`MESSAGE_DELTA` 不落库。
Run 终态会以 `run_completed` 推送，`status` 为 `completed / failed / cancelled / timeout`。

### 6.4 错误与重试矩阵

| 现象 | 服务端行为 | App 应该做什么 |
|---|---|---|
| App 未连接 | `APP_UNREACHABLE`，可重试 | 重连后继续服务新调用 |
| 工具超时未回 | `TOOL_TIMEOUT`，可重试 | 检查是否卡死或超时设置过小 |
| 工具未注册 | `TOOL_NOT_FOUND` | 核对工具名与注册列表 |
| 跨 App 调用 | `TOOL_DENIED` / `LOCAL_TOOL_FORBIDDEN` | 检查 `app_id` 与凭据 |
| App 返回 `ok=false` | 错误码透传给模型 | 使用稳定、可读的 `code` |
| `correlation_id` 不匹配 | `LOCAL_TOOL_CORRELATION_MISMATCH` | 用原 `msg_id` 重发，不要换 id |
| 重复回包 | 静默忽略 | 不要主动重发成功结果 |
| 非法消息 | `BAD_MESSAGE`，连接保留 | 修正 JSON 结构 |

---

## 7. 配置参考

### 7.1 环境变量

`.env` 供本地 demo 和 Compose 使用：

| 变量 | 用途 |
|---|---|
| `LEVIER_DATABASE_URL` | Levier 的 PostgreSQL DSN |
| `LEVIER_TEST_DATABASE_URL` | 测试库 DSN |
| `LEVIER_ADMIN_TOKEN` | Admin token，必须与配置里的 `http.admin_token` 一致 |
| `LEVIER_APP_NAME` | Compose 中 agent 的 App 名称 |
| `DEEPSEEK_API_KEY` | Harness 调用 DeepSeek 的 key |
| `DEEPSEEK_BASE_URL` | 模型网关地址 |
| `DEEPSEEK_MODEL` | 默认模型名 |
| `POSTGRES_PASSWORD` | Compose 中 PostgreSQL 密码 |
| `MCP_E2E_TOKEN` | MCP 回归用的 secret |

> 旧名 `MIDDLEWARE_*` / `MW_*` 在一个版本内仍兼容；新部署请使用 `LEVIER_*`。

### 7.2 Levier 配置（`levier.example.yaml`）

```yaml
http:
  addr: ":8080"
  admin_token: "admin-change-me"
  ws_token_ttl: "10m"
  allow_legacy_bearer: false      # 生产保持 false

grpc:
  addr: ":9090"
  daemon_tokens:
    daemon-local-1: "change-me"   # 预分配 daemon 身份

database:
  url: "postgres://..."
  max_conns: 20

workflow:
  workers: 4
  lease_ttl: "30s"
  max_attempts: 5
  event_buffer: 256

log:
  level: "info"
  format: "json"
```

生产部署前必须替换 `change-me` 类占位符，并为每个 daemon 分配独立 token。

### 7.3 Harness 配置（`execution/harness/levier.example.yaml`）

```yaml
middleware:
  address: "127.0.0.1:9090"
  tls: false

mcp_runtime:
  socket: "/tmp/levier-mcp.sock"

daemon:
  id: "daemon-local-1"
  auth_token: "change-me"
  runtime_type: "normal"
  max_concurrent_runs: 4
  strategies: ["react", "echo"]
  labels:
    pool: "local"

logging:
  level: "info"
  format: "json"
```

`daemon_id` 与 token 必须先在 Levier 配置里预分配；Harness 不能自助注册。
不声明 `supported_strategies` 的 Harness 永远不会拿到对应策略的任务。

### 7.4 Compose 默认值

| 变量 | 默认值 |
|---|---|
| `POSTGRES_PASSWORD` | `change-me` |
| `LEVIER_ADMIN_TOKEN` | `admin-change-me` |
| `LEVIER_APP_NAME` | `counter-agent` |
| `MCP_E2E_TOKEN` | `mcp-e2e-secret` |
| daemon token | `change-me`（本地 demo 固定值） |

这些都是本地演示配置，共享主机或生产环境必须替换。

---

## 8. 部署

### 8.1 Compose 栈

```text
postgres      PostgreSQL 16
levier        Go 控制面（8080 HTTP / 9090 gRPC）
mcp-runtime   MCP 连接进程（共享 /run/levier/mcp.sock）
harness       Agent 循环（daemon-local-1）
agent         counter_agent 参考 App
smoke         一次性 smoke 容器（profile）
```

`deploy/` 下还有各镜像的 Dockerfile；也可以单独构建：

```sh
docker build -f deploy/levier.Dockerfile -t levier:dev .
docker build -f deploy/harness.Dockerfile -t levier-harness:dev .
docker build -f deploy/mcp-runtime.Dockerfile -t levier-mcp-runtime:dev .
docker build -f deploy/agent.Dockerfile -t levier-agent:dev .
```

### 8.2 数据与密钥

Compose 使用三个命名卷：

```text
pgdata        PostgreSQL 数据
mcp_socket    mcp-runtime 与 harness 共享的 unix socket
agent_data    App 凭据（0600）
```

密钥处理原则：

```text
App key 只在创建/轮换时返回一次，数据库只存 hash
Admin token 只用于 bootstrap / rotate
ws_token 只放内存，不入库
MCP secret 用 ${secret:name} 占位，由 runtime 解析
日志不打印 Authorization、token、完整凭据
凭据文件权限 0600
```

### 8.3 可选：接入 MCP

已有 MCP Server 时，不需要改 App 代码：

1. 运维用 Admin token 注册 MCP Server：

```http
POST {LEVIER_URL}/admin/mcp
Authorization: Bearer {LEVIER_ADMIN_TOKEN}

{
  "name": "github",
  "transport": "stdio",
  "config": {
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "env": {"GITHUB_PERSONAL_ACCESS_TOKEN": "${secret:github_token}"}
  },
  "allowed_tools": ["search_repositories", "get_file_contents"]
}
```

2. App 注册时通过 `mcp_refs` 引用：

```json
{"name": "my-app", "mcp_refs": ["github"]}
```

3. 启动 `mcp_runtime`，并让 Harness 的 `mcp_runtime.socket` 指向同一个 socket。

工具在模型侧的名字是 `mcp__<server>__<tool>`；`allowed_tools` 由 runtime 强制。
`mcp_runtime` 的部署与 secret 细节见 `docs/MCP.md`。

---

## 9. 运维与排障

### 9.1 健康与指标

```sh
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/readyz
curl -s http://127.0.0.1:8080/metrics
```

重点指标：

```text
levier_runs_total{status,runtime}
levier_events_total{type}
levier_events_dropped_total{reason}
levier_tool_invocations_total{tool,scope,status}
levier_daemon_capacity{daemon,kind,runtime}
levier_ws_connections{app}
levier_workflow_*（claim / lease / reaper）
```

### 9.2 常见现象

| 现象 | 原因 | 处理 |
|---|---|---|
| `/readyz` 503 | 数据库不可用 | 检查 PostgreSQL、DSN、迁移 |
| Harness 一直不接任务 | daemon 未注册 / token 不匹配 / 策略未声明 | 看 middleware 日志与 `levier_daemon_capacity` |
| App 收不到工具调用 | WebSocket 未连上 / 多实例抢连接 | 看 `levier_ws_connections`，确认只有一个逻辑连接 |
| 工具总是 `TOOL_TIMEOUT` | 工具执行过慢或 `tool_ms` 太小 | 优化工具或调大超时（≤ 240s） |
| `TOOL_NOT_FOUND` | 注册列表与调用名不一致 | 重新注册完整工具列表 |
| `APP_UNREACHABLE` | 回包前连接断开 | 保持连接常驻，断线退避重连 |
| Run 一直 `queued` | 没有匹配的 Harness 或容量不足 | 检查 daemon 容量、策略、标签 |
| Run 反复 requeue | Harness 崩溃或事件缓冲满 | 看 Harness 日志、`levier_events_dropped_total` |
| `409` 创建 Run | 同一个 `Idempotency-Key` 但请求内容不同 | 换 key，或复用完全相同的请求 |
| 启动报 `already serving` | 8080 上已有旧进程 | 停掉旧进程或换端口 |

### 9.3 故障矩阵

```sh
make fault-matrix
```

已覆盖：

```text
Harness kill -9 中途 → requeue → 新 attempt 完成
Levier 重启      → Harness 重连 → Run 完成
App 断线             → pending 工具失败 → Run 收敛
取消                 → cancelled + MW 终态事件 + 任务取消
超时                 → timeout + MW 终态事件 + 任务取消
```

### 9.4 轮换 App key

```http
POST {LEVIER_URL}/admin/apps/{APP_ID}/rotate-key
Authorization: Bearer {LEVIER_ADMIN_TOKEN}
```

新 key 只返回一次。轮换后旧 key 立即失效，App 需要用新 key 重新验证和换 ws_token。

---

## 10. 测试与回归

```sh
make lint                 # gofmt + vet + secrets + structure
make structure-check
make deps-check
make build
make agent-core-test
make harness-test
make mcp-runtime-test
make counter-agent-test
make secrets-check

# 真实 PostgreSQL；不设置会跳过 DB 集成用例
set -a; . ./.env.local; set +a
LEVIER_TEST_POSTGRES=1 go test -count=1 ./...
LEVIER_TEST_POSTGRES=1 go test -race -count=1 ./...
```

端到端回归：

```sh
make mvp-demo        # echo，不需要模型 key
make mvp-demo-react  # 真实 DeepSeek + local tools
make fault-matrix    # 故障与恢复
make retrofit-e2e    # 真实 Harness + DeepSeek + local tools
make mcp-e2e         # MCP runtime + 凭据解析
```

Daemon 协议自检（会创建临时 App / Run，使用空闲 daemon id）：

```sh
make daemon-check
# 或
levier daemon check --daemon-id <id> --token <token>
```

App 接入自检：

```sh
python3 tests/app/app_check.py --credentials /path/to/app.json
python3 tests/app/app_check.py --credentials /path/to/app.json \
  --run-tool --tool-name counter_add --arguments '{"delta":2}'
```

退出码 `0` 表示通过，`1` 表示失败（stderr 给出具体检查项）；`--json` 适合 CI 解析。

---

## 11. 目录与文档

```text
cmd/              levier（serve / app / run / migrate / daemon check）/ migrate
internal/         控制面、存储、调度、网关、HTTP/WS
contracts/        HTTP / WS 机器可读契约
proto/            gRPC 契约
gen/              proto 生成的 Go 代码
execution/agent_core/       语言无关的 Agent 循环与 conformance fixtures
execution/harness/          Python Harness
execution/mcp_runtime/      独立 MCP 连接进程
examples/         counter_agent、HTTP/Node 接入示例
tests/            单元/契约/E2E 与检查脚本
scripts/          发布构建与校验脚本
docs/             设计、契约、接入、迁移文档
deploy/           容器镜像与 Compose
```

| 文档 | 内容 |
|---|---|
| `docs/SCOPE.md` | 产品边界与责任矩阵 |
| `docs/MAP.md` | 仓库地图与单元入口 |
| `docs/NAMING.md` | 命名、标识符与改名记录 |
| `docs/MIGRATION-1D.md` | Compose / 数据库 / 指标迁移 |
| `docs/APP_ONBOARDING.md` | App 接入流程与协议细节 |
| `docs/DESIGN.md` | 核心设计（当前实现） |
| `docs/MVP.md` | 单机 MVP 的边界、启动、验收与限制 |
| `docs/CLI_REFERENCE.md` | CLI 命令、参数、环境变量、JSON、退出码 |
| `docs/INSTALL.md` | CLI 安装、升级、卸载、checksum、镜像/离线 |
| `docs/RELEASE.md` | release 构建、checksums 与发布流程 |
| `docs/TODO.md` | 公开前检查清单与 6 配置外部化计划 |
| `docs/ARCHITECTURE.md` | 一页架构与依赖方向 |
| `docs/DAEMON_CONTRACT.md` | Daemon / Harness 协议 |
| `docs/MCP.md` | MCP 接入与运行 |
| `docs/AGENT_CORE_CONTRACT.md` | Agent Core v1 契约 |
| `docs/api/openapi.yaml` | HTTP 契约（OpenAPI 3.1） |
| `contracts/app-ws.schema.json` | App WebSocket v1（JSON Schema） |
| `deploy/README.md` | 容器镜像与 Compose |

参考实现（用来对照改造，不是接入规范）：

```text
examples/counter_agent       Python 标准库实现：bootstrap、常驻服务、提交 Run
examples/node_counter_agent  Node.js 原生 WebSocket 实现，验证语言无关
examples/http_client         只验证 HTTP：认证、创建、查询、取消
```

接入其他语言时，只需满足第 4.2 节的最小能力，不要求照搬目录结构。

---

## 12. 当前边界

```text
[runtime]     只支持 normal
[sandbox]     未实现
[longterm]    checkpoint 协议在 Harness 侧已有，Levier 侧未闭环
[多实例/HA]   不在 MVP 范围
[exactly-once] 不保证；按业务键幂等或接受重复
[压测]        不在 MVP 范围
[MCP]         已验证 stdio 与本地 SSE / streamable_http；TLS、反代、鉴权挑战未测
```

新增范围前，先更新 `docs/DESIGN.md` 与对应契约，再改代码。
