<!-- Levier — action-oriented CLI design (5A-3). -->

# CLI 动作命令设计

> 本文是设计与执行记录，不是用户手册。
> 当前命令参考见 [`CLI_REFERENCE.md`](CLI_REFERENCE.md)；分发路线见 [`CLI_DISTRIBUTION_PLAN.md`](CLI_DISTRIBUTION_PLAN.md)。

| 项目 | 内容 |
|---|---|
| 状态 | 5A-3a/5A-3b/5A-3c/5A-3d 已完成；5A-4 文档收敛完成 |
| 范围 | `app`、`run`、`migrate`、`daemon` |
| 原则 | 只读操作安全；变更操作可确认、可回滚、可脚本化 |

---

## 1. 两类命令

只读：

```text
levier version
levier status
levier doctor
levier app show
levier migrate status
```

诊断（会创建临时 App / Run）：

```text
levier daemon check
```

变更：

```text
levier app init
levier app rotate-key
levier run submit
levier run cancel
levier migrate up
```

`run watch` 只读，但会持续等待终态。

---

## 2. 认证矩阵

| 命令 | 凭据 |
|---|---|
| `app init` | Admin token |
| `app rotate-key` | Admin token |
| `app show` | App key |
| `run submit` | App key |
| `run watch` | App key |
| `run cancel` | App key |
| `migrate status/up` | 数据库 DSN |
| `daemon check` | Admin token + Daemon token |

规则：

```text
Admin token 不进入 App 命令
App key 不进入 Admin 命令
凭据不打印、不写入日志
```

---

## 3. 凭据文件

格式：

```json
{
  "app_id": "app_...",
  "app_name": "counter-agent",
  "api_key": "mw_..."
}
```

要求：

```text
权限 0600
原子写入：临时文件 + rename
已存在时默认拒绝覆盖
--force 覆盖前生成 .bak
--json 不输出 api_key
```

默认路径：

```text
LEVIER_CREDENTIALS
或 os.UserConfigDir()/levier/<app-name>.json
```

---

## 4. 安全与确认

变更命令：

```text
--dry-run  只展示将发生什么
--yes      非交互确认
--json     机器可读
```

规则：

```text
TTY 下，破坏性操作要求输入 y/N
非 TTY 下，没有 --yes 直接失败
--dry-run 不写文件、不发写请求
```

破坏性操作：

```text
app rotate-key   旧 key 立即失效
app init --force 覆盖凭据文件
run cancel       改变 Run 状态
```

`migrate up` 不要求 `--yes`，但先打印目标库和 pending 数量。

---

## 5. 输出与退出码

人类可读输出要给出下一步：

```text
app created: app_...
credentials: /path/to/credentials.json (0600)
next: levier run submit --credentials /path/to/credentials.json --prompt "..."
```

`--json` 字段稳定：

```text
app init:
  {"app_id":"...","app_name":"...","credentials":"...","created":true}

run submit:
  {"run_id":"...","status":"queued"}

run watch:
  {"run_id":"...","status":"completed","attempt":1}

migrate status:
  {"version":9,"applied":9,"pending":0}

daemon check:
  {"ok":true,"checks":{"hello":"ok","task_assign":"01...","ack":"accepted"}}
```

退出码：

```text
0  成功
1  运行失败
2  用法 / 配置错误
3  认证失败
4  网络 / 连接失败
5  状态不允许
```

错误输出包含：

```text
error.code
error.message
error.request_id
fix
```

---

## 6. 命令设计

### 6.1 `levier app init`

```text
--name <app>
--credentials-out <path>
--server <url>
--rotate
--force
--dry-run
--json
```

行为：

```text
POST /admin/apps
201：保存 key
200：App 已存在，不返回 key；提示 rotate-key 或使用 --rotate
--rotate：对已有 App 调 rotate-key
```

不打印 key。

### 6.2 `levier app show`

```text
--credentials <path>
--server <url>
--json
```

```text
GET /api/v1/apps/{app_id}
```

输出 app_id、name、status、multiagent、时间。

### 6.3 `levier app rotate-key`

```text
--credentials <path>
--server <url>
--yes
--json
```

```text
POST /admin/apps/{app_id}/rotate-key
```

成功：

```text
备份旧凭据
原子写入新凭据
不打印新 key
```

### 6.4 `levier run submit`

```text
--credentials <path>
--prompt <text>
--strategy react|echo
--provider deepseek
--model deepseek-flash
--max-steps 8
--run-ms 60000
--tool-ms 10000
--idempotency-key <key>
--watch
--json
```

```text
POST /api/v1/runs
```

`--watch` 等价于 submit 后立即 watch。

### 6.5 `levier run watch`

```text
--credentials <path>
--run-id <id>
--interval 500ms
--timeout 30m
--json
```

第一阶段用轮询，不引入 WebSocket 客户端依赖。

退出码：

```text
completed -> 0
failed    -> 1
cancelled -> 5
timeout   -> 5
```

### 6.6 `levier run cancel`

```text
--credentials <path>
--run-id <id>
--yes
--json
```

```text
GET /api/v1/runs/{id} 先确认状态
POST /api/v1/runs/{id}/cancel
```

终态再 cancel：提示并返回 5。

### 6.7 `levier migrate status/up`

```text
--config <path>
--json
```

`status`：

```text
连接数据库
读取 schema_migrations
列出 pending
```

`up`：

```text
打印目标库与 pending 数量
执行迁移
输出 applied/version
```

### 6.8 `levier daemon check`

```text
--grpc <addr>
--server <url>
--daemon-id <id>
--token <token>
--admin-token <token>
--strategy <name>
--timeout <duration>
--json
```

实现：

```text
cmd/levier/daemon.go
internal/daemoncheck/daemoncheck.go
cmd/mw-daemon-check/main.go   兼容旧入口，薄封装
```

行为：

```text
创建临时 App
连接 RuntimeChannel，检查 hello / task_assign / ack / events / completed
创建第二个 Run，检查 cancel / cancelled
--json 输出 {ok, checks, error}
退出码：0 成功；1 检查失败；2 用法错误；3 认证失败；4 网络失败；5 daemon id 冲突
```

默认值来源：

```text
LEVIER_GRPC
LEVIER_SERVER
LEVIER_DAEMON_ID
LEVIER_DAEMON_TOKEN
LEVIER_ADMIN_TOKEN
```

注意：

```text
这是诊断命令，不是只读命令：会创建临时 App 和 Run。
必须使用预先分配且空闲的 daemon id/token。
如果同一个 daemon id 已有真实 Harness 连接，服务端会拒绝重复连接。
```

### 6.9 启动与迁移策略

统一行为：

```text
levier serve
  连接数据库
  检查 pending migration
  pending > 0 -> 报错并退出 1
  不自动执行 DDL

levier migrate up
  显式执行迁移
```

Compose entrypoint：

```sh
/app/levier migrate up -config "$CONFIG"
exec /app/levier serve -config "$CONFIG"
```

生产：

```text
migration Job -> levier migrate up
             -> levier serve
```

`serve --auto-migrate` 只作为后续的开发/单机选项，不作为默认行为。

---

## 7. 内部结构

```text
cmd/levier/app.go         app 子命令
cmd/levier/run.go         run 子命令
cmd/levier/migrate.go     migrate 子命令
cmd/levier/daemon.go      daemon 子命令
cmd/levier/apiclient.go   App/Admin HTTP 客户端
cmd/levier/credentials.go 凭据读写
cmd/levier/flags.go       --help / 参数错误统一处理
internal/daemoncheck/     Daemon 协议自检（CLI 与旧入口共用）
cmd/levier/confirm.go     交互确认
```

CLI 只做参数解析、调用和输出；不复制控制面业务逻辑。用户参考见
[`CLI_REFERENCE.md`](CLI_REFERENCE.md)。

---

## 8. 测试

```text
httptest 覆盖 create / rotate / get / submit / cancel
临时目录覆盖凭据写入、0600、备份、拒绝覆盖
非 TTY 下缺少 --yes 失败
--json 字段稳定
退出码映射
所有子命令 --help 退出 0，且不打印 token 默认值
```

---

## 9. 实施顺序

```text
5A-3a  app init / show / rotate-key
5A-3b  run submit / watch / cancel
5A-3c  migrate status / up
5A-3d  daemon check
5A-4   CLI 文档收敛
```

先做 5A-3a，因为它产出 `run` 命令需要的凭据文件。

---

## 10. 不做什么

```text
不做交互式 wizard
不做 GUI
不做自动更新
不把 Admin token 存进凭据文件
不让 CLI 直接操作数据库业务表
不在 --json 中输出密钥
```

---

## 11. 执行记录

### 2026-11-16：5A-3a 完成

```text
levier app init
levier app show
levier app rotate-key
```

实现：

```text
凭据 0600 + 原子写入 + .bak
--force 覆盖
--dry-run
--json
--yes 非交互确认
不打印 api_key
```

验证：

```text
go test ./cmd/levier                  PASS
app init 创建新 App                   PASS
app show 读取 App                     PASS
app rotate-key 轮换并备份             PASS
```

### 2026-11-16：5A-3b 完成

新增：

```text
cmd/levier/run.go
cmd/levier/run_test.go
```

命令：

```text
levier run submit --prompt ... [--strategy react] [--watch]
levier run watch --run-id ...
levier run cancel --run-id ... --yes
```

行为：

```text
支持 Idempotency-Key
轮询 GET /api/v1/runs/{id}
终态读取 events 中的 MESSAGE_COMPLETED
completed -> 0
failed -> 1
cancelled / timeout -> 5
非 TTY cancel 需要 --yes
```

验证：

```text
go test ./cmd/levier                          PASS
run submit（echo，真实容器）                   PASS
run watch（真实容器）                          PASS
run submit --watch --json（真实容器）           PASS
cancel 单元测试                                PASS
```

### 2026-11-17：5A-3c 完成

新增：

```text
internal/db/status.go
cmd/levier/migrate.go
cmd/levier/migrate_test.go
```

命令：

```text
levier migrate status [--config path] [--json]
levier migrate up [--config path] [--json]
```

行为：

```text
只读 status：current / applied / pending
up：显示目标库和 pending，再执行迁移
数据库 URL 脱敏
--json 输出稳定字段
```

验证：

```text
go test ./cmd/levier / internal/db             PASS
migrate status（真实容器，version 9）           PASS
migrate status --json（真实容器）               PASS
migrate up（真实容器，already up to date）      PASS
```

### 2026-11-17：启动迁移策略统一

```text
serve 启动检查 pending migration
pending > 0 -> 退出 1
Compose entrypoint: migrate up -> serve
```

验证：

```text
空库 levier serve -> pending 9，退出 1          PASS
docker compose smoke                            PASS
```

### 2026-11-17：5A-3d 完成

新增：

```text
internal/daemoncheck/daemoncheck.go
cmd/levier/daemon.go
cmd/levier/daemon_test.go
cmd/mw-daemon-check/main.go   改为薄封装
```

命令：

```text
levier daemon check [--grpc addr] [--server url] [--daemon-id id] [--json]
```

行为：

```text
复用原 mw-daemon-check 的协议一致性检查
创建临时 App / Run
检查 hello、task_assign、ack、completed、cancel、cancelled
--json 输出 {ok, checks, error}
退出码：0 成功；1 检查失败；2 用法错误；3 认证失败；4 网络失败；5 daemon id 冲突
```

验证：

```text
go test ./cmd/levier / internal/daemoncheck       PASS
daemon check --help                              PASS（退出 0）
daemon check 未知参数                            PASS（退出 2）
daemon check 错误 Admin token                    PASS（退出 3）
daemon check 不可达 server                       PASS（退出 4）
daemon check 重复 daemon id                      PASS（退出 5）
真实 stack 协议一致性检查                        PASS
临时停止 harness 以释放 daemon-local-1；检查后已恢复
```

### 2026-11-17：5A-4 完成（CLI 文档收敛）

新增：

```text
docs/CLI_REFERENCE.md
cmd/levier/flags.go
```

变更：

```text
所有子命令 --help 输出到 stdout，退出 0
未知参数仍退出 2，并输出用法到 stderr
--help 不打印 Admin / daemon token 的环境变量默认值
doctor 增加 pending migration 检查
docs/README.md / README.md / docs/MAP.md 指向 CLI_REFERENCE.md
CLI_ACTIONS.md / CLI_DISTRIBUTION_PLAN.md 顶部标注为设计/路线，指向用户参考
未实现的 config check/print、--quiet 明确标为计划项
```

验证：

```text
go test ./cmd/levier                            PASS
所有子命令 --help                               PASS（退出 0）
doctor --help                                   PASS
CLI_REFERENCE.md 与 --help / JSON 字段核对       PASS
doctor 正常 stack                               PASS（7/7，退出 0）
doctor 空库 / pending migration                 PASS（migrations fail，退出 1）
```
