<!-- Levier — single-node MVP guide. -->

# 单机 MVP 指南

这份文档只描述**现在能稳定演示和验收**的最小形态。
它不是生产部署手册，也不覆盖并发、HA、sandbox 或 longterm。

## 1. 范围

```text
一个 Levier 进程
一个 PostgreSQL
一个 Harness 进程
一个 App 进程
runtime = normal
```

MVP 必须做到：

```text
Run 能创建、排队、调度、完成
工具调用能经过 App WebSocket 闭环
事件能落库，并可按 cursor 回放
取消、Run/Step/Tool 超时能收敛
Harness 断线后 Run 能重排
MW 重启后 queued/active Run 能恢复
```

MVP 不承诺：

```text
工具 exactly-once
多 Harness 竞争下的最优放置
sandbox 隔离
longterm checkpoint 续跑
多 MW / 多实例 / HA
高并发与压测指标
```

## 2. 崩溃语义

当前 normal runtime 是 **at-least-once**：

```text
Harness 在 Run 中途崩溃
→ MW 把 Run 重新排队
→ 新 attempt 从头执行
→ 已执行过的 local 工具可能再次执行
```

因此有副作用的工具必须二选一：

```text
A. App 按 invocation_id 做幂等
B. 接受重复副作用，并在业务上可补偿
```

`counter_add` 是演示工具，不保证 exactly-once。

## 3. 准备

### 3.1 环境文件

```sh
cp -n .env.example .env
```

至少设置：

```text
LEVIER_DATABASE_URL   PostgreSQL DSN
LEVIER_ADMIN_TOKEN            与 config 的 http.admin_token 一致
```

`react` 模式还需要：

```text
DEEPSEEK_API_KEY
DEEPSEEK_BASE_URL
DEEPSEEK_MODEL
```

`.env` 和 `.env.local` 都不会提交；密钥不会打印到 demo 输出。

### 3.2 依赖

```sh
make build
(cd execution/harness && uv sync --locked)
```

### 3.3 PostgreSQL

本机示例（Homebrew，端口 5433）：

```sh
pg_ctl -D /opt/homebrew/var/postgresql@16 -o "-p 5433" start
```

如果使用 5433，把 `.env` 的 DSN 改成 `:5433`。
`tests/e2e/mvp_demo.sh` 会先跑迁移，连不上数据库会直接失败并指向迁移日志。

## 4. 运行

### 4.1 echo：不依赖模型

```sh
make mvp-demo
```

验证：

```text
MW 启动
Harness 连接
App init + SDK 工具注册 + WS 连接
提交 echo Run
Run completed
MESSAGE_COMPLETED 内容为 hello mvp
```

### 4.2 故障矩阵

```sh
make fault-matrix
```

覆盖：

```text
Harness kill -9 中途 → requeue → attempt 2 完成
MW 重启中途 → Harness 重连 → Run 完成
App 断线 → 工具调用失败 → App 重启后新 Run 完成
取消 → cancelled + source=mw 事件 + 无 active task
超时 → timeout + source=mw 事件 + 无 active task
```

输出在 `data/fault-matrix/<时间>/`。

### 4.3 react：真实模型 + 本地工具

```sh
make mvp-demo-react
```

验证：

```text
3 次工具调用（counter_add ×2、counter_get ×1）
TOOL_CALL_BEGIN = TOOL_CALL_END = 3
无 ERROR 事件
最终回答包含 5
```

成功输出示例：

```text
mode: react
run_id: 01M...
status: completed
attempt: 1
events: 16
tool_calls: begin=3 end=3
errors: 0
final_message: The final counter value is **5**...
mvp-demo: PASS
```

## 5. 脚本做了什么

`tests/e2e/mvp_demo.sh`：

```text
1. 读取 .env.local / .env
2. make build
3. levier migrate up
4. levier serve（启动时检查 pending migration）
5. 等 /healthz
6. 启动 harness
7. 用 `levier app init` 创建一个唯一 App
8. 启动 `examples/counter_agent/app.py`（SDK 注册工具 + 连 WS）
9. 用 `levier run submit` 提交 Run
10. 轮询 /api/v1/runs/{id}
11. 拉事件、统计工具调用、校验终态
12. 写 summary.txt
13. 退出时 kill 进程并删除凭据文件
```

临时输出：

```text
data/mvp-demo/<时间>-<mode>/
  middleware.log
  harness.log
  serve.log
  bootstrap.json
  submit_run.json
  summary.txt
```

`data/` 已 gitignore。日志里不写 App key 或模型 key。

## 6. 常见问题

| 现象 | 原因 | 处理 |
|---|---|---|
| `LEVIER_DATABASE_URL is required` | 没 source `.env` | `cp -n .env.example .env` 后重跑 |
| 迁移失败 | PostgreSQL 未启动或端口不对 | 看 `data/mvp-demo/.../migrate.log` |
| `already serving` | 8080 上还有旧 MW | 停掉旧进程或改 `LEVIER_URL` |
| App did not start serving | app init/注册/WS 失败 | 看 `serve.log`、`serve.err` |
| react 模式失败 | 没有有效 `DEEPSEEK_API_KEY` | 设置 key 或先用 echo 模式 |
| Run failed | 模型/工具错误 | 看 `summary.txt` 和 `middleware.log` |

## 7. 安全边界

```text
Admin token 只用于 bootstrap/rotate
运行时只用 App key
App key 只在 bootstrap 时出现一次，保存在 demo 目录下的 0600 文件
demo 退出时删除凭据文件
不把 key 写进日志、summary 或 git
```

自动检查：

```sh
make secrets-check
```

扫描 git-tracked 源文件，排除生成代码和 evidence。
覆盖 `sk-*`、Bearer、PostgreSQL 带密码 DSN、`DEEPSEEK_API_KEY=<value>`、
以及明显的 inline API key。示例配置使用 `change-me` 占位符。

共享机器上仍建议使用独立的 App 名和数据库。

## 8. 已完成与剩余风险

已完成的可用性修复：

```text
R17 Hello 对账收敛
R20 控制面终态事件 + timeout 清理
R13 关键事件保护
R14 Gateway panic 兜底
R12 有界 replay / close-send
R21 Harness 退避与出站背压
R19b CreateRun + Enqueue 同事务
R2  /readyz
R8  密钥扫描
R22 成功请求访问日志
```

仍未覆盖：

```text
Docker Compose 真实构建/启动（配置已提供，需在有 Docker 的环境执行）
PostgreSQL 整体停机演练
40s 长工具真实 E2E
多 Harness / 多 MW / HA
sandbox / longterm
```

最终回归证据保留在本地 `docs/reports/`（不纳入版本库）。
