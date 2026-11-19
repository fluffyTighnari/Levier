<!-- Levier — operator run surface and admin read APIs. -->

# 运行操作面

> 第一版范围：Run 发现、Run 详情、单个 Run 取消、daemon 只读视图、workflow task 只读视图。
>
> 本阶段不包含 GUI、批量操作、daemon drain/resume 和 Dead Letter 重试。

---

## 1. 使用前提

```sh
export LEVIER_SERVER=http://127.0.0.1:8080
export LEVIER_ADMIN_TOKEN=...
```

所有 `levier ops` 命令使用 Admin token，不使用 App 凭据。Admin token 不要放进 App 的长期运行环境。

---

## 2. Run 列表

```sh
levier ops runs list
levier ops runs list --status running
levier ops runs list --app-id app_... --limit 20
levier ops runs list --daemon-id daemon-local-1
levier ops runs list --status queued,failed --json
```

支持：

```text
status       单个或用逗号分隔的多个状态
app-id       App 过滤
daemon-id    按 assigned daemon 过滤
session-id   按 session 过滤
limit         1..200，默认 50
cursor        上一页返回的 next_cursor
json          输出原始结构
```

列表按创建时间倒序，返回 `next_cursor` 时将它传回 `--cursor` 获取下一页。

---

## 3. Run 详情

```sh
levier ops runs show --run-id <run_id>
levier ops runs show --run-id <run_id> --json
```

详情包括：

```text
Run ID、App ID
status、strategy、attempt
assigned daemon
session ID、trace ID
最后事件 attempt / seq
创建和更新时间
当前 RunTask 状态、错误信息
```

任务详情不包含 Task payload，避免在操作面暴露执行输入。

---

## 4. 取消 Run

```sh
levier ops runs cancel \
  --run-id <run_id> \
  --reason "stuck in tool call" \
  --yes
```

行为：

```text
只接受非终态 Run
终态 Run 返回原状态，不伪造取消
通过已有 Control.CancelRun 流程取消 daemon 和本地工具
写入结构化 admin action 日志
```

`--yes` 是必须的。没有它，命令不会发送取消请求。

---

## 5. Daemon 视图

```sh
levier ops daemons
levier ops daemons --json
```

展示：

```text
daemon_id
status / healthy
runtime type
capacity
running / reserved / available
最后心跳
已声明策略
```

这一版只读。Drain/resume 留到后续版本，不能在当前操作面停调 daemon。

---

## 6. Workflow 任务

```sh
levier ops tasks
levier ops tasks --status DEAD
levier ops tasks --status PENDING,LEASED --type RUN_DISPATCH
levier ops tasks --run-id <run_id> --json
```

支持：

```text
status    逗号分隔，默认 DEAD
type      逗号分隔
run-id    按 Run 过滤
limit     默认 100
offset    默认 0
```

任务视图不包含 payload。Dead Letter 只用于查看和判断原因，本版不提供重试操作。

---

## 7. API

Admin API：

```text
GET  /admin/runs
GET  /admin/runs/{run_id}
POST /admin/runs/{run_id}/cancel
GET  /admin/daemons
GET  /admin/tasks
```

公开的 `/status` 不变，仍然只返回聚合信息，不返回 App、Run 或 daemon 标识符。

---

## 8. 安全边界

```text
只接受 Admin token
不返回 App key、daemon token、数据库 DSN
不暴露 Task payload
取消必须显式 --yes
单个 Run 操作，不做批量取消
不提供任意 SQL 或状态覆盖
```

---

## 9. 后续版本

```text
daemon drain / resume
Dead Letter 重试
批量操作
Web 控制台
多实例操作协调
```
