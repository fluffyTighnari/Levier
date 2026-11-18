<!-- Levier — configuration inventory and externalization contract (6A). -->

# 配置盘点与外部化契约（6A）

> 状态：6A 盘点完成；6B 控制面配置已实现；6C/6D/6E 未开始。
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

Harness
  execution/harness/levier.example.yaml + HARNESS_* 环境变量

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

### 3.5 Log

| YAML | 环境变量 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|---|
| `log.level` | `LEVIER_LOG_LEVEL` | enum | `info` | 是 | 否 |
| `log.format` | `LEVIER_LOG_FORMAT` | enum | `json` | 是 | 否 |

### 3.6 兼容与缺口

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

仍需在 6C/6D 处理：

```text
Harness / MCP / Agent 配置命名统一
Compose 默认 secret 清理
外部 Secret Manager（后续）
```

---

## 4. Harness 配置

文件：`execution/harness/levier.example.yaml`
代码：`execution/harness/src/levier_harness/config.py`

| YAML / 参数 | 环境变量 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|---|
| CLI `--config` | `HARNESS_CONFIG` | path | 内置 example | 否 | 否 |
| `middleware.address` | `HARNESS_LEVIER_ADDRESS` / 旧 `HARNESS_MIDDLEWARE_ADDRESS` | host:port | `127.0.0.1:9090` | 是 | 否 |
| `middleware.tls` | `HARNESS_LEVIER_TLS` / 旧 `HARNESS_MIDDLEWARE_TLS` | bool | `false` | 否 | 否 |
| `daemon.id` | `HARNESS_DAEMON_ID` | string | 空 | 是 | 否 |
| `daemon.auth_token` | `HARNESS_DAEMON_AUTH_TOKEN` | string | 空 | 是 | **是** |
| `daemon.runtime_type` | `HARNESS_DAEMON_RUNTIME_TYPE` | enum | `normal` | 是 | 否 |
| `daemon.max_concurrent_runs` | `HARNESS_DAEMON_MAX_CONCURRENT_RUNS` | int | `4` | 是 | 否 |
| `daemon.strategies` | `HARNESS_DAEMON_STRATEGIES` | list | `react,echo` | 是 | 否 |
| `daemon.labels` | `HARNESS_DAEMON_LABELS` | map | 空 | 否 | 否 |
| `mcp_runtime.socket` | `HARNESS_MCP_RUNTIME_SOCKET` | path | 空 | 否 | 否 |
| `logging.level` | `HARNESS_LOGGING_LEVEL` | enum | `info` | 是 | 否 |
| `logging.format` | `HARNESS_LOGGING_FORMAT` | enum | `json` | 是 | 否 |
| 运行时模型 key | `DEEPSEEK_API_KEY` | string | 空 | react 时需要 | **是** |
| 运行时模型地址 | `DEEPSEEK_BASE_URL` | URL | `https://api.deepseek.com` | 否 | 否 |
| 运行时模型名 | `DEEPSEEK_MODEL` | string | `deepseek-flash` | 否 | 否 |

注意：

```text
HARNESS_* 不是 LEVIER_HARNESS_*；命名待 6C 统一
DEEPSEEK_* 由 LLM provider 直接读取，不在 Harness YAML 中
```

---

## 5. MCP Runtime 配置

文件/入口：`execution/mcp_runtime/src/levier_mcp_runtime/__main__.py`

| 参数 | 环境变量 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|---|
| `--socket` | `MCP_RUNTIME_SOCKET` | path | `/tmp/levier-mcp.sock` | 否 | 否 |
| `--stdio` | 无 | bool | `false` | 否 | 否 |
| `--secrets-file` | `MCP_RUNTIME_SECRETS_FILE` | path | 空 | 否 | 文件内容秘密 |
| `--log-level` | `MCP_RUNTIME_LOG_LEVEL` | enum | `info` | 否 | 否 |

Secrets：

```text
LEVIER_SECRET_<NAME>
旧名 MIDDLEWARE_SECRET_<NAME>
secrets-file JSON：secret name -> value
```

`MCP_E2E_TOKEN` 是 Compose 回归环境变量，会注入为 `LEVIER_SECRET_E2E_TOKEN`。

---

## 6. Counter Agent 配置

文件：`examples/counter_agent/config.py`
说明：这是示例 App，不是控制面核心，但需要与统一配置契约对齐。

| 变量/参数 | 类型 | 默认 | 必填 | 秘密 |
|---|---|---|---|---|
| `LEVIER_URL` / 旧 `MW_URL` | URL | `http://127.0.0.1:8080` | 是 | 否 |
| `LEVIER_ADMIN_TOKEN` / 旧 `MW_ADMIN_TOKEN` | string | 空 | bootstrap 时是 | **是** |
| `LEVIER_APP_NAME` / 旧 `MW_APP_NAME` | string | `retrofit-counter` | 否 | 否 |
| `COUNTER_APP_CREDENTIALS` | path | 用户目录 | 运行时时是 | 文件内容秘密 |
| `DEEPSEEK_API_KEY` | string | 空 | baseline 模式 | **是** |
| `--credentials` / `--mw-url` / `--tools-file` | CLI flags | 见脚本 | 视命令 | 否 |

凭据文件包含：

```text
app_id
app_name
api_key
```

---

## 7. Compose / 部署配置

文件：`docker-compose.yml`、`.env.example`、`deploy/README.md`

| 变量 | 消费者 | 默认 | 秘密 | 说明 |
|---|---|---|---|---|
| `POSTGRES_PASSWORD` | postgres / levier | `change-me` | **是** | Compose 数据库密码 |
| `LEVIER_DATABASE_URL` | 本地 CLI / 测试 | 示例 DSN | **是** | 包含密码 |
| `LEVIER_HTTP_ADMIN_TOKEN` | levier | `admin-change-me` | **是** | 来自 `LEVIER_ADMIN_TOKEN` |
| `LEVIER_VERSION` / `LEVIER_COMMIT` / `LEVIER_BUILD_TIME` | levier build args | dev | 否 | 构建元数据 |
| `LEVIER_LOG_LEVEL` / `LEVIER_LOG_FORMAT` | levier | info/json | 否 | 日志 |
| `MCP_E2E_TOKEN` | mcp-runtime | `mcp-e2e-secret` | **是** | 测试 secret |
| `HARNESS_LEVIER_ADDRESS` | harness | `levier:9090` | 否 | 连接地址 |
| `HARNESS_DAEMON_ID` | harness | `daemon-local-1` | 否 | daemon 身份 |
| `HARNESS_DAEMON_AUTH_TOKEN` | harness | `change-me` | **是** | daemon 认证 |
| `HARNESS_MCP_RUNTIME_SOCKET` | harness | socket path | 否 | MCP socket |
| `LEVIER_URL` | agent / smoke | `http://levier:8080` | 否 | HTTP 地址 |
| `LEVIER_ADMIN_TOKEN` | agent | `admin-change-me` | **是** | bootstrap |
| `LEVIER_APP_NAME` | agent | `counter-agent` | 否 | App 名称 |
| `COUNTER_APP_CREDENTIALS` | agent | `/data/credentials.json` | 文件秘密 | 凭据文件 |
| `DEEPSEEK_API_KEY` | harness / baseline | 空 | **是** | 模型 key |
| `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL` | harness | DeepSeek 默认 | 否 | 模型配置 |

开发默认值只适合本地：

```text
change-me
admin-change-me
mcp-e2e-secret
```

生产必须替换，6B/6D 要加启动检查。

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
HARNESS_DAEMON_AUTH_TOKEN
LEVIER_SECRET_* / MCP secrets file
DEEPSEEK_API_KEY
LEVIER_ADMIN_TOKEN
App credentials 文件中的 api_key
POSTGRES_PASSWORD
MCP_E2E_TOKEN（测试）
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

- [ ] `HARNESS_*`、`MCP_RUNTIME_*`、`DEEPSEEK_*`、`COUNTER_*` 命名不统一
- [ ] `MW_*` / `MIDDLEWARE_*` 兼容策略不一致
- [ ] Harness / MCP / Agent 配置校验风格不一致
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
Harness：目标 LEVIER_HARNESS_*，保留 HARNESS_* fallback
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

仍待后续：

```text
config print 是否输出来源（file/env/default）
配置热更新（当前建议不做）
Harness / MCP / Agent 命名统一
```

---

## 12. 6A 验收

- [x] 控制面配置盘点
- [x] Harness 配置盘点
- [x] MCP Runtime 配置盘点
- [x] Agent 配置盘点
- [x] Compose / 部署变量盘点
- [x] CLI 环境变量盘点
- [x] 秘密清单
- [x] 环境变量覆盖缺口
- [x] 命名冲突与兼容问题
- [x] `docs/CONFIG.md` 初版
- [x] 配置契约冻结并进入 6B
