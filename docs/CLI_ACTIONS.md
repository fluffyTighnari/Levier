<!-- Levier — action-oriented CLI design (5A-3). -->

# CLI 动作命令设计

| 项目 | 内容 |
|---|---|
| 状态 | 5A-3a 已完成；5A-3b/c/d 未开始 |
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
| `daemon check` | Daemon token |

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

第一阶段可以复用 `mw-daemon-check` 的逻辑；需要时把共用的自检代码抽成包。

---

## 7. 内部结构

```text
cmd/levier/app.go         app 子命令
cmd/levier/run.go         run 子命令
cmd/levier/migrate.go     migrate 子命令
cmd/levier/daemon.go      daemon 子命令
cmd/levier/apiclient.go   App/Admin HTTP 客户端
cmd/levier/credentials.go 凭据读写
cmd/levier/confirm.go     交互确认
```

CLI 只做参数解析、调用和输出；不复制控制面业务逻辑。

---

## 8. 测试

```text
httptest 覆盖 create / rotate / get / submit / cancel
临时目录覆盖凭据写入、0600、备份、拒绝覆盖
非 TTY 下缺少 --yes 失败
--json 字段稳定
退出码映射
```

---

## 9. 实施顺序

```text
5A-3a  app init / show / rotate-key
5A-3b  run submit / watch / cancel
5A-3c  migrate status / up
5A-3d  daemon check
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
