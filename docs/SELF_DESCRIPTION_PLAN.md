<!-- Levier — service self-description plan (development document). -->

# 7. 服务自描述改造计划

> 状态：已实现 7A–7E，Compose smoke 已通过。
> 本文位于 `docs/`，属于内部开发文档，不进入镜像、release、HTTP 响应或对外分发。
> 真正的对外暴露契约放在单独的 `public/` 区域。

---

## 1. 文档与暴露边界

先明确两类内容：

```text
docs/                         内部开发文档
public/                       唯一允许对外暴露的契约与示例
GET /meta                     运行时的服务自描述内容
```

规则：

```text
docs/ 不进入 Docker 镜像
docs/ 不进入 release 压缩包
docs/ 不作为 HTTP 静态资源暴露
docs/ 可以继续留在私有 Git 仓库中，供开发维护使用
public/ 才是外部客户端可以依赖的内容边界
```

> 如果后续决定“docs/ 不进 Git”，需要单独调整仓库结构；
> 本文先按“docs/ 是私有开发文档，但不对外发布”处理。

最终对外暴露的契约建议放在：

```text
public/
  service-meta/
    v1/
      schema.json
      example.json
```

`/meta` 的运行时输出必须符合 `public/service-meta/v1/schema.json`。
`docs/` 中的说明只解释设计和用法，不作为客户端依赖的契约。

---

## 2. 阶段目标

任何人或程序拿到一个运行中的 Levier，只需要：

```sh
curl http://localhost:8080/meta
```

或：

```sh
levier describe --server http://localhost:8080
```

就能知道：

- 服务身份
- 版本、commit、构建时间
- HTTP API 版本
- WebSocket 协议版本
- 支持的运行时类型
- 内置策略名称
- 工具作用域
- MCP transport
- 已实现功能
- 请求体、超时、token 等限制
- 认证方式
- 主要接口路径

`/meta` 不返回：

- 数据库密码
- Admin token
- daemon token
- MCP secret
- App API key
- 配置文件路径
- 数据库连接字符串

---

## 3. 本阶段不做什么

```text
不做服务注册中心
不做服务发现
不做动态 daemon 列表
不做动态容量和负载
不做新的 /status 接口
不做 SDK / Adapter
不声明尚未实现的 sandbox / longterm
不暴露 docs/ 内容
不发布正式版本
```

动态状态继续由：

```text
/healthz
/readyz
/metrics
levier status
```

负责。

---

## 4. `/meta` 契约

### 4.1 路径

```text
GET /meta
```

与 `/healthz`、`/readyz` 一样，不需要认证。
只返回公开的静态元数据。

### 4.2 响应结构

```json
{
  "meta_version": 1,
  "service": "levier",
  "build": {
    "version": "0.1.0-rc.5",
    "commit": "2ce014d",
    "built": "2026-11-18T00:00:00Z",
    "go": "go1.26",
    "platform": "linux/amd64"
  },
  "api": {
    "http_version": "v1",
    "http_base_path": "/api/v1",
    "ws_path": "/api/v1/ws",
    "ws_protocol": 1
  },
  "capabilities": {
    "runtime_types": ["normal"],
    "tool_scopes": ["local", "global"],
    "mcp_transports": ["stdio", "sse", "streamable_http"],
    "features": [
      "run_submit",
      "run_cancel",
      "event_replay",
      "app_registration",
      "mcp_management",
      "websocket",
      "local_tools",
      "global_tools",
      "mcp_tools"
    ]
  },
  "limits": {
    "max_body_bytes": 1048576,
    "default_run_timeout_ms": 1800000,
    "default_tool_timeout_ms": 60000,
    "max_tool_timeout_ms": 240000,
    "ws_token_ttl_seconds": 600
  },
  "auth": {
    "app": {
      "scheme": "bearer",
      "header": "Authorization"
    },
    "admin": {
      "scheme": "bearer",
      "header": "Authorization"
    },
    "websocket": {
      "scheme": "query",
      "parameter": "token"
    }
  },
  "endpoints": {
    "health": "/healthz",
    "ready": "/readyz",
    "metrics": "/metrics",
    "apps": "/api/v1/apps",
    "app": "/api/v1/apps/{app_id}",
    "runs": "/api/v1/runs",
    "run": "/api/v1/runs/{run_id}",
    "run_events": "/api/v1/runs/{run_id}/events",
    "run_cancel": "/api/v1/runs/{run_id}/cancel",
    "ws_token": "/api/v1/ws-token",
    "websocket": "/api/v1/ws",
    "admin_apps": "/admin/apps",
    "admin_mcp": "/admin/mcp"
  }
}
```

### 4.3 版本规则

```text
meta_version 是自描述协议版本，不是 Levier 版本
新增字段            保持 meta_version = 1
删除、改名、改语义  提升 meta_version
客户端必须忽略不认识的字段
```

### 4.4 能力声明原则

- `runtime_types` 只声明真实实现的 `normal`
- 不声明 `sandbox`、`longterm`
- 实际可用 Runtime 以 Harness `Hello.runtime_name` 为准
- `features` 只列出已经实现的闭环能力
- 第一版不声明 `sessions`、`multiagent`

`limits` 必须来自真实运行配置：

| 字段 | 来源 |
|---|---|
| `max_body_bytes` | HTTP API 实际配置 |
| `default_run_timeout_ms` | 当前默认 run 超时 |
| `default_tool_timeout_ms` | 当前默认 tool 超时 |
| `max_tool_timeout_ms` | HTTP 校验上限 |
| `ws_token_ttl_seconds` | `http.ws_token_ttl` |

### 4.5 响应头

```text
Content-Type: application/json
Cache-Control: no-store
X-Request-ID: ...
```

---

## 5. 公开契约文件

新增：

```text
public/service-meta/v1/schema.json
public/service-meta/v1/example.json
```

要求：

- `schema.json` 是客户端可以依赖的机器可读契约
- `example.json` 用占位值，不包含真实 secret
- `/meta` 的测试和 smoke 使用该 schema 校验
- `docs/` 中的 schema 说明不能成为第二份真相

后续可以考虑把 App OpenAPI 也迁移到：

```text
public/api/v1/openapi.yaml
```

但这不在本次自描述改造的第一步内，避免同时移动现有 API 文档。

---

## 6. CLI 设计

新增：

```sh
levier describe [--server URL] [--json] [--timeout 5s]
```

默认：

```text
LEVIER_SERVER
http://127.0.0.1:8080
```

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

`--json` 输出原始 `/meta` JSON。

`levier doctor` 增加：

```text
HTTP /meta
```

检查：

- `/meta` 可达
- 返回合法 JSON
- `service = levier`
- `meta_version` 受支持
- `api.http_version = v1`

旧服务端没有 `/meta` 时：

```text
fail
fix: upgrade server or use matching client version
```

---

## 7. 代码改造

### 7.1 新增 `internal/selfdesc`

```text
internal/selfdesc/
  doc.go
  selfdesc.go
  selfdesc_test.go
```

职责：

- 定义 `meta_version`
- 定义 `Document` / `Build` / `API` / `Capabilities` / `Limits` / `Auth` / `Endpoints`
- 从构建信息和配置构造文档
- 保证不包含 secret
- 提供稳定的 JSON 结构

### 7.2 HTTP

```text
internal/httpapi/self_description.go
```

新增：

```go
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request)
```

路由：

```go
s.mux.HandleFunc("GET /meta", s.handleMeta)
```

`httpapi.Config` 增加元数据字段，`NewServer` 在未提供时使用默认文档，保证测试和嵌入使用不崩。

### 7.3 启动装配

在 `cmd/levier/main.go` 构造：

```go
meta := selfdesc.New(selfdesc.Options{
    Version: version.Version,
    Commit:  version.Commit,
    Built:   version.Built,
    MaxBodyBytes: ...,
    RunTimeoutMS: ...,
    ToolTimeoutMS: ...,
    MaxToolTimeoutMS: ...,
    WSTokenTTL: cfg.HTTP.WSTokenTTL.Std(),
})
```

传入 `httpapi.Config`。

### 7.4 CLI

```text
cmd/levier/describe.go
cmd/levier/describe_test.go
```

修改：

```text
cmd/levier/cli.go
cmd/levier/main.go
cmd/levier/doctor.go
```

### 7.5 文档

```text
docs/SELF_DESCRIPTION_PLAN.md   内部计划（本文）
docs/CLI_REFERENCE.md            describe / doctor 文档
docs/README.md                   索引
docs/TODO.md                     7 阶段清单
public/service-meta/v1/*         对外契约
```

`docs/` 只是开发说明，不进入对外分发。

---

## 8. 分阶段实施

### 7A：契约冻结

- 建立 `public/service-meta/v1/schema.json`
- 建立 `public/service-meta/v1/example.json`
- 写本计划并更新文档索引
- 确定字段、版本规则、公开边界

验收：

- schema 可被 JSON Schema 工具解析
- example 不含 secret
- `/meta` 字段确认

---

### 7B：核心自描述包

- 新增 `internal/selfdesc`
- 构建信息装配
- capabilities / limits / auth / endpoints
- 单元测试
- 秘密不泄露测试

验收：

- 不同配置生成不同 limits
- 响应字段与 schema 一致
- 不包含 token、密码、DSN

---

### 7C：HTTP `/meta`

- 新增 `GET /meta`
- `httpapi.Config` 装配
- OpenAPI / 路由一致性测试
- 无认证访问测试
- no-store 响应头
- metrics / request ID 兼容

验收：

```sh
curl -s http://localhost:8080/meta
```

无需 token 即可得到完整元数据。

---

### 7D：CLI 与 doctor

- `levier describe`
- `levier describe --json`
- `doctor` 增加 `/meta` 检查
- 参数错误、服务不可达、JSON 错误测试
- CLI 帮助和文档

验收：

```sh
levier describe
levier describe --json
levier doctor
```

---

### 7E：smoke 与文档闭环

- `deploy/smoke.py` 增加 `GET /meta`
- 更新 README / CLI_REFERENCE
- 更新 TODO
- 运行完整验证

验收：

```sh
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

## 9. 测试矩阵

| 测试 | 覆盖内容 |
|---|---|
| `internal/selfdesc` 单元测试 | 文档结构、版本、默认值 |
| secret 泄露测试 | 输入 token / DSN 后确认响应不包含 |
| HTTP `/meta` 测试 | 200、JSON、Content-Type、无认证 |
| 方法测试 | `POST /meta` 返回 405 |
| 路由一致性测试 | `/meta` 与 OpenAPI / 路由表一致 |
| limits 测试 | 配置改变后 metadata 跟随变化 |
| CLI describe 测试 | 普通输出、JSON、超时、404 |
| doctor 测试 | `/meta` 成功、缺失、版本不支持 |
| smoke | 运行中的 Compose 栈能读到 `/meta` |
| 回归测试 | 现有 health / ready / metrics / run 不受影响 |

---

## 10. 兼容性与风险

```text
旧服务端没有 /meta
  → 新 CLI describe 明确提示“服务端版本过旧”
  → 不静默降级

新服务端有 /meta
  → 旧客户端不受影响
  → 新客户端可以读取

meta_version = 1
  → 新增字段可接受
  → 删除/改名必须升级版本
```

风险：

| 风险 | 控制 |
|---|---|
| `/meta` 暴露 secret | 白名单字段 + 泄露测试 |
| 元数据与实际配置漂移 | limits 从真实配置构造 |
| 声明未实现能力 | 只声明 `normal`，不声明 sandbox/longterm/sessions |
| 旧客户端不兼容 | 新增字段只增不改 |
| docs 被误当公开契约 | 公开契约只认 `public/`；docs 不进入镜像/release |
| OpenAPI 与实际路由不一致 | 增加路由一致性测试 |

---

## 11. 完成标准

```text
[x] GET /meta 无需认证即可访问
[x] 返回 service / build / api / capabilities / limits / auth / endpoints
[x] 不包含任何 secret、密码、token、DSN
[x] public/service-meta/v1/schema.json 存在且可解析
[x] levier describe 可用
[x] levier describe --json 可被机器解析
[x] doctor 能检查 /meta
[x] OpenAPI 和相关契约测试通过
[x] Compose smoke 增加 meta: ok
[x] go test ./... 通过
[x] make lint 通过
[x] GitHub Actions CI 绿
[x] docs/ 未进入镜像、release 或 HTTP 暴露面
```

---

## 12. 需要确认的决策

1. `/meta` 不加认证，只暴露公开能力与协议
2. `/meta` 只做静态自描述，不做动态 daemon 列表
3. CLI 命令名使用 `levier describe`
4. 第一版只声明 `normal`、`react`、`echo`
5. 对外契约使用 `public/service-meta/v1/`，`docs/` 只作为内部开发文档
6. `docs/` 继续留在私有 Git 仓库，但不进入发布物；如果要求 Git 完全不跟踪 docs，需要单独做仓库结构调整
