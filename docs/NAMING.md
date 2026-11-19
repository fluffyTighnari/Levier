<!-- Levier — naming and identifier plan (hop 1). -->

# 命名与标识落地（第 1 跳）

| 项目 | 内容 |
|---|---|
| 状态 | 命名 1A–1D 完成；cutover 已执行（fresh DB）；目录归位与结构护栏完成 |
| 产品名 | Levier |
| 1A 结论 | 可用（软冲突），无同领域硬冲突 |
| 前置 | `docs/SCOPE.md` 已封板 |
| 本文 | 定义标识符、改名顺序、兼容策略；本文不执行改名 |

---

## 1. 名字

```text
产品名      Levier
含义        法语“杠杆”；控制点、放大能力
发音        /lə.vje/，中文近似“勒维耶”
英文近似    luh-VYAY
```

选择理由：

```text
不是类别词，可检索
6 个字母，无重音，适合做二进制名
可做环境变量前缀 LEVIER_
可做 Go module、Python 包、Docker 镜像名
语义贴合控制面：杠杆、支点、放大
```

---

## 2. 可用性检查

执行第 1B 之前，先做一次 30 分钟的检查，结果记录在本文末尾。

```text
GitHub 搜索：levier agent runtime / levier orchestration / levier control plane
PyPI：levier
npm：levier
Docker Hub：levier
域名：levier.dev / levier.io / getlevier.dev
搜索引擎：Levier AI / Levier software
商标：如果要做商业产品，查目标市场的同名商标
```

判定标准：

```text
软冲突：普通词、非同一领域、无活跃项目 → 可以用
硬冲突：同领域活跃项目、同名商业产品、商标冲突 → 不直接用，加限定词或换名
普通撞名（姓氏、地名、金融公司）不算硬冲突
```

硬冲突时不要静默改名；记录在本文，重新评审。

---

## 3. 标识符地图

| 类别 | 当前 | 目标 | 时机 | 兼容策略 |
|---|---|---|---|---|
| Go module | `github.com/fluffyTighnari/middleware` | `github.com/<org>/levier` | 1B | 无别名，机械替换 |
| 服务二进制 | `bin/middleware` | `bin/levier` | 1B | 保留 `middleware` 包装脚本一个版本 |
| 管理 CLI | `mwctl`（计划） | `levier` 子命令 | 1B | 不单独做 `mwctl` |
| 配置环境变量 | `MIDDLEWARE_CONFIG` | `LEVIER_CONFIG` | 1B | 旧名兼容 |
| 产品环境变量 | `MIDDLEWARE_*` | `LEVIER_*` | 1B | 旧名兼容一个版本 |
| 示例环境变量 | `MW_URL` / `MW_ADMIN_TOKEN` / `MW_APP_NAME` | `LEVIER_URL` / `LEVIER_ADMIN_TOKEN` / `LEVIER_APP_NAME` | 1B | 旧名兼容一个版本 |
| Harness 环境变量 | `HARNESS_*` | `HARNESS_*` | 不变 | 组件级前缀，保留 |
| Python 发行名 | `middleware-harness` 等 | `levier-harness` 等 | 1C | 新包名，旧包名保留一个版本 |
| Python import | `middleware_harness` 等 | `levier_react_runtime` 等 | 1C | 旧包名作为 shim |
| Docker 镜像 | `middleware-harness` 等 | `levier/harness` 等 | 1B | 旧镜像名保留一个版本 |
| Dockerfile | `middleware.Dockerfile` | `levier.Dockerfile` | 1B | 旧文件名保留一个版本 |
| 配置文件名 | `config.example.yaml` | `levier.example.yaml` | 1B | 旧文件保留一个版本 |
| Compose 项目 | `middleware-mvp` | `levier-mvp` | 1D | 有状态，单独迁移 |
| Compose 服务名 | `middleware` | `levier` | 1D | 有状态，单独迁移 |
| 数据库名/用户 | `middleware` | `levier` | 1D | 有状态，单独迁移 |
| 指标前缀 | `mw_` | `levier_` | 1D | 仪表盘迁移 |
| 文档与 README | `Middleware` | `Levier` | 1B | 全量替换 |

---

## 4. 改名分四批

### 1A：检查与冻结

```text
完成可用性检查
确认 Go module 所属 org
确认“一个二进制”还是“服务端 + 管理端两个二进制”
把结果写进本文
```

### 1B：无状态改名（第一批 PR）

目标：改身份，不改数据、不改网络拓扑。

```text
go.mod module path
proto 的 go_package option，重新生成 gen/
cmd/middleware → cmd/levier
Makefile 的构建产物
MIDDLEWARE_* / MW_* 环境变量（保留旧名兼容）
Dockerfile 文件名与镜像名
config.example.yaml → levier.example.yaml
README、docs、examples、CI 中的产品名
```

本批**不动**：

```text
Compose 项目名
Compose 服务名 middleware
数据库名与用户
指标前缀 mw_
Python import 包名
```

验收：

```sh
make lint
make build
make test
make react-runtime-test
make mcp-runtime-test
make agent-core-test
docker compose --profile smoke run --rm smoke
```

### 1C：Python 包名（第二批 PR）

目标：把发行名和 import 名迁到 `levier_*`，但不破坏现有代码。

```text
middleware-harness      → levier-harness
middleware-agent-core   → levier-agent-core
middleware-mcp-runtime  → levier-mcp-runtime

import middleware_harness      → levier_react_runtime
import agent_core              → levier_agent_core
import mcp_runtime             → levier_mcp_runtime
```

兼容方式：

```text
旧 import 包保留为 shim，内部 re-export 新包
旧发行名保留一个版本
uv.lock 重新生成
Dockerfile 使用新发行名
```

验收：

```sh
cd harness && uv sync --locked
make react-runtime-test
make mcp-runtime-test
make agent-core-test
make docker-smoke
```

### 1D：有状态改名（第三批 PR）

目标：迁移项目名、服务名、数据库名、指标前缀。  
这一批会影响数据卷和仪表盘，必须单独评审。

```text
Compose 项目 middleware-mvp → levier-mvp
Compose 服务 middleware → levier
数据库 middleware → levier
数据库用户 middleware → levier
指标前缀 mw_ → levier_
```

迁移要求：

```text
先备份 PostgreSQL
新项目名使用新卷，或显式迁移旧卷
数据库用 dump/restore，不直接改名
仪表盘与告警规则同步更新
保留一个版本的旧指标名，或明确公告 breaking change
```

验收：

```sh
docker compose -p levier-mvp up --build -d
docker compose -p levier-mvp --profile smoke run --rm smoke
curl -s localhost:8080/metrics | grep levier_
```

### 1E：清理与护栏

```text
删除兼容 shim
删除旧环境变量别名
删除旧镜像名与旧 Dockerfile
加 naming 检查脚本
```

`tests/checks/check_naming.sh` 检查：

```text
禁止 github.com/.../middleware
禁止新增 MIDDLEWARE_ / MW_ 前缀
禁止新代码 import 旧 Python 包
禁止文档引用旧二进制名
旧标识符只允许出现在兼容层和迁移文档
```

---

## 5. 兼容策略

环境变量：

```text
新名优先
旧名仍可读
同时设置时新名生效
启动日志提示旧名已弃用
一个版本后删除旧名
```

Python import：

```text
旧包保留为 shim
新代码只 import 新包
shim 不包含业务逻辑
一个版本后删除
```

二进制：

```text
bin/levier 为主
bin/middleware 为包装脚本，打印弃用提示
一个版本后删除
```

---

## 6. 不做的事

```text
不改 Go internal 包名
不改协议字段名
不改数据库表名
不改错误码
不改事件类型
不在同一个 PR 里做“改名 + 重构”
不在没有备份的情况下改数据库名
```

---

## 7. 决策待确认

```text
[ ] Go module 所属 org：github.com/fluffyTighnari/levier？
[ ] 一个二进制 levier，还是 levierd（服务端）+ levier（管理端）？
[ ] 1D 有状态改名现在就做，还是等第一次正式发布前做？
[ ] 指标前缀是否确定改为 levier_？
[ ] Python import 包名是否在 1C 就改，还是等 1.0 前？
```

推荐默认：

```text
module：github.com/fluffyTighnari/levier
二进制：一个 levier，默认 levier serve
1D：等第一次正式发布前做
指标：1D 改为 levier_
Python import：1C 改，带 shim
```

---

## 8. 可用性检查记录

```text
检查日期：2026-11-15
GitHub：q=levier in:name 有同名仓库和用户，但没有同领域的活跃
        agent runtime / control plane 项目；最近的是 Levier-IA
        （法国 AI 培训相关组织）和若干 0 star 同名仓库
PyPI：404，可用
npm：404，可用
Docker Hub：本地网络无法访问 API，需手动复查
域名：levier.dev       NXDOMAIN，可能可用
      getlevier.dev    NXDOMAIN，可能可用
      levier.io        已被注册，当前是 parked lander，无活跃产品
商标：未查，商业发布前需人工/法务复查
结论：可用（软冲突）
备注：没有发现同领域硬冲突；Docker Hub 与商标待复查
```

---

## 9. 执行记录

### 2026-11-15：1B-1 完成

```text
go.mod module path        github.com/fluffyTighnari/levier
proto go_package          已更新并重新生成 Go / Python
cmd/middleware            → cmd/levier
构建产物                  bin/levier
Dockerfile                构建 /out/levier，容器内 /app/levier
entrypoint                启动 /app/levier
e2e 默认路径               bin/levier / ./cmd/levier
文档中的路径引用           已更新
```

本批未改：

```text
环境变量 MIDDLEWARE_* / MW_*
Compose 项目名与服务名
数据库名与用户
指标前缀 mw_
Python 发行名与 import 包名
Dockerfile / entrypoint 文件名
```

验收：

```sh
go build ./...          PASS
go test ./...           PASS
make build              PASS（生成 bin/levier）
go vet ./...            PASS
buf lint / buf build    PASS
buf generate            幂等（重复生成无额外 diff）
secret scan             PASS
```

未完成验证：

```text
docker compose build middleware
  容器的 go mod download 访问 proxy.golang.org 被当前网络拒绝；
  这是环境问题，不是代码问题，需在有外网/代理的机器上复验。

make react-runtime-test
  Windows 的 PYTHONPATH 分隔符与 Makefile 的冒号写法不兼容；
  已用正确 Windows 路径验证 middleware.runtime.v1 可以导入。
```

备注：Windows 下 `make lint` 的 `find` 检查不可靠，CI 的 Ubuntu 环境为准。

### 2026-11-15：1B-2a 完成

服务端环境变量已切到 `LEVIER_*`，旧名保留一个版本：

```text
LEVIER_HTTP_* / LEVIER_GRPC_* / LEVIER_DATABASE_URL
LEVIER_LOG_* / LEVIER_WORKFLOW_*
LEVIER_CONFIG
LEVIER_SECRET_<NAME>
```

兼容行为：

```text
新名优先
新名缺失时回退 MIDDLEWARE_*
secret resolver 同时接受 MIDDLEWARE_SECRET_<NAME>
```

同时更新：

```text
docker-compose.yml 中 middleware / mcp-runtime 的注入变量
deploy/middleware-entrypoint.sh 的配置路径
internal/config 测试（新名、旧名回退、新名优先）
mcp_runtime secret 测试
```

本批未改：

```text
App / 示例的 MW_URL、MW_ADMIN_TOKEN、MW_APP_NAME
MIDDLEWARE_BIN、MIDDLEWARE_CONFIG、MIDDLEWARE_TEST_* 等脚本/测试变量
.env.example
Doc 中的旧变量说明
```

验收：

```sh
go test ./...                          PASS
go build ./...                         PASS
go vet ./...                           PASS
mcp_runtime test_secrets               PASS
```

### 2026-11-15：1B-2b 完成

App / 示例 / 脚本 / 测试环境变量已切到 `LEVIER_*`，旧名保留：

```text
LEVIER_URL / LEVIER_ADMIN_TOKEN / LEVIER_APP_NAME
LEVIER_BIN / LEVIER_CONFIG
LEVIER_TEST_POSTGRES / LEVIER_TEST_DATABASE_URL
LEVIER_DATABASE_URL
```

兼容行为：

```text
新名优先
新名缺失时回退 MW_* / MIDDLEWARE_*
examples、deploy、tests/e2e 都支持两套名字
.env.example 以 LEVIER_* 为主，旧名以注释保留
```

同时更新：

```text
examples/counter_agent 的 SDK App 与测试
examples/http_client、node_counter_agent
deploy/agent-entrypoint.sh、deploy/smoke.py
tests/e2e 三个脚本与 fault_matrix.py
Go 集成测试环境变量 helper
README、MVP、APP_ONBOARDING、DESIGN、MCP、deploy 文档
```

本批未改：

```text
HARNESS_MIDDLEWARE_ADDRESS / HARNESS_MIDDLEWARE_TLS
  属于 Harness 配置节命名，放到 1B-3 与 config 文件名一起处理。
Dockerfile / entrypoint 文件名
Compose 项目名与服务名
数据库名与用户
指标前缀 mw_
Python 发行名与 import 包名
```

验收：

```text
go test ./...                          PASS
go build ./...                         PASS
go vet ./...                           PASS
mcp_runtime secrets test               PASS
counter_agent SDK 测试                 PASS
docker compose config                  PASS
```

### 2026-11-15：1B-3 完成（文件级）

文件名与 Harness 环境变量已切换：

```text
deploy/middleware.Dockerfile          → deploy/levier.Dockerfile
deploy/middleware-entrypoint.sh       → deploy/levier-entrypoint.sh
config.example.yaml                   → levier.example.yaml
harness/config.example.yaml           → harness/levier.example.yaml
HARNESS_MIDDLEWARE_ADDRESS            → HARNESS_LEVIER_ADDRESS（旧名兼容）
HARNESS_MIDDLEWARE_TLS                → HARNESS_LEVIER_TLS（旧名兼容）
```

同时更新：

```text
docker-compose.yml 的 Dockerfile 路径与 Harness 地址变量
Makefile、cmd/levier、cmd/migrate 的默认配置路径
deploy/levier.Dockerfile、deploy/runtime.Dockerfile
tests/e2e 的配置默认路径
README、deploy、MCP、harness 文档
```

本批未改（留给 1C / 1D）：

```text
Harness 配置节 middleware:（仅文件名与 env 改了）
Compose 项目名 middleware-mvp
Compose 服务名 middleware
数据库名/用户 middleware
指标前缀 mw_
Python 发行名与 import 包名
```

验收：

```text
go build ./...                         PASS
go test ./...                          PASS
harness test_config.py                 PASS
docker compose config                  PASS
```

### 2026-11-15：1C 完成（Python 包名）

发行名：

```text
middleware-agent-core  → levier-agent-core
middleware-harness     → levier-harness
middleware-mcp-runtime → levier-mcp-runtime
```

import 包：

```text
agent_core        → levier_agent_core
middleware_harness → levier_react_runtime
mcp_runtime       → levier_mcp_runtime
```

旧包保留 shim：

```text
agent_core/src/agent_core/__init__.py
harness/src/middleware_harness/__init__.py
mcp_runtime/src/mcp_runtime/__init__.py
```

同时更新：

```text
三个 pyproject.toml
harness/uv.lock
deploy/runtime.Dockerfile
deploy/mcp-runtime.Dockerfile
Makefile、tests/e2e、harness/mcp_runtime 测试
README、AGENT_CORE_CONTRACT、MCP、deploy 文档
```

本批未改：

```text
proto 包名 middleware.runtime.v1
Harness 配置节 middleware:
mcp_runtime 配置节
目录名 agent_core/、harness/、mcp_runtime/
Compose 项目名/服务名、数据库名、指标前缀
```

验收：

```text
agent_core tests                      PASS
harness 配置/IDs/日志/健康测试          PASS
mcp_runtime secrets 测试               PASS
counter_agent SDK 测试                 PASS
新旧包 import 与 submodule import       PASS
go build / go test                     PASS
docker compose config                  PASS
```

注意：本机 `uv lock` 因配置的 PyPI 镜像返回 403 未能重新生成；`harness/uv.lock` 只做了包名机械替换。  
在有可用索引的机器上需要再跑一次 `cd harness && uv lock` 确认 lockfile。

### 2026-11-15：1D 完成（有状态标识符）

配置与代码已切到新名字：

```text
Compose 项目     middleware-mvp            → levier-mvp
Compose 服务     middleware                → levier
PostgreSQL 用户  middleware                → levier
PostgreSQL 库    middleware                → levier
测试库           middleware_test           → levier_test
MCP socket       /run/middleware/mcp.sock  → /run/levier/mcp.sock
本地 MCP socket  /tmp/middleware-mcp.sock  → /tmp/levier-mcp.sock
指标前缀         mw_                       → levier_
```

同时更新：

```text
docker-compose.yml
.env.example
levier.example.yaml
harness/levier.example.yaml
deploy/agent-entrypoint.sh、deploy/smoke.py、deploy/mcp-runtime.Dockerfile
tests/e2e/mcp_e2e.sh、retrofit_e2e.sh
README、DESIGN、MCP、deploy、harness、mcp_runtime 文档
新增 docs/MIGRATION-1D.md
```

**cutover 已执行**（2026-11-15，fresh DB 方案）：旧 stack 已停止并移除容器，旧卷保留；新 stack `levier-mvp` 已启动，smoke PASS。  
备份与恢复步骤见 `docs/MIGRATION-1D.md`。

本批未改：

```text
proto 包名 middleware.runtime.v1
Harness 配置节 middleware:
mcp_runtime 配置节
数据库内部表名/序列名中的 mw_
```

验收：

```text
docker compose config      PASS（name: levier-mvp）
go test ./...              PASS
python -m compileall       PASS
指标测试                    PASS
```

### 2026-11-15：第 3 跳完成（目录归位）

执行：

```text
git mv agent_core  execution/agent_core
git mv harness     execution/react_runtime
git mv mcp_runtime execution/mcp_runtime
```

保持不变：

```text
Python 包名、Go module、proto
容器内路径 /app/agent_core、/app/harness、/app/mcp_runtime
Compose / 数据库 / 指标
```

同时更新：

```text
.github/workflows/ci.yml
Makefile
buf.gen.yaml
deploy/runtime.Dockerfile、deploy/mcp-runtime.Dockerfile
tests/e2e、tests/checks、tests/contract
docs/MAP.md、SCOPE.md、DESIGN.md、ARCHITECTURE.md、CONTRACT.md、MCP.md、README.md
execution/*/README.md
harness 默认配置路径改为包相对路径
```

验收：

```text
go build ./...                         PASS
go test ./...                          PASS
agent_core tests                       PASS
harness 配置/IDs/日志/健康测试          PASS
mcp_runtime secrets 测试               PASS
buf lint / buf generate                PASS
docker compose build levier harness mcp-runtime  PASS
docker compose --profile smoke run --rm smoke     PASS
```

### 2026-11-15：第 4 跳完成（结构护栏）

新增：

```text
tests/checks/check_structure.sh
tests/checks/check_structure.py
make structure-check
```

检查：

```text
顶层目录必须存在，旧顶层目录不能复活
每个顶层单元必须有 README.md
internal / cmd / execution 不能引用 examples/
agent_core 不能 import harness / MCP / protobuf
cmd/* 只放 package main
gen/*.go 必须标记为生成代码
operational 文件不能引用搬迁前的旧路径
```

接入：

```text
make lint 包含 structure-check
CI: make lint + make deps-check
docs/MAP.md、CODE_STYLE.md、tests/README.md 已同步
```

验收：

```text
make structure-check                   PASS
make lint                              PASS
负向测试：agent_core import harness     FAIL（符合预期）
负向测试：根目录出现 harness/           FAIL（符合预期）
```
