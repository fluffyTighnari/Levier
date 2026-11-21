<!-- Levier — configuration inventory and externalization contract (6A). -->

# 配置盘点与外部化契约（6A）

> 状态：6A 盘点完成；6B 控制面配置已实现；6C 执行侧配置已实现；6D 部署配置已实现；6E 测试与文档已完成，Compose smoke 已通过；10C retention 配置已实现。
> 目标：先明确“现在有哪些配置、从哪里来、哪些是秘密、哪些还缺失”，再进入 6B 改代码。
> 相关：`TODO.md`、`CLI_REFERENCE.md`、`deploy/README.md`。

---

## 1. 目标

配置外部化后应满足：

```text
不改代码，只改配置
配置来源明确
启动时校验
秘密不进入日志
秘密不进入 release
生产环境拒绝示例默认值
```

统一优先级目标：

```text
命令行 flag
  > 环境变量
  > 配置文件
  > 内置默认值
```

---

## 2. 当前配置来源

```text
Levier 控制面
  internal/config + levier.example.yaml + LEVIER_* 环境变量

React Runtime
  execution/react_runtime/react.example.yaml + LEVIER_RUNTIME_* / LEVIER_MIDDLEWARE_* environment variables

MCP Runtime
  命令行参数 + MCP_RUNTIME_* + LEVIER_SECRET_*

Counter Agent
  环境变量 + .env + 凭据文件

Compose / 部署
  .env + docker-compose.yml 环境变量

CLI
  flag + LEVIER_* 环境变量
```

---

## 3. 控制面配置

文件：`levier.example.yaml`
代码：`internal/config/config.go`

### 3.0 版本与环境

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 | 现状 |
|---|---|---|---|---|---|---|
| `config_version` | 无 | int | `1` | 是 | 否 | 仅文件 |
| `environment` | `LEVIER_ENV` | enum | `development` | 是 | 否 | 支持 env |

### 3.1 HTTP

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 | 现状 |
|---|---|---|---|---|---|---|
| `http.addr` | `LEVIER_HTTP_ADDR` | host:port | `:8080` | 是 | 否 | 支持 env |
| `http.read_timeout` | `LEVIER_HTTP_READ_TIMEOUT` | duration | `15s` | 是 | 否 | 支持 env |
| `http.write_timeout` | `LEVIER_HTTP_WRITE_TIMEOUT` | duration | `30s` | 是 | 否 | 支持 env |
| `http.shutdown_timeout` | `LEVIER_HTTP_SHUTDOWN_TIMEOUT` | duration | `10s` | 是 | 否 | 支持 env |
| `http.bearer_token` | `LEVIER_HTTP_BEARER_TOKEN` | string | 空 | 否 | **是** | 兼容旧 Bearer |
| `http.bearer_token_file` | `LEVIER_HTTP_BEARER_TOKEN_FILE` | path | 空 | 否 | **是** | 文件内容秘密 |
| `http.admin_token` | `LEVIER_HTTP_ADMIN_TOKEN` | string | 空 | 生产必填 | **是** | 支持 env |
| `http.admin_token_file` | `LEVIER_HTTP_ADMIN_TOKEN_FILE` | path | 空 | 生产推荐 | **是** | 文件内容秘密 |
| `http.allow_legacy_bearer` | `LEVIER_HTTP_ALLOW_LEGACY_BEARER` | bool | `false` | 否 | 否 | 兼容开关 |
| `http.ws_token_ttl` | `LEVIER_HTTP_WS_TOKEN_TTL` | duration | `10m` | 是 | 否 | 支持 env |

### 3.2 gRPC

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 | 现状 |
|---|---|---|---|---|---|---|
| `grpc.addr` | `LEVIER_GRPC_ADDR` | host:port | `:9090` | 是 | 否 | 支持 env |
| `grpc.max_recv_msg_bytes` | `LEVIER_GRPC_MAX_RECV_MSG_BYTES` | int | `16777216` | 是 | 否 | 支持 env |
| `grpc.max_send_msg_bytes` | `LEVIER_GRPC_MAX_SEND_MSG_BYTES` | int | `16777216` | 是 | 否 | 支持 env |
| `grpc.shutdown_timeout` | `LEVIER_GRPC_SHUTDOWN_TIMEOUT` | duration | `10s` | 是 | 否 | 支持 env |
| `grpc.daemon_tokens` | 无 | map | 空 | 运行必需 | **是** | 兼容旧文件 |
| `grpc.daemon_tokens_file` | `LEVIER_GRPC_DAEMON_TOKENS_FILE` | path | 空 | 生产推荐 | **是** | JSON secret file |

### 3.3 Database

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 | 现状 |
|---|---|---|---|---|---|---|
| `database.url` | `LEVIER_DATABASE_URL` | DSN | 空 | 是 | **是** | 支持 env |
| `database.url_file` | `LEVIER_DATABASE_URL_FILE` | path | 空 | 生产推荐 | **是** | 文件内容秘密 |
| `database.max_conns` | `LEVIER_DATABASE_MAX_CONNS` | int | `20` | 是 | 否 | 支持 env |
| `database.min_conns` | `LEVIER_DATABASE_MIN_CONNS` | int | `2` | 是 | 否 | 支持 env |
| `database.connect_timeout` | `LEVIER_DATABASE_CONNECT_TIMEOUT` | duration | `5s` | 是 | 否 | 支持 env |
| `database.health_timeout` | `LEVIER_DATABASE_HEALTH_TIMEOUT` | duration | `2s` | 是 | 否 | 支持 env |

### 3.4 Workflow

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|---|
| `workflow.workers` | `LEVIER_WORKFLOW_WORKERS` | int | `4` | 是 | 否 |
| `workflow.lease_ttl` | `LEVIER_WORKFLOW_LEASE_TTL` | duration | `30s` | 是 | 否 |
| `workflow.poll_interval` | `LEVIER_WORKFLOW_POLL_INTERVAL` | duration | `200ms` | 是 | 否 |
| `workflow.max_attempts` | `LEVIER_WORKFLOW_MAX_ATTEMPTS` | int | `5` | 是 | 否 |
| `workflow.reap_interval` | `LEVIER_WORKFLOW_REAP_INTERVAL` | duration | `10s` | 是 | 否 |
| `workflow.reap_limit` | `LEVIER_WORKFLOW_REAP_LIMIT` | int | `100` | 是 | 否 |
| `workflow.recovery_delay` | `LEVIER_WORKFLOW_RECOVERY_DELAY` | duration | `10s` | 是 | 否 |
| `workflow.event_buffer` | `LEVIER_WORKFLOW_EVENT_BUFFER` | int | `256` | 是 | 否 |

### 3.5 Retention

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 | 现状 |
|---|---|---|---|---|---|---|
| `retention.enabled` | `LEVIER_RETENTION_ENABLED` | bool | `false` | 是 | 否 | 默认关闭 |
| `retention.run_days` | `LEVIER_RETENTION_RUN_DAYS` | int | `30` | 是 | 否 | 终态 Run 保留天数 |
| `retention.session_days` | `LEVIER_RETENTION_SESSION_DAYS` | int | `7` | 是 | 否 | 过期 Session 保留天数 |
| `retention.batch_size` | `LEVIER_RETENTION_BATCH_SIZE` | int | `1000` | 是 | 否 | 单批删除上限，最大 10000 |

第一版以**终态 Run** 为清理单位：

```text
只删除 completed / failed / cancelled / timeout 且超过 run_days 的 Run
删除 Run 时由外键级联删除 run_events / run_tasks / tool_invocations
绝不删除 queued / assigned / running 的 Run
Session 只有 expires_at 过期且没有 active Run 时才标记 / 删除（active / paused 都会过期）
retention.enabled=false 时 prune 不删除任何数据
```

`maintenance prune` 默认 dry-run；只有显式 `--yes` 才执行删除。更细粒度的
event / task / tool invocation 独立保留期留到后续版本。

### 3.6 Log

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|---|
| `log.level` | `LEVIER_LOG_LEVEL` | enum | `info` | 是 | 否 |
| `log.format` | `LEVIER_LOG_FORMAT` | enum | `json` | 是 | 否 |

### 3.7 兼容与缺口

当前 `MIDDLEWARE_*` 作为旧前缀，在一个版本内作为 `LEVIER_*` 的 fallback。

6B 已补齐：

```text
http.read_timeout / write_timeout env
grpc.max_recv_msg_bytes / max_send_msg_bytes env
database 连接池和超时 env
grpc.daemon_tokens_file
http.admin_token_file / bearer_token_file
database.url_file
config_version / environment
config check / config print
```

6C/6D 已补齐：

```text
React Runtime / MCP / Agent 配置命名统一
Compose 默认 secret 清理
生产式 Compose secret 文件挂载
```

10C 已补齐：

```text
retention.enabled / run_days / session_days / batch_size
LEVIER_RETENTION_* 环境变量
retention 默认关闭
```

仍待后续：

```text
外部 Secret Manager
```

---

## 4. React Runtime configuration

The React Runtime uses the standard Runtime SDK configuration shape. It does
not have a private `daemon:` parser.

```yaml
middleware:
  address: "127.0.0.1:9090"
  auth_token: "change-me"

runtime:
  name: "react1"
  instance_id: "daemon-local-1"
  type: "normal"
  max_concurrent_runs: 4

logging:
  level: "info"
```

The SDK reads `middleware`, `runtime`, and `logging`. MCP is resolved per run by
`ctx.toolbox()` from `LEVIER_MCP_RUNTIME_SOCKET`, `LEVIER_MCP_ADDR`, or the user
profile; there is no React-specific `mcp_runtime` YAML section.

Canonical environment variables:

```text
LEVIER_RUNTIME_CONFIG
LEVIER_MIDDLEWARE_ADDRESS
LEVIER_MIDDLEWARE_TLS
LEVIER_RUNTIME_TOKEN
LEVIER_RUNTIME_TOKEN_FILE
LEVIER_RUNTIME_NAME
LEVIER_RUNTIME_INSTANCE_ID
LEVIER_RUNTIME_TYPE
LEVIER_RUNTIME_MAX_CONCURRENT_RUNS
LEVIER_RUNTIME_LABELS
LEVIER_RUNTIME_LOG_LEVEL
```

Use the SDK environment names above. `DEEPSEEK_*` remains a legacy local
Runtime fallback; production should use the control-plane provider catalog and
secret refs instead.

---

## 4.1 Model provider catalog

The control plane owns provider metadata and secret references. It never
stores or returns a plaintext model key.

```yaml
# levier model connect ...
name: deepseek
adapter: openai_compatible
base_url: https://api.deepseek.com
default_model: deepseek-flash
models: [deepseek-flash, deepseek-reasoner]
secret_ref: deepseek_api_key
enabled: true
default: true
```

Secret values come from:

```text
LEVIER_SECRET_<UPPER_REF>
LEVIER_SECRETS_FILE
LEVIER_SECRETS_WRITE_FILE      # 0600 managed file, writable via Admin API
```

Example:

```text
secret_ref=deepseek_api_key
LEVIER_SECRET_DEEPSEEK_API_KEY=sk-...
```

or a 0600 JSON file:

```json
{"deepseek_api_key": "sk-...", "openai_api_key": "sk-..."}
```

Provider endpoints:

```text
GET    /api/v1/model-providers        App-readable catalog, no secrets
GET    /admin/model-providers         Admin catalog with secret_ref status
POST   /admin/model-providers         Upsert provider metadata
DELETE /admin/model-providers/{name}
POST   /admin/model-providers/{name}/check
PUT    /admin/model-providers/{name}/secret   Store the key in the managed file
```

`levier model connect` uses the last endpoint, so App authors only provide URL,
key, and model. Production may keep using environment variables or a read-only
secret file.

Provider keys are resolved at dispatch time and placed only in the per-run
`TaskAssign.ModelProfile`. They are not included in provider list responses,
events, logs, checkpoints, or `/meta`.

## 5. MCP Runtime 配置

文件/入口：`execution/mcp_runtime/src/levier_mcp_runtime/__main__.py`

| 参数 | 环境变量 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|---|
| `--socket` | `LEVIER_MCP_SOCKET` / 旧 `MCP_RUNTIME_SOCKET` | path | `/tmp/levier-mcp.sock` | 否 | 否 |
| `--addr` | `LEVIER_MCP_ADDR` | host:port | 空 | 否 | 否 |
| `--stdio` | 无 | bool | `false` | 否 | 否 |
| `--secrets-file` | `LEVIER_MCP_SECRETS_FILE` / 旧 `MCP_RUNTIME_SECRETS_FILE` | path | 空 | 否 | 文件内容秘密 |
| `--log-level` | `LEVIER_MCP_LOG_LEVEL` / 旧 `MCP_RUNTIME_LOG_LEVEL` | enum | `info` | 否 | 否 |

Secrets：

```text
LEVIER_SECRET_<NAME>
旧名 MIDDLEWARE_SECRET_<NAME>
secrets-file JSON：secret name -> value
```

`secrets-file` 在 Unix 上要求 0600 权限。
`MCP_E2E_TOKEN` 只用于本地 e2e 脚本；Compose 不再内置测试 secret。

---

## 6. Counter Agent 配置

文件：`sdk/python/src/levier_app/credentials.py`、`examples/counter_agent/app.py`
说明：示例 App 只读取 canonical 凭据和 server 地址，不再持有 Admin token。

| 变量/参数 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|
| `LEVIER_CREDENTIALS` / `LEVIER_APP_CREDENTIALS` | path | `~/.config/levier/app.json` | 运行时是 | 文件内容秘密 |
| `LEVIER_SERVER` / `LEVIER_APP_URL` / 旧 `LEVIER_URL` | URL | 凭据中的 `server` 或 `http://127.0.0.1:8080` | 否 | 否 |
| `LEVIER_APP_NAME` | string | `counter-agent` | 仅 `app init` 时 | 否 |
| `LEVIER_ADMIN_TOKEN` | string | 空 | 仅 `app init` 时 | **是** |

凭据通过 `levier app init` 创建并写入 0600 文件；运行中的 App 只读取
`app_id` / `app_name` / `api_key` / `server`，不需要 Admin token。

凭据文件格式版本为 1，包含：

```text
credential_version
app_id
app_name
api_key
server
created_at
updated_at
```

旧文件只包含 `app_id` / `app_name` / `api_key` 时仍可读取；下一次写入会升级格式。文件必须为 0600。

---

## 6.1 用户级 CLI profile

`levier setup` 写入的 profile 主要描述“去哪里找”。它默认不保存 Admin token
或 App API key；显式使用 `--admin-token` / `--runtime-token` 时才会把对应值写入
0600 profile：

```text
Windows      %APPDATA%\levier\config.yaml
Linux        ~/.config/levier/config.yaml
macOS        ~/Library/Application Support/levier/config.yaml
```

字段：

| 字段 | 对应参数 | 用途 |
|---|---|---|
| `server` | `--server` | 默认 HTTP 地址 |
| `admin_token` | `--admin-token` | 可选，显式保存的 Admin token |
| `credentials` | `--credentials` | 默认 App 凭据文件 |
| `app_name` | `--name` | 默认 App 名 |
| `mcp_refs` | `--mcp-ref` | 默认 MCP 引用 |
| `runtime` | `--runtime` | 默认目标 runtime |
| `runtime_token` | `--runtime-token` | 本地 runtime 启动凭据 |
| `middleware_address` | `--grpc` | 默认 gRPC 地址 |
| `runtime_config` | `--runtime-config` | 默认 Runtime YAML |
| `mcp_socket` | `--mcp-socket` | 默认 MCP socket/TCP 地址 |
| `control_config` | `--control-config` | 控制面 YAML（运维命令） |
| `stack_file` | `--stack-file` | 默认 `docker-compose.yml` |

`levier init` 还会在当前项目写入 `.levier.json`，保存 App 名、凭据路径、
server、runtime 和 MCP 引用。它不含 key，并已在仓库 `.gitignore` 中忽略。

`LEVIER_HOME` 可以把整份 profile 和安装数据放到临时目录，适合测试。

---

## 7. Compose / 部署配置

文件：`docker-compose.yml`、`docker-compose.prod.yml`、`.env.example`、`deploy/README.md`

开发基础配置：

| 变量 | 消费者 | 默认 | 秘密 | 说明 |
|---|---|---|---|---|
| `POSTGRES_PASSWORD` | postgres / levier | `change-me` | **是** | 仅本地开发 |
| `LEVIER_DATABASE_URL` | 本地 CLI / 测试 | 示例 DSN | **是** | 仅本地开发 |
| `LEVIER_HTTP_ADMIN_TOKEN` | levier | `admin-change-me` | **是** | 来自 `LEVIER_ADMIN_TOKEN`，仅开发 |
| `LEVIER_VERSION` / `LEVIER_COMMIT` / `LEVIER_BUILD_TIME` | levier build args | dev | 否 | 构建元数据 |
| `LEVIER_LOG_LEVEL` / `LEVIER_LOG_FORMAT` | levier | info/json | 否 | 日志 |
| `LEVIER_MIDDLEWARE_ADDRESS` | react-runtime | `levier:9090` | 否 | 连接地址 |
| `LEVIER_RUNTIME_INSTANCE_ID` | react-runtime | `daemon-local-1` | 否 | daemon 身份 |
| `LEVIER_RUNTIME_TOKEN` | react-runtime | `change-me` | **是** | 仅开发 |
| `LEVIER_MCP_RUNTIME_SOCKET` | react-runtime | socket path | 否 | MCP socket |
| `LEVIER_APP_URL` | agent / smoke | `http://levier:8080` | 否 | HTTP 地址 |
| `LEVIER_APP_CREDENTIALS` | agent | `/data/credentials.json` | 文件秘密 | 由 host `levier app init` 写入 `data/agent` |
| `DEEPSEEK_API_KEY` | react-runtime / baseline | 空 | **是** | 模型 key |
| `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL` | react-runtime | DeepSeek 默认 | 否 | 模型配置 |

生产式 overlay（`docker-compose.prod.yml`）：

| secret 文件 | 消费者 |
|---|---|
| `deploy/secrets/levier_postgres_password` | PostgreSQL |
| `deploy/secrets/levier_admin_token` | Levier HTTP admin（测试 App 由 host 上的 `levier app init` 创建） |
| `deploy/secrets/levier_daemon_tokens.json` | Levier gRPC daemon tokens |
| `deploy/secrets/levier_database_url` | Levier 数据库 DSN |
| `deploy/secrets/levier_react_runtime_daemon_token` | React Runtime daemon token |
| `deploy/secrets/levier_mcp_secrets.json` | MCP Runtime |

`levier_admin_token` 同时供控制面与 Agent 使用；React Runtime 的 daemon token 必须与
`levier_daemon_tokens.json` 中对应 daemon 的值一致；`levier_database_url` 中的密码
必须与 `levier_postgres_password` 一致。

开发默认值只适合本地：

```text
change-me
admin-change-me
```

生产式 overlay 使用 secret 文件，不把真实值写入 Compose 或 YAML。

---

## 8. CLI 环境变量

这些属于 CLI 使用面，详细行为见 `CLI_REFERENCE.md`：

| 变量 | 用途 |
|---|---|
| `LEVIER_CONFIG` | serve / doctor / migrate 配置路径 |
| `MIDDLEWARE_CONFIG` | serve 旧配置路径 fallback |
| `LEVIER_SERVER` | HTTP 地址 |
| `LEVIER_CREDENTIALS` | App 凭据文件 |
| `LEVIER_ADMIN_TOKEN` | Admin token |
| `LEVIER_APP_NAME` | App 名称 |
| `LEVIER_GRPC` | daemon check gRPC 地址 |
| `LEVIER_DAEMON_ID` | daemon check id |
| `LEVIER_DAEMON_TOKEN` | daemon check token |

测试专用：

```text
LEVIER_TEST_DATABASE_URL
LEVIER_TEST_POSTGRES
```

---

## 9. 秘密清单

必须按秘密处理：

```text
http.admin_token
http.bearer_token
grpc.daemon_tokens.*
database.url（含密码）
LEVIER_RUNTIME_TOKEN
LEVIER_SECRET_* / MCP secrets file
DEEPSEEK_API_KEY
LEVIER_ADMIN_TOKEN（仅 app init / rotate / 测试初始化）
App credentials 文件中的 api_key
POSTGRES_PASSWORD
MCP_E2E_TOKEN（仅本地测试）
LEVIER_TEST_DATABASE_URL（测试）
```

规则：

```text
不打印
不写日志
不写入 release
不写入镜像
不放入 --json
检查脚本只允许示例占位符
```

---

## 10. 盘点结论

### P0 必须处理

- [x] 控制面 env 覆盖不完整
- [x] `grpc.daemon_tokens` 没有安全注入入口
- [x] 示例默认 token 可能被直接用于生产
- [x] 秘密和普通配置混在 YAML
- [x] 缺少统一的配置检查命令

### P1 应处理

- [x] `HARNESS_*`、`MCP_RUNTIME_*`、`DEEPSEEK_*`、`COUNTER_*` 命名不统一
- [x] `MW_*` / `MIDDLEWARE_*` 兼容策略不一致
- [ ] React Runtime / MCP / Agent 配置校验风格不一致
- [x] 缺少配置项总表

### P2 后续

- [ ] 配置热更新
- [ ] 外部 Secret Manager
- [ ] 配置中心
- [ ] 多租户配置

---

## 11. 已采用的契约（6B）

```text
Levier 产品命名空间：LEVIER_
控制面：LEVIER_HTTP_* / LEVIER_GRPC_* / LEVIER_DATABASE_* / LEVIER_WORKFLOW_* / LEVIER_LOG_*
React Runtime：目标 LEVIER_RUNTIME_* / LEVIER_MIDDLEWARE_*，保留 HARNESS_* fallback
MCP：目标 LEVIER_MCP_*，保留 MCP_RUNTIME_* fallback
Agent：目标 LEVIER_APP_*，保留 LEVIER_* / MW_* fallback
```

6B 已采用：

```text
config_version: 1
environment: development | production
秘密通过环境变量或 0600 文件
admin_token_file
bearer_token_file
daemon_tokens_file
database.url_file
config check
config print（脱敏）
```

6C 已采用：

```text
React Runtime 支持 LEVIER_RUNTIME_* / LEVIER_MIDDLEWARE_*，保留 HARNESS_* / HARNESS_MIDDLEWARE_* fallback
React Runtime 支持 daemon.auth_token_file
MCP 支持 LEVIER_MCP_*，保留 MCP_RUNTIME_* fallback
MCP secrets-file 在 Unix 上检查 0600
Agent 只读取 LEVIER_APP_CREDENTIALS / LEVIER_CREDENTIALS
Agent 不接收 Admin token；身份由 host 上的 levier app init 创建
Agent 的 server 地址优先使用凭据文件中的 server
```

6D 已采用：

```text
docker-compose.prod.yml 生产式 overlay
deploy/config/levier.prod.yaml 只含非秘密配置
deploy/secrets/* 宿主机 0600 secret 文件
admin token 只用于控制面和测试初始化，不进入 Agent 运行时
数据库 DSN / daemon tokens / MCP secrets 均通过 *_file 注入
```

6E 已采用：

```text
internal/config：解析、未知字段、非法类型、环境变量优先级、secret 文件测试
cmd/levier：config check、config check --json、config print --json、脱敏测试
docs：CONFIG / CLI_REFERENCE / INSTALL / 迁移说明
Compose smoke：本地开发栈已通过
```

### 11.1 从旧配置迁移

```text
1. 在 YAML 顶层加入 config_version: 1 和 environment
2. 控制面环境变量改用 LEVIER_*；旧 MIDDLEWARE_* 仍兼容一个版本
3. React Runtime / MCP / Agent 改用 LEVIER_RUNTIME_* / LEVIER_MIDDLEWARE_* / LEVIER_MCP_* / LEVIER_APP_*
4. 生产环境把 token / DSN 移到 0600 secret 文件，YAML 只保留 *_file 路径
5. 启动前运行 config check；生产再加 --production
```

旧配置仍可直接用于本地开发；生产配置不要继续使用 `change-me` 或
`admin-change-me`，否则 `config check --production` 会拒绝。

最小生产配置结构：

```yaml
config_version: 1
environment: production
http:
  admin_token_file: /run/secrets/levier_admin_token
grpc:
  daemon_tokens_file: /run/secrets/levier_daemon_tokens.json
database:
  url_file: /run/secrets/levier_database_url
```

仍待后续：

```text
config print 是否输出来源（file/env/default）
配置热更新（当前建议不做）
React Runtime / MCP / Agent 内部校验代码风格进一步统一
```

---

## 12. 6A 验收

- [x] 控制面配置盘点
- [x] React Runtime 配置盘点
- [x] MCP Runtime 配置盘点
- [x] Agent 配置盘点
- [x] Compose / 部署变量盘点
- [x] CLI 环境变量盘点
- [x] 秘密清单
- [x] 环境变量覆盖缺口
- [x] 命名冲突与兼容问题
- [x] `docs/CONFIG.md` 初版
- [x] 配置契约冻结并进入 6B
