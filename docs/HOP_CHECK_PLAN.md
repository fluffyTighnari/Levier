<!-- Levier — hop-by-hop self-check plan (development document). -->

# 9. 逐跳自检改造计划

> 状态：第一版已实现：只读 selfcheck + `--deep` daemon protocol / App WebSocket / MCP health。
> 本文位于 `docs/`，属于内部开发文档，不进入镜像、release、HTTP 响应或对外分发。
> 对外暴露的自检结果格式放在 `public/service-selfcheck/v1/`。

---

## 1. 目标

提供一个命令，按链路顺序检查 Levier 的每一跳：

```sh
levier selfcheck
```

如果链路中断，用户能直接看到是哪一跳失败，而不是只得到一个笼统的 `PASS/FAIL`：

```text
[1/10] config             ok
[2/10] http /healthz      ok
[3/10] database           ok
[4/10] migrations         ok
[5/10] http /readyz       ok
[6/10] service /meta      ok
[7/10] service /status    ok
[8/10] admin auth         ok
[9/10] harness registered ok
[10/10] echo run          skipped (no --deep)

selfcheck: PASS (with skipped hops)
```

目标是让“服务是否真的能工作”从一个端到端黑盒结果，变成一条可定位的分段诊断链。

---

## 2. 和现有检查的关系

现有能力：

```text
levier doctor          配置 / DB / migration / health / ready / meta / admin / app
levier daemon check    daemon Hello / TaskAssign / ack / event / cancel
tests/app/app_check.py App HTTP / WS / ws_token / 消息校验 / 工具调用
deploy/smoke.py        Compose 端到端 echo Run
```

第 9 阶段不重复实现这些检查，而是做统一编排：

```text
levier selfcheck
  ├─ 复用 doctor 的被动检查思路
  ├─ 复用 daemoncheck 的主动协议检查
  ├─ 可选调用 App 自检逻辑
  └─ 把结果映射成有序 hop
```

`daemon check` 和 `app_check.py` 继续保留，作为单项深度诊断。

---

## 3. 自检分层

### 3.1 默认：只读自检

不创建 App / Run，不修改数据库，适合：

- 部署前
- 生产巡检
- 故障定位
- CI

检查：

```text
config
http /healthz
database
migrations
http /readyz
service /meta
service /status
admin auth（有 admin token 时）
daemon registered（有 daemon 信息时）
app credentials（有 credentials 时，只读验证）
```

### 3.2 `--deep`：主动链路自检

在默认检查基础上，创建临时 App / Run：

```text
temp App
  → TaskAssign
  → Harness ack
  → RUN_STATUS
  → completed
```

可选：

```text
--credentials  → 验证真实 App WebSocket
--mcp-socket   → 验证 MCP runtime health
--daemon-token → 验证 daemon 协议链路
```

真实 local tool 调用继续使用 `tests/app/app_check.py --run-tool`。

---

## 4. 自检 hops

### 4.1 默认 hops

| # | hop | 检查 | 失败/跳过条件 |
|---:|---|---|---|
| 1 | `config` | 本地 YAML + env + secret 文件 | 配置无法加载 |
| 2 | `http` | `GET /healthz` | 进程不可达 |
| 3 | `database` | 数据库连接 | DB 不可用 |
| 4 | `migrations` | pending migration | schema 落后 |
| 5 | `ready` | `GET /readyz` | 依赖未就绪 |
| 6 | `meta` | `GET /meta` / `meta_version` | 服务不兼容 |
| 7 | `status` | `GET /status` / `status_version` | 状态接口不可用 |
| 8 | `admin` | Admin token 调用管理接口 | 无 token 或认证失败 |
| 9 | `daemon` | daemon 注册情况 | 无 daemon；只警告或按照参数失败 |
| 10 | `app` | App key / App 状态（提供凭据时） | 凭据无效 |

### 4.2 `--deep` hops

| # | hop | 检查 |
|---:|---|---|
| 11 | `dispatch` | 创建临时 App 和 Run，观察 TaskAssign |
| 12 | `events` | 收到 running / completed 事件 |
| 13 | `ws` | 真实 App WebSocket hello + ping/pong（提供凭据时） |
| 14 | `mcp` | MCP runtime health（`--mcp-socket`） |
| 15 | `app websocket` | 真实 App hello / pong（提供凭据时） |

local tool 调用继续由 `tests/app/app_check.py --run-tool` 负责，不在第一版 selfcheck 中重复实现。

each hop has:

```text
ok
fail
skip
```

`skip` 不等于失败，但会在摘要中显示。

---

## 5. CLI 设计

```sh
levier selfcheck [options]
```

建议参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| `--config` | `LEVIER_CONFIG` 或 `levier.example.yaml` | 本地配置 |
| `--server` | `LEVIER_SERVER` 或 `http://127.0.0.1:8080` | HTTP 地址 |
| `--grpc` | `LEVIER_GRPC` 或 `127.0.0.1:9090` | gRPC 地址 |
| `--admin-token` | `LEVIER_ADMIN_TOKEN` | Admin 检查 |
| `--credentials` | `LEVIER_CREDENTIALS` | App 检查 |
| `--daemon-id` | `LEVIER_DAEMON_ID` | 预期 daemon |
| `--daemon-token` | `LEVIER_DAEMON_TOKEN` | deep 协议检查 |
| `--mcp-socket` | 空 | MCP runtime socket |
| `--deep` | `false` | 启用主动链路检查 |
| `--timeout` | `5s` | 单 hop 超时 |
| `--json` | `false` | JSON 输出 |

### 输出

人类可读：

```text
selfcheck: levier http://127.0.0.1:8080
[1/10] config             ok
[2/10] http /healthz      ok
[3/10] database           ok
[4/10] migrations         ok
[5/10] http /readyz       ok
[6/10] service /meta      ok
[7/10] service /status    ok
[8/10] admin auth         ok
[9/10] harness registered ok
[10/10] app credentials   skip (no --credentials)

selfcheck: PASS (10 hops, 1 skipped)
```

JSON：

```json
{
  "ok": true,
  "server": "http://127.0.0.1:8080",
  "started_at": "2026-11-18T12:00:00Z",
  "finished_at": "2026-11-18T12:00:02Z",
  "hops": [
    {
      "name": "config",
      "status": "ok",
      "message": "levier.example.yaml",
      "duration_ms": 3
    }
  ]
}
```

不输出：

- token
- API key
- DSN
- secret 文件内容
- daemon_id / app_id / run_id（除非 `--json` 中的明确 `run_id` 仅用于诊断且不进入公开契约？建议默认不输出）

---

## 6. 执行顺序和短路

```text
config
  ↓ fail → 后续全部 skip
http
  ↓ fail → 后续全部 skip
database
  ↓ fail → ready/status/daemon 等 skip
migrations
  ↓ fail → ready 视为 degraded
ready
  ↓
meta
  ↓
status
  ↓
admin
  ↓
daemon
  ↓
app
  ↓
deep hops（如果启用）
```

原则：

- 先执行前置 hop
- 前置失败后，依赖 hop 标记为 `skip`
- 不把“没提供凭据”误报成 fail
- 单个 hop 失败不立即退出，继续收集可执行信息
- 最后统一返回退出码

---

## 7. 代码结构

新增：

```text
internal/selfcheck/
  doc.go
  selfcheck.go
  selfcheck_test.go
```

职责：

- `Hop` / `Result` / `Report` 类型
- 有序 hop runner
- 依赖关系
- 超时和脱敏
- `--deep` 主动检查
- 复用 `daemoncheck` / HTTP 客户端

CLI：

```text
cmd/levier/selfcheck.go
cmd/levier/selfcheck_test.go
```

修改：

```text
cmd/levier/cli.go
cmd/levier/main.go
docs/CLI_REFERENCE.md
docs/README.md
docs/TODO.md
deploy/smoke.py（可选 selfcheck hook）
```

---

## 8. 自检结果与 HTTP

第一阶段只做 **CLI 自检**。

不建议在公共 HTTP 上提供 `GET /selfcheck`，因为：

- `--deep` 会创建临时资源
- 自检会触发主动协议交互
- 可能被误用为攻击面

已有被动面：

```text
/healthz
/readyz
/meta
/status
/metrics
```

后续如果需要远程自检，增加受保护的管理接口：

```text
POST /admin/selfcheck
```

但这是后续阶段，不在 9 第一版。

---

## 9. 安全边界

自检输出不得包含：

```text
Admin token
daemon token
App API key
数据库 DSN
secret 文件内容
```

错误信息需要脱敏。

`--deep` 产生的临时 App / Run：

- 使用唯一名称
- 使用短超时
- 不删除已有资源
- 文档说明会产生测试数据

---

## 10. 分阶段实施

### 9A：计划与输出契约

- 冻结 hop 名称、顺序、状态
- 定义 JSON 输出
- 定义 exit code
- 建立 `public/service-selfcheck/v1/` schema / example（如果决定公开结果格式）

### 9B：只读 selfcheck

- `internal/selfcheck` 只读 runner
- config / health / DB / migration / ready / meta / status / admin / daemon / app
- CLI `levier selfcheck`
- `--json`
- 单 hop 测试

### 9C：`--deep`

- 复用 `daemoncheck`
- 临时 App / Run
- TaskAssign / ack / events / completed
- App WebSocket 检查（有 credentials 时）
- MCP health（有 `--mcp-socket` 时）
- local tool 继续使用 `tests/app/app_check.py --run-tool`

### 9D：文档与 smoke

- CLI_REFERENCE
- README
- deploy README
- smoke 增加 `selfcheck` 或等价检查
- 故障示例

### 9E：验收

```text
go test ./...
make lint
make contract-check
make react-runtime-test
make mcp-runtime-test
make counter-agent-test
docker compose config --quiet
docker compose up -d --no-build
docker compose --profile smoke run --rm smoke
```

---

## 11. 测试矩阵

| 测试 | 覆盖 |
|---|---|
| selfcheck 单元 | hop 顺序、依赖、skip |
| 脱敏测试 | token / DSN / API key 不出现 |
| config hop | 正常 / 缺失 / production 占位 |
| DB hop | 正常 / DB 不可用 |
| meta/status hop | 版本不支持 |
| admin hop | 无 token / 401 / 200 |
| daemon hop | 无 daemon / 有 daemon |
| deep hop | 临时 App / Run / completed |
| App WS | 有 credentials / 无 credentials |
| tool hop | 真实工具成功 / 超时 |
| MCP hop | socket 存在 / 不存在 |
| JSON | 结构、退出码 |
| Compose smoke | selfcheck 整体 PASS |

---

## 12. 完成标准

```text
[x] levier selfcheck 可运行
[x] 默认只读，不创建资源
[x] --deep 可检查 daemon / Run / App WebSocket / MCP health
[x] 失败能定位到具体 hop
[x] 缺失可选凭据时 skip，而不是 fail
[x] 输出不泄露 token / DSN / API key
[x] --json 稳定可解析
[x] daemon check / app_check.py 现有行为不破坏
[ ] smoke 覆盖 selfcheck
[x] go test ./... 通过
[x] make lint 通过
[x] GitHub Actions CI 绿
```

---

## 13. 需要确认的决策

1. 第一版实现 `levier selfcheck`，不新增公共 `/selfcheck` HTTP 接口
2. 默认只读；`--deep` 才创建临时 App / Run
3. `skip` 不算失败
4. 可选凭据缺失时跳过 App / MCP / tool hop
5. `--deep` 产生的临时测试数据在文档中明确说明
6. 失败时输出具体 hop 和修复建议，不只返回 PASS/FAIL
