<!-- Levier — CLI reference. -->

# Levier CLI 参考

> 适用版本：5A CLI 完成后的单机 / Compose 版本。
>
> 本文是**用户参考**；设计动机与执行记录见 [`CLI_ACTIONS.md`](CLI_ACTIONS.md)，分发路线见 [`CLI_DISTRIBUTION_PLAN.md`](CLI_DISTRIBUTION_PLAN.md)。
>
> 安装见 [`INSTALL.md`](INSTALL.md)。
>
> 最后更新：2026-11-18。

---

## 1. 入口与安装

本地构建：

```sh
make build
bin/levier version
```

Compose 内直接调用同一个二进制：

```sh
docker compose exec levier /app/levier status
docker compose exec levier /app/levier doctor
```

尚未发布 release 二进制；安装与分发见 5B。

---

## 2. 全局约定

### 2.1 参数

所有命令同时接受单横线和双横线：

```text
-config path
--config path
```

文档统一使用 `--`。

帮助：

```sh
levier help
levier app --help
levier app init --help
```

帮助输出到 stdout，退出码为 `0`；参数错误输出到 stderr，退出码为 `2`。

### 2.2 默认地址

```text
HTTP   http://127.0.0.1:8080
gRPC   127.0.0.1:9090
配置   levier.example.yaml
```

在 Compose 容器内访问 `levier` 服务时使用服务名：

```text
--server http://levier:8080
--grpc levier:9090
```

### 2.3 输出与密钥

```text
人类可读结果 -> stdout
错误与用法     -> stderr
--json         -> stdout 输出稳定 JSON
```

规则：

```text
不打印 api_key
不把 Admin token 写入凭据文件
--json 中不包含密钥
```

### 2.4 凭据文件

默认结构：

```json
{
  "app_id": "app_...",
  "app_name": "counter-agent",
  "api_key": "mw_..."
}
```

要求：

```text
文件权限 0600
目录权限 0700
临时文件 + rename 原子写入
覆盖时生成 .bak
```

默认路径：

```text
LEVIER_CREDENTIALS
或 os.UserConfigDir()/levier/<安全化后的 app-name>.json
```

---

## 3. 环境变量

| 变量 | 使用命令 | 说明 |
|---|---|---|
| `LEVIER_CONFIG` | `serve`、`doctor`、`migrate`、`maintenance`、`config` | YAML 配置路径 |
| `LEVIER_ENV` | `serve`、`config` | `development` 或 `production` |
| `MIDDLEWARE_CONFIG` | `serve` | 旧名兼容；`LEVIER_CONFIG` 优先 |
| `LEVIER_SERVER` | `doctor`、`status`、`app`、`run`、`ops`、`daemon check` | HTTP 地址 |
| `LEVIER_CREDENTIALS` | `app init/show/check/rotate-key`、`run`、`doctor` | 凭据文件路径 |
| `LEVIER_ADMIN_TOKEN` | `app init`、`app rotate-key`、`app list`、`ops`、`daemon check`、`doctor` | Admin token |
| `LEVIER_APP_NAME` | `app init` | App 名称 |
| `LEVIER_GRPC` | `daemon check` | gRPC 地址 |
| `LEVIER_DAEMON_ID` | `daemon check` | 预分配 daemon id |
| `LEVIER_DAEMON_TOKEN` | `daemon check` | daemon token |
| `LEVIER_RETENTION_ENABLED` | `maintenance` | 启用 retention |
| `LEVIER_RETENTION_RUN_DAYS` | `maintenance` | 终态 Run 保留天数 |
| `LEVIER_RETENTION_SESSION_DAYS` | `maintenance` | 过期 Session 保留天数 |
| `LEVIER_RETENTION_BATCH_SIZE` | `maintenance` | 单批删除上限 |

优先级：

```text
命令行 flag > 环境变量 > 命令默认值
```

`doctor` 读取 Admin token 的顺序：

```text
LEVIER_ADMIN_TOKEN
或配置文件中的 http.admin_token
```

---

## 4. 快速路径

```sh
# 1. 准备 Admin token
export LEVIER_ADMIN_TOKEN='...'

# 2. 创建 App 和凭据文件
levier app init \
  --name demo \
  --credentials-out ./demo.json

# 3. 确认 App 凭据和接入链路
levier app check --credentials ./demo.json

# 可选：查看服务端 App 列表
levier app list

# 4. 提交并等待一个 Run
levier run submit \
  --credentials ./demo.json \
  --runtime echo1 \
  --prompt "hello" \
  --watch

# 5. 查看控制面状态
levier status
levier doctor --credentials ./demo.json
```

`echo` 不依赖模型；`react` 需要 Harness、模型配置和工具。

---

## 5. 服务端命令

### 5.1 `levier serve`

```sh
levier serve [--config path]
```

兼容旧启动方式：

```sh
levier [--config path]
```

配置默认值：

```text
LEVIER_CONFIG
或 MIDDLEWARE_CONFIG
或 levier.example.yaml
```

启动顺序：

```text
加载配置
连接数据库
检查 pending migration
pending > 0 -> 报错并退出 1
不自动执行 DDL
启动 HTTP / gRPC
```

Compose entrypoint 明确执行：

```sh
/app/levier migrate up -config "$CONFIG"
exec /app/levier serve -config "$CONFIG"
```

退出码：

```text
0  正常退出 / 收到 SIGINT、SIGTERM
1  配置、数据库、迁移检查或监听失败
2  参数错误
```

### 5.2 `levier version`

```sh
levier version [--json]
```

人类可读输出：

```text
levier dev
commit: <sha>
built: <time>
go: go1.x
platform: linux/amd64
```

JSON 字段：

```json
{
  "version": "dev",
  "commit": "unknown",
  "built": "unknown",
  "go": "go1.26.11",
  "platform": "linux/amd64"
}
```

退出码：`0` 成功；`2` 参数错误。

### 5.3 `levier doctor`

```sh
levier doctor [--config path] [--server url] [--credentials path] [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | YAML 配置 |
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址 |
| `--credentials` | `LEVIER_CREDENTIALS` | 可选，检查 App key |
| `--timeout` | `5s` | 单次请求 / 查询超时 |
| `--json` | `false` | JSON 输出 |

检查项：

```text
config parse
database connect
migrations（可读 schema_migrations，且 pending = 0）
HTTP /healthz
HTTP /readyz
HTTP /meta
admin auth
app credentials（提供 --credentials 时）
```

状态值：

```text
ok
fail
skip
```

JSON 结构：

```json
{
  "ok": true,
  "config": "levier.example.yaml",
  "server": "http://127.0.0.1:8080",
  "checks": [
    {
      "name": "migrations",
      "status": "ok",
      "message": "version 11 (11 applied)"
    }
  ]
}
```

退出码：

```text
0  所有检查都没有 fail
1  至少一个检查 fail
2  参数错误
```

`skip` 不等于失败；缺少可选凭据时会显示 `skip`。

### 5.4 `levier status`

```sh
levier status [--server url] [--json] [--timeout 5s]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址 |
| `--timeout` | `5s` | 单次请求超时 |
| `--json` | `false` | 输出 `/status` 快照 |

首选接口：

```text
GET /status
```

`/status` 返回聚合状态：

- 服务版本、运行时间、快照时间
- 存活 / 就绪 / 数据库状态
- daemon 数量、容量、使用量
- App 注册数和 WebSocket 连接数
- Run 总数、活跃数和各状态计数
- workflow pending / leased / done / dead 统计

普通输出示例：

```text
server      http://127.0.0.1:8080
status      ok
version     0.1.0-rc.5 (2ce014d)
uptime      1h2m
health      ok
ready       ok
database    up
daemons     connected=1 healthy=1 capacity=4 used=1 available=3
apps        registered=1 websocket=1
runs        total=1 active=0 cancelled=0 completed=1 failed=0 queued=0 running=0 timeout=0
workflow    total=1 pending=0 leased=0 dead=0 oldest_pending=0s
```

如果服务端是旧版本、没有 `/status`，`levier status` 会回退到旧方式：

```text
/healthz
/readyz
/metrics
```

回退时输出保持旧的 `health / ready / metrics / daemons / apps / runs` 结构。

退出码：

```text
0  状态快照就绪（readiness = ok）
1  服务不可达、协议版本不支持、readiness degraded，或旧 metrics 检查失败
2  参数错误
```

公开契约：`public/service-status/v1/schema.json`。

### 5.5 `levier config check`

```sh
levier config check [--config path] [--production] [--json]
```

行为：

```text
读取 YAML
应用 LEVIER_* 环境变量
读取 admin_token_file / daemon_tokens_file / url_file
执行结构校验
--production 额外拒绝示例默认值和过宽 secret 文件权限
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | YAML 配置 |
| `--production` | `false` | 生产秘密检查 |
| `--json` | `false` | JSON 输出 |

JSON：

```json
{
  "ok": true,
  "config": "levier.example.yaml",
  "version": 1,
  "environment": "development"
}
```

失败时包含 `error`，退出 `1`。参数错误退出 `2`。

### 5.6 `levier config print`

```sh
levier config print [--config path] [--json]
```

行为：

```text
输出生效后的配置
默认 YAML
--json 输出稳定 JSON
秘密字段显示 ***
数据库 URL 只保留用户名和 host，密码变为 xxxxx
不会输出 secret 文件内容
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | YAML 配置 |
| `--json` | `false` | JSON 输出 |

退出码：

```text
0  成功
1  配置无法加载或校验
2  参数错误
```

---

### 5.7 `levier describe`

```sh
levier describe [--server url] [--json] [--timeout 5s]
```

行为：

```text
GET /meta
不读取配置文件
不需要 Admin token 或 App key
输出版本、协议、能力、限制、认证方式和主要接口
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | Levier HTTP 地址 |
| `--json` | `false` | 输出原始 `/meta` JSON |
| `--timeout` | `5s` | 请求超时 |

普通输出示例：

```text
service      levier
version      0.1.0-rc.5
commit       2ce014d
api          v1
ws protocol  1
runtime      normal
limits       body=1048576 run=1800000ms tool=60000ms ws_token=600s
endpoints    health, ready, metrics, runs, apps, websocket
```

退出码：

```text
0  成功
1  服务不可达、HTTP 非 200、JSON 非法或协议版本不支持
2  参数错误
```

> `describe` 只访问公开的 `/meta`；`doctor` 也会检查同一接口。
> 公共 schema 位于 `public/service-meta/v1/schema.json`，示例见
> `public/service-meta/v1/example.json`。

---

### 5.8 `levier selfcheck`

```sh
levier selfcheck [options]
```

逐跳检查：

```text
config
http /healthz
database
migrations
http /readyz
service /meta
service /status
admin auth
daemon registered
app credentials
```

默认是只读检查，不创建 App 或 Run。缺少可选凭据时显示 `skip`，不是 `fail`。

主动检查（会创建临时 App / Run）：

```sh
levier selfcheck --deep \
  --admin-token "$LEVIER_ADMIN_TOKEN" \
  --daemon-token "$LEVIER_DAEMON_TOKEN"
```

`--deep` 复用 `daemon check` 的协议链路检查，并在提供凭据时检查真实 App WebSocket、在提供
socket 时检查 MCP runtime health；真实 local tool 可继续使用 `tests/app/app_check.py`。

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | 本地配置 |
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址 |
| `--grpc` | `LEVIER_GRPC` 或 `127.0.0.1:9090` | gRPC 地址 |
| `--admin-token` | `LEVIER_ADMIN_TOKEN` | Admin 检查 |
| `--credentials` | `LEVIER_CREDENTIALS` | App 凭据检查 |
| `--daemon-id` | `LEVIER_DAEMON_ID` | deep 检查 daemon id |
| `--daemon-token` | `LEVIER_DAEMON_TOKEN` | deep 检查 token |
| `--mcp-socket` | `LEVIER_MCP_SOCKET` | deep 检查 MCP runtime socket |
| `--runtime` | `echo1` | deep 检查策略 |
| `--timeout` | `5s` | 单 hop 超时 |
| `--deep` | `false` | 启用主动 daemon 协议检查 |
| `--json` | `false` | 输出结构化 JSON |

退出码：

```text
0  没有 fail hop
1  至少一个 hop fail
2  参数错误
```

公开输出 schema：

```text
public/service-selfcheck/v1/schema.json
```

---

### 5.9 `levier maintenance`

```sh
levier maintenance status [--config path] [--json]
levier maintenance prune [--config path] [--dry-run] [--yes] [--json]
```

维护命令直接连接 PostgreSQL，不经过 HTTP，也没有公共 maintenance API。

`status` 只读，显示：

```text
retention.enabled / run_days / session_days / batch_size
runs total / active / terminal / eligible
oldest terminal age
run_events total
run_tasks counts by status
tool_invocations counts by status
sessions total / active / paused / expired / eligible
run / session cutoff time
```

`prune` 默认 dry-run，只显示候选数量，不删除数据。只有同时满足以下条件才会执行：

```text
retention.enabled = true
显式传入 --yes
没有同时传入 --dry-run
```

执行时只删除超过 `run_days` 的终态 Run；该 Run 的 events、tasks 和 tool
invocations 通过外键级联删除。过期 Session 会先标记为 `expired`，再按
`session_days` 清理；仍被 active Run 引用的 Session 不会被删除。

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | YAML 配置 |
| `--dry-run` | `true` | 只显示候选；与 `--yes` 同时使用会报错 |
| `--yes` | `false` | 确认执行删除 |
| `--timeout` | status `15s` / prune `60s` | 数据库操作超时 |
| `--json` | `false` | 输出结构化 JSON |

普通 dry-run 输出示例：

```text
mode:      dry-run
retention: enabled=true run_days=30 session_days=7 batch_size=1000
runs:      eligible=12 deleted=0
sessions:  eligible=2 expired=0 deleted=0
batches:   0
cutoff:    runs=2026-10-20T01:50:54Z sessions=2026-11-12T01:50:54Z
note:      no data was deleted; rerun with --yes after reviewing the plan
```

退出码：

```text
0  查询 / dry-run / 执行成功
1  数据库错误，或 --yes 时 retention 未启用
2  配置或参数错误
```

---

## 6. App 命令

### 6.1 `levier app init`

```sh
levier app init \
  --name <app> \
  [--credentials-out path] \
  [--server url] \
  [--admin-token token] \
  [--rotate] \
  [--yes] \
  [--force] \
  [--dry-run] \
  [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--name` | `LEVIER_APP_NAME` | 必填，App 名称 |
| `--credentials-out` | `LEVIER_CREDENTIALS` 或 `~/.config/levier/<app>.json` | 凭据输出路径 |
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址，会写入凭据文件 |
| `--admin-token` | `LEVIER_ADMIN_TOKEN` | 创建 App 所需 |
| `--rotate` | `false` | App 已存在时轮换 key |
| `--yes` | `false` | 非交互确认轮换 |
| `--force` | `false` | 覆盖已有凭据文件 |
| `--dry-run` | `false` | 只展示将发生什么，不发写请求 |
| `--timeout` | `10s` | 单次请求超时 |
| `--json` | `false` | JSON 输出 |

行为：

```text
--dry-run 不要求 Admin token，不写文件
凭据文件已存在且没有 --force -> 退出 2
App 已存在且没有 --rotate -> 提示并用 rotate-key，退出 0
--rotate 需要确认；非 TTY 必须 --yes
写入凭据：0600、原子写入、覆盖时生成 .bak
创建或轮换后先用新 key 验证 App，再报告成功
--json 不输出 api_key，只输出不可逆指纹
```

JSON 字段：

```json
{
  "app_id": "app_...",
  "app_name": "demo",
  "server": "http://127.0.0.1:8080",
  "credentials": "./demo.json",
  "credential_version": 1,
  "key_fingerprint": "a1b2c3d4e5f6",
  "created": true,
  "rotated": false
}
```

可选字段：

```text
backup      覆盖凭据文件时输出
```

退出码：

```text
0  创建成功、App 已存在但未轮换、dry-run 成功
1  写入凭据失败或服务端返回空 key
2  参数错误、缺少 Admin token、凭据已存在且未 --force
3  Admin 认证失败
4  网络 / 服务端错误
5  轮换确认被拒绝
```

### 6.2 `levier app show`

```sh
levier app show --credentials <path> [--server url] [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--credentials` | `LEVIER_CREDENTIALS` | 必填 |
| `--server` | 凭据文件中的地址，或 `LEVIER_SERVER` | HTTP 地址 |
| `--timeout` | `10s` | 单次请求超时 |
| `--json` | `false` | JSON 输出 |

输出包含服务端 App 元数据，以及本地凭据文件路径、版本、服务地址和 key 指纹；不会输出 key：

```json
{
  "app_id": "app_...",
  "name": "demo",
  "status": "active",
  "multiagent": false,
  "created_at": "...",
  "updated_at": "...",
  "credentials": "./demo.json",
  "credential_version": 1,
  "server": "http://127.0.0.1:8080",
  "key_fingerprint": "a1b2c3d4e5f6"
}
```

退出码：

```text
0  成功
1  其他服务端错误
2  参数错误或凭据文件不可读
3  App key 无效
4  网络错误
```

### 6.3 `levier app rotate-key`

```sh
levier app rotate-key \
  --credentials <path> \
  [--admin-token token] \
  --yes \
  [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--credentials` | `LEVIER_CREDENTIALS` | 必填，读取 app_id 并写回新 key |
| `--admin-token` | `LEVIER_ADMIN_TOKEN` | 必填 |
| `--server` | 凭据文件中的地址，或 `LEVIER_SERVER` | HTTP 地址 |
| `--yes` | `false` | 非交互确认轮换 |
| `--timeout` | `10s` | 单次请求超时 |
| `--json` | `false` | JSON 输出 |

行为：

```text
读取 app_id
确认后轮换 key
旧 key 立即失效
写回凭据文件并生成 .bak
--json 不输出 api_key
```

退出码：

```text
0  轮换成功
1  写入凭据失败或服务端返回空 key
2  参数错误、缺少 Admin token、凭据不可读
3  Admin 认证失败
4  网络错误
5  终端里拒绝确认，或非 TTY 没有 --yes
```

### 6.4 `levier app check`

```sh
levier app check --credentials <path> [--server url] [--json]
```

只读检查本地凭据和接入链路：

```text
凭据文件是否存在、格式是否正确、版本是否支持
凭据文件权限是否为 0600
服务端是否可连接
App key 是否有效
App 是否处于 active
是否能够取得 WebSocket token
```

不会创建 Run，也不会修改工具。JSON 中只包含检查结果和 key 指纹，不包含 key 或 WebSocket token。

| 参数 | 默认 | 说明 |
|---|---|---|
| `--credentials` | `LEVIER_CREDENTIALS` | 必填 |
| `--server` | 凭据文件中的地址，或 `LEVIER_SERVER` | HTTP 地址 |
| `--timeout` | `10s` | 单次请求超时 |
| `--json` | `false` | JSON 输出 |

退出码：`0` 全部通过；`1` 有检查失败；`2` 参数或凭据文件错误。

### 6.5 `levier app list`

```sh
levier app list [--server url] [--admin-token token] [--json]
```

需要 Admin token，返回 App ID、名称、状态和创建/更新时间，不返回 API key 或 hash。

退出码：`0` 成功；`1` 服务端错误；`2` 参数或 Admin token 错误。

---

## 7. Run 命令

### 7.1 `levier run submit`

```sh
levier run submit \
  --credentials <path> \
  --prompt "<text>" \
  [--runtime <name>] \
  [--watch] \
  [--json]
```

常用参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--credentials` | `LEVIER_CREDENTIALS` | 必填 |
| `--prompt` | 无 | 必填 |
| `--runtime` | `react1` | Runtime 路由名 |
| `--provider` | `deepseek` | 模型 provider |
| `--model` | `deepseek-flash` | 模型名 |
| `--max-steps` | `8` | ReAct 最大步数 |
| `--run-ms` | `60000` | Run 超时，毫秒 |
| `--tool-ms` | `10000` | 单工具超时，毫秒 |
| `--idempotency-key` | 空 | 可选 Idempotency-Key |
| `--request-id` | 空 | 可选 X-Request-ID / trace 关联 |
| `--watch` | `false` | 提交后等待终态 |
| `--interval` | `500ms` | watch 轮询间隔 |
| `--watch-timeout` | `30m` | watch 总超时 |
| `--timeout` | `10s` | 单次 HTTP 请求超时 |
| `--json` | `false` | JSON 输出 |

不等待：

```sh
levier run submit \
  --credentials ./demo.json \
  --runtime echo1 \
  --prompt "hello"
```

输出：

```text
run submitted: 01...
status: queued
next: levier run watch --run-id 01... --credentials ./demo.json
```

JSON：

```json
{
  "run_id": "01...",
  "status": "queued"
}
```

等待终态：

```sh
levier run submit \
  --credentials ./demo.json \
  --runtime echo1 \
  --prompt "hello" \
  --watch
```

`--watch --json` 只输出最终 watch 结果：

```json
{
  "run_id": "01...",
  "status": "completed",
  "attempt": 1,
  "final_message": "...",
  "events": 16
}
```

退出码：

```text
0  提交成功（未 watch）或 completed
1  failed 或其他非预期终态
2  参数错误、凭据不可读
3  App 认证失败
4  网络 / 服务端错误
5  cancelled、timeout 或 watch 超时
```

### 7.2 `levier run watch`

```sh
levier run watch \
  --run-id <id> \
  --credentials <path> \
  [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--run-id` | 无 | 必填 |
| `--credentials` | `LEVIER_CREDENTIALS` | 必填 |
| `--interval` | `500ms` | 轮询间隔 |
| `--timeout` | `30m` | watch 总超时 |
| `--request-timeout` | `10s` | 单次 HTTP 请求超时 |
| `--json` | `false` | JSON 输出 |

终态映射：

```text
completed  -> 0
failed     -> 1
cancelled  -> 5
timeout    -> 5
watch 超时 -> 5
```

### 7.3 `levier run cancel`

```sh
levier run cancel \
  --run-id <id> \
  --credentials <path> \
  --yes
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--run-id` | 无 | 必填 |
| `--credentials` | `LEVIER_CREDENTIALS` | 必填 |
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址 |
| `--yes` | `false` | 非交互确认取消 |
| `--timeout` | `10s` | 单次请求超时 |
| `--json` | `false` | JSON 输出 |

行为：

```text
先读取 Run 状态
已经处于终态 -> 退出 5
非 TTY 没有 --yes -> 退出 5
成功取消 -> 输出 run_id + status
```

JSON：

```json
{
  "run_id": "01...",
  "status": "cancelled"
}
```

退出码：

```text
0  取消成功
1  其他服务端错误
2  参数错误或凭据不可读
3  App 认证失败
4  网络错误
5  已经是终态或确认被拒绝
```

---

## 8. Ops 命令

```sh
levier ops runs list
levier ops runs show --run-id <id>
levier ops runs cancel --run-id <id> --reason "..." --yes
levier ops daemons
levier ops tasks --status DEAD
```

所有命令使用 Admin token：

```text
--admin-token 或 LEVIER_ADMIN_TOKEN
```

### 8.1 `levier ops runs list`

```sh
levier ops runs list [--status running] [--app-id id] [--daemon-id id] \
  [--session-id id] [--limit 50] [--cursor value] [--json]
```

列表按创建时间倒序，返回 `next_cursor` 时传给 `--cursor` 获取下一页。

### 8.2 `levier ops runs show`

```sh
levier ops runs show --run-id <id> [--json]
```

显示 Run 状态、App、assigned daemon、attempt、最后事件位置和 RunTask。任务输出不包含 payload。

### 8.3 `levier ops runs cancel`

```sh
levier ops runs cancel --run-id <id> [--reason text] --yes [--json]
```

终态 Run 返回原状态；非终态 Run 走已有 CancelRun 流程。没有 `--yes` 不发送取消请求。

### 8.4 `levier ops daemons`

```sh
levier ops daemons [--json]
```

只读显示 daemon 容量、running/reserved/available、健康状态和最后心跳。

### 8.5 `levier ops tasks`

```sh
levier ops tasks [--status DEAD] [--type RUN_DISPATCH] [--run-id id] \
  [--limit 100] [--offset 0] [--json]
```

默认查看 `DEAD`。不返回 Task payload；本版不支持重试。

Admin API：

```text
GET  /admin/runs
GET  /admin/runs/{run_id}
POST /admin/runs/{run_id}/cancel
GET  /admin/daemons
GET  /admin/tasks
```

---

## 9. 迁移命令

### 9.1 `levier migrate status`

```sh
levier migrate status [--config path] [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | YAML 配置 |
| `--timeout` | `30s` | 数据库操作超时 |
| `--json` | `false` | JSON 输出 |

只读。输出：

```text
database: postgres://user:change-me@host:5432/levier?sslmode=disable
current: 11
applied: 11
pending: 0
```

JSON：

```json
{
  "database": "postgres://user:change-me@host:5432/levier?sslmode=disable",
  "current": 10,
  "applied": 11,
  "pending": []
}
```

`pending` 元素：

```json
{
  "version": 10,
  "name": "add_example",
  "filename": "0010_add_example.sql"
}
```

退出码：`0` 成功（即使有 pending）；`1` 数据库 / 查询失败；`2` 配置或参数错误。

### 9.2 `levier migrate up`

```sh
levier migrate up [--config path] [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | YAML 配置 |
| `--timeout` | `2m` | 数据库操作超时 |
| `--json` | `false` | JSON 输出 |

行为：

```text
打印目标库和 pending
执行迁移
输出 applied / current
已经是最新 -> already up to date，退出 0
不要求 --yes
```

JSON：

```json
{
  "database": "postgres://user:change-me@host:5432/levier?sslmode=disable",
  "before": 9,
  "after": 10,
  "applied": 1,
  "pending": 0
}
```

退出码：`0` 成功；`1` 数据库 / 迁移失败；`2` 配置或参数错误。

迁移策略：

```text
Compose / 生产：migrate up -> serve
serve 启动时只检查 pending，不自动迁移
```

---

## 10. `levier daemon check`

```sh
levier daemon check \
  [--grpc 127.0.0.1:9090] \
  [--server http://127.0.0.1:8080] \
  [--daemon-id daemon-local-1] \
  [--token change-me] \
  [--admin-token ...] \
  [--runtime echo1] \
  [--json]
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--grpc` | `LEVIER_GRPC` 或 `127.0.0.1:9090` | gRPC 地址 |
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址 |
| `--daemon-id` | `LEVIER_DAEMON_ID` 或 `daemon-local-1` | 预分配 daemon id |
| `--token` | `LEVIER_DAEMON_TOKEN` 或 `change-me` | daemon token |
| `--admin-token` | `LEVIER_ADMIN_TOKEN` 或 `admin-change-me` | 创建临时 App |
| `--runtime` | `echo1` | 声明并运行的策略 |
| `--timeout` | `30s` | 单步超时 |
| `--json` | `false` | JSON 输出 |

`change-me` / `admin-change-me` 只适合本地示例；生产必须显式传入。

作用：

```text
模拟一个 RuntimeChannel daemon
创建临时 App / Run
验证 hello、task_assign、ack、completed、cancel、cancelled
```

注意：

```text
这是诊断命令，不是只读命令
会创建临时 App 和 Run
必须使用预分配且空闲的 daemon id/token
同一 daemon id 已有真实 Harness 时，服务端返回 AlreadyExists
```

退出码：

```text
0  通过
1  检查项失败
2  参数错误
3  Admin / daemon 认证失败
4  网络 / 连接失败
5  daemon id 已被占用
```

旧入口仍保留：

```sh
go run ./cmd/mw-daemon-check --grpc 127.0.0.1:9090 --http http://127.0.0.1:8080
```

---

## 11. 兼容入口

```text
levier [--config path]       等同于 levier serve
cmd/mw-daemon-check          旧 daemon 自检入口，内部复用 internal/daemoncheck
cmd/migrate                  旧迁移入口，保留兼容；新路径用 levier migrate up
```

---

## 12. 尚未实现

以下出现在路线图中，但当前版本**没有**实现：

```text
--quiet（全局输出选项）
levier completion
levier tui
levier upgrade
```

不要在生产脚本中使用未实现命令。

---

## 13. 常见错误

### `credentials path is required`

```text
缺少 --credentials 或 LEVIER_CREDENTIALS
```

### `admin token is required`

```text
app init / rotate-key 需要 --admin-token 或 LEVIER_ADMIN_TOKEN
```

### `refusing without --yes in a non-interactive session`

```text
CI / 管道中执行 rotate-key 或 run cancel 必须显式 --yes
```

### `database schema is behind`

```text
serve 检测到 pending migration
先执行 levier migrate up -config <path>
```

### `daemon "..." already has an active stream`

```text
daemon check 使用的 id 正被真实 Harness 占用
换一个预分配的空闲 id，或临时停止该 Harness
```

### `run watch timed out`

```text
Run 未在 --timeout / --watch-timeout 内进入终态
检查 Harness、模型和工具调用，必要时用 levier status / doctor
```

---

## 14. 退出码汇总

| 命令 | 0 | 1 | 2 | 3 | 4 | 5 |
|---|---|---|---|---|---|---|
| `version` | 成功 | - | 参数 | - | - | - |
| `serve` | 正常退出 | 启动失败 | 参数 | - | - | - |
| `doctor` | 全部通过 | 有 fail | 参数 | - | - | - |
| `status` | 全部 ok | 有异常 | 参数 | - | - | - |
| `app init` | 成功 | 写入/验证失败 | 参数 / 冲突 | 认证 | 网络 | 确认拒绝 |
| `app show` | 成功 | 其他错误 | 参数 / 凭据 | 认证 | 网络 | - |
| `app check` | 全部通过 | 有检查失败 | 参数 / 凭据 | - | - | - |
| `app list` | 成功 | 服务端错误 | 参数 / Admin token | 认证 | 网络 | - |
| `app rotate-key` | 成功 | 写入/验证失败 | 参数 / 凭据 | 认证 | 网络 | 确认拒绝 |
| `run submit` | 成功 / completed | failed | 参数 | 认证 | 网络 | cancelled / timeout |
| `run watch` | completed | failed | 参数 | 认证 | 网络 | cancelled / timeout |
| `run cancel` | 取消成功 | 其他错误 | 参数 / 凭据 | 认证 | 网络 | 终态 / 确认拒绝 |
| `ops runs list/show` | 成功 | 服务端错误 | 参数 / 过滤 | 认证 | 网络 | - |
| `ops runs cancel` | 取消成功 | 服务端错误 | 参数 | 认证 | 网络 | 确认拒绝 |
| `ops daemons` | 成功 | 服务端错误 | 参数 | 认证 | 网络 | - |
| `ops tasks` | 成功 | 服务端错误 | 参数 / 过滤 | 认证 | 网络 | - |
| `migrate status` | 成功 | 数据库错误 | 配置 / 参数 | - | - | - |
| `migrate up` | 成功 | 迁移错误 | 配置 / 参数 | - | - | - |
| `maintenance status` | 成功 | 数据库错误 | 配置 / 参数 | - | - | - |
| `maintenance prune` | 成功 | 数据库错误 / retention 未启用 | 配置 / 参数 | - | - | - |
| `daemon check` | 通过 | 检查失败 | 参数 | 认证 | 网络 | daemon 冲突 |

---

## 15. 维护约定

CLI 文档与代码同步位置：

```text
cmd/levier/cli.go          顶层命令分发
cmd/levier/*.go            子命令参数与输出
cmd/levier/flags.go        --help 行为
cmd/levier/*_test.go       帮助、JSON、退出码回归
```

修改任一命令的以下内容时，必须同步更新本文：

```text
命令名
参数名或默认值
环境变量
JSON 字段
退出码
确认行为
```

验收：

```sh
go test ./cmd/levier
make lint
make structure-check
```
