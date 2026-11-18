<!-- Levier — CLI and distribution plan. -->

# 第 5 跳计划：CLI 与分发

> 当前命令参考见 [`CLI_REFERENCE.md`](CLI_REFERENCE.md)。
> 本文保留路线、设计约束和执行记录。

| 项目 | 内容 |
|---|---|
| 状态 | 5A-1/5A-2/5A-3a/5A-3b/5A-3c/5A-3d/5A-4 已完成；5B-1/5B-2/5B-3/5B-4 已完成；5B-5 未开始 |
| 目标 | 一个 `levier` 二进制既能服务，也能管理；用户不需要 git clone |
| 范围 | 5A CLI 操作面；5B 分发与安装面 |
| 不包含 | TUI、GUI、自动更新、brew/scoop/apt（后续） |

---

## 1. 5A：统一 CLI

### 1.1 目标

```sh
levier serve
levier version
levier doctor
levier status
```

保持兼容：

```sh
levier -config levier.example.yaml
```

### 1.2 命令面

已实现：

```text
levier version
levier serve
levier migrate status
levier migrate up
levier doctor
levier status
levier app init
levier app show
levier app rotate-key
levier run submit
levier run watch
levier run cancel
levier daemon check
```

计划中，当前未实现：

```text
levier config check
levier config print
levier tui
levier completion
levier upgrade
```

### 1.3 输出与退出码

```text
默认人类可读
--json 机器可读
不打印密钥
--quiet 为计划项，当前未实现
```

目标约定：

```text
0  成功
1  运行 / 检查失败
2  用法 / 配置错误
3  认证失败
4  网络 / 连接失败
5  状态不允许 / 确认拒绝
```

当前每个命令的实际退出码见 [`CLI_REFERENCE.md`](CLI_REFERENCE.md) 第 13 节。

### 1.4 配置优先级

```text
flag > 环境变量 > 配置文件 > 默认值
```

```text
LEVIER_CONFIG
LEVIER_SERVER
LEVIER_CREDENTIALS
LEVIER_ADMIN_TOKEN
```

### 1.5 5A 分步

```text
5A-1  CLI 骨架 + version + serve 兼容
5A-2  doctor + status
5A-3  app / run / migrate / daemon
5A-4  CLI 文档收敛
```

5A-1 验收：

```sh
levier version
levier serve -config levier.example.yaml
levier -config levier.example.yaml
```

### 1.6 5A-4 文档收敛

目标：把分散在 README、设计稿、执行记录里的 CLI 信息收敛成一份用户可用的命令参考。

产物：

```text
docs/CLI_REFERENCE.md   唯一用户参考：命令、参数、环境变量、JSON、退出码
docs/README.md          文档索引
README.md               快速入口链接
CLI_ACTIONS.md          保留设计与执行记录，顶部指向参考
CLI_DISTRIBUTION_PLAN.md 保留路线与验收，顶部指向参考
```

同步修正：

```text
所有子命令 --help 输出到 stdout，退出 0
用法错误继续退出 2
未实现的 config check/print、--quiet 明确标为计划项
每个命令的退出码按实际实现记录，不套用单一全局表
```

验收：

```text
文档中的命令与 --help 一致
文档中的默认值与代码一致
文档中的 JSON 字段与代码一致
示例不包含真实密钥
make lint / structure-check / go test ./... 通过
```

---

## 2. 5B：分发与安装

### 2.1 目标

干净机器上，不 `git clone`：

```sh
docker run ghcr.io/<org>/levier:v0.1.0
curl -fsSL .../install.sh | sh
levier version
go install github.com/<org>/levier/cmd/levier@v0.1.0
```

### 2.2 渠道

| 渠道 | 用户命令 | 优先级 |
|---|---|---|
| 容器镜像 | `docker run` / Compose | P0 |
| Release 二进制 + install.sh | `curl ... \| sh` | P0 |
| `go install` | `go install ...@v0.1.0` | P1 |
| Homebrew / Scoop / apt | `brew install ...` | P2 |

服务端主路径是容器；CLI 主路径是 release 二进制。

### 2.3 版本与构建信息

```text
SemVer：v0.1.0
git tag：v0.1.0
ldflags：version / commit / build time
```

`levier version`：

```text
levier v0.1.0
commit: abc1234
built: 2026-11-15T...
go: go1.26
platform: linux/amd64
```

### 2.4 Release Workflow

新增 `.github/workflows/release.yml`，tag `v*` 触发：

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
```

产物：

```text
levier_v0.1.0_linux_amd64.tar.gz
levier_v0.1.0_darwin_arm64.tar.gz
levier_v0.1.0_windows_amd64.zip
checksums.txt
```

#### 5B-1 实施范围

共享构建脚本：

```text
scripts/build-release.sh
```

维护者流程见 [`RELEASE.md`](RELEASE.md)。

职责：

```text
读取 VERSION / COMMIT / BUILT
交叉编译 5 个目标
生成 tar.gz / zip
生成 checksums.txt
输出到 dist/
```

本地入口：

```sh
make release-dist
```

CI 发布：

```text
release workflow -> verify
                 -> build-release.sh
                 -> gh release create/upload
```

验收：

```text
dist/ 下 5 个压缩包 + checksums.txt
checksums.txt 可用 sha256sum -c 校验
linux/amd64 产物能运行 levier version
windows/amd64 产物能运行 levier.exe version
release workflow 的 tag 版本写入 ldflags
```

### 2.5 install.sh

```text
识别 OS / ARCH
解析版本（默认 latest）
下载 tar.gz / zip
校验 sha256
安装到 /usr/local/bin 或 ~/.local/bin
打印 PATH 提示
```

参数：

```text
--version
--prefix
--dry-run
```

用户安装说明见 [`INSTALL.md`](INSTALL.md)。

不自动修改 shell 配置，不跳过 checksum。Windows 另提供 `install.ps1`。

#### 5B-2 实施范围

新增：

```text
scripts/install.sh
scripts/install.ps1
docs/INSTALL.md（初版，5B-5 再补完整支持矩阵）
```

共同规则：

```text
默认仓库 fluffyTighnari/levier
默认 latest release
支持固定 --version
下载 checksums.txt 后校验目标归档
校验失败立即退出
不自动修改 shell / 用户 PATH
支持 --dry-run
支持 --prefix / -Prefix
支持 --base-url / -BaseUrl 用于镜像与本地验证
```

本地验收：

```text
本地 HTTP 服务提供 dist/ 产物
install.sh 安装到临时目录并运行 levier version
install.ps1 安装到临时目录并运行 levier.exe version
checksum 错误时安装失败
```

### 2.6 容器发布

```text
ghcr.io/<org>/levier:v0.1.0
ghcr.io/<org>/levier:latest
ghcr.io/<org>/levier-harness:v0.1.0
ghcr.io/<org>/levier-mcp-runtime:v0.1.0
ghcr.io/<org>/levier-agent:v0.1.0
```

先 linux/amd64，再补 arm64。

#### 5B-3 实施范围

在 release workflow 增加 `images` job：

```text
needs: verify
permissions:
  contents: read
  packages: write
```

发布镜像：

```text
levier
levier-harness
levier-mcp-runtime
levier-agent
```

规则：

```text
平台先只发布 linux/amd64
tag 使用 v0.1.0
稳定版本额外更新 latest
预发布 tag 不更新 latest
levier 镜像注入 VERSION / COMMIT / BUILD_TIME
使用 docker/metadata-action 生成 OCI labels
使用 GITHUB_TOKEN 登录 ghcr.io
```

本地验收：

```text
docker compose build 四个镜像
YAML 解析 release workflow
不实际 push（本地无 GHCR 凭据）
```

### 2.7 go install

前提：

```text
仓库公开
module path = github.com/<org>/levier
打了 v0.1.0 tag
```

私有仓库需要内部 Go proxy 或只走 release 二进制。

#### 5B-4 实施范围

不发布正式 `v0.1.0`，使用现有 prerelease tag 做验证。

验证命令：

```sh
GOPRIVATE=github.com/fluffyTighnari/*
GONOSUMDB=github.com/fluffyTighnari/*
go install github.com/fluffyTighnari/levier/cmd/levier@<tag>
```

验收：

```text
干净 GOBIN / GOMODCACHE
安装后的 levier version 正常
输出版本与 tag 一致
平台为当前机器
```

限制：

```text
仓库私有，公开匿名 go install 暂时无法验证
需要临时 Git/Go 认证或内部 proxy
仓库公开后再补一次无认证验证
```

文档：

```text
docs/INSTALL.md 增加 go install 章节
docs/RELEASE.md 记录验证结果
```

### 2.8 文档与安全

```text
docs/INSTALL.md    5B-2 初版；5B-5 补支持矩阵、升级、卸载、离线
支持矩阵
容器 / install.sh / go install
升级 / 卸载 / 离线
checksum 校验
```

```text
发布 sha256 checksums
生产环境不要无校验 curl | sh
后续可加 cosign / sigstore
镜像使用不可变 tag
```

---

## 3. 执行顺序

```text
5A-1  CLI 骨架 + version + serve
5A-2  doctor + status
5A-3  app / run / migrate / daemon
5A-4  CLI 文档收敛
5B-1  release workflow + checksums
5B-2  install.sh / install.ps1
5B-3  GHCR 镜像
5B-4  go install 验证
5B-5  INSTALL 文档
```

## 4. 总验收

```text
[x] levier version 输出版本/commit/平台
[x] levier -config 兼容旧启动方式
[x] levier serve 可用
[x] levier doctor 能逐跳报错
[x] levier status --json 可用
[x] levier app init 幂等
[x] levier run submit --watch 跑到 completed
[x] CLI 命令参考收敛到 docs/CLI_REFERENCE.md
[x] release workflow + 本地 5 平台产物/checksums
[x] install.sh / install.ps1 本地安装与 checksum 校验
[x] GHCR image workflow + 本地四镜像构建
[x] go install @v0.1.0-rc.5（私有认证）
[ ] git tag v0.1.0 触发 release
[ ] Release 有 5 个平台产物 + checksums
[ ] install.sh 在干净机器安装成功
[ ] docker run GHCR 镜像 /healthz 200
[ ] go install @v0.1.0 成功
[ ] 不 git clone 也能完成安装和启动
```

---

## 5. 执行记录

### 2026-11-15：5A-1 完成

新增：

```text
internal/version/version.go
cmd/levier/cli.go
cmd/levier/cli_test.go
```

行为：

```text
levier version [--json]
levier serve [-config path]
levier [-config path]   兼容旧启动方式
levier help
```

构建信息：

```text
Makefile: VERSION / COMMIT / BUILD_TIME -> ldflags
deploy/levier.Dockerfile: 同名 build args；PATH 包含 /app
docker-compose.yml: 可选 build args
```

验收：

```text
go test ./...                           PASS
go build ./...                          PASS
make build                              PASS
levier version                          PASS
levier version --json                   PASS
ldflags 注入 version/commit/built        PASS
unknown command 返回用法                 PASS
```

### 2026-11-16：5A-2 完成

新增：

```text
cmd/levier/clihttp.go
cmd/levier/status.go
cmd/levier/doctor.go
cmd/levier/status_test.go
cmd/levier/doctor_test.go
```

`levier status`：

```text
/healthz
/readyz
/metrics
daemon 数量与容量
App WebSocket 数量
Run 状态计数
--json
```

`levier doctor`：

```text
配置解析
数据库连接
迁移版本
/healthz
/readyz
admin auth（GET /admin/mcp）
App 凭据（可选 --credentials）
--json
```

验收：

```text
go test ./...                                  PASS
go build ./...                                 PASS
levier status（宿主）                           PASS
levier status --json（宿主）                    PASS
docker compose exec levier levier doctor        PASS
docker compose exec levier levier status        PASS
```

### 2026-11-16：5A-3a 完成（app 命令）

新增：

```text
cmd/levier/apiclient.go
cmd/levier/credentials.go
cmd/levier/confirm.go
cmd/levier/app.go
cmd/levier/credentials_test.go
cmd/levier/app_test.go
docs/CLI_ACTIONS.md
```

命令：

```text
levier app init --name <app> [--credentials-out path] [--rotate] [--force] [--dry-run]
levier app show --credentials path
levier app rotate-key --credentials path --yes
```

安全行为：

```text
凭据 0600
原子写入
已存在默认拒绝覆盖
--force 覆盖前生成 .bak
不打印 key
非 TTY 缺少 --yes 拒绝 rotate
```

验收：

```text
go test ./cmd/levier                          PASS
levier app init（真实容器）                   PASS
levier app show（真实容器）                   PASS
levier app rotate-key --yes（真实容器）        PASS
rotate 后凭据文件与 .bak 存在                  PASS
```

### 2026-11-16：5A-3b 完成（run 命令）

新增：

```text
cmd/levier/run.go
cmd/levier/run_test.go
```

命令：

```text
levier run submit --prompt ... [--watch] [--json]
levier run watch --run-id ... [--json]
levier run cancel --run-id ... --yes
```

验收：

```text
go test ./cmd/levier                          PASS
run submit echo（真实容器）                    PASS
run watch（真实容器）                          PASS
run submit --watch --json（真实容器）           PASS
```

### 2026-11-17：5A-3c 完成（migrate 命令）

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

验收：

```text
go test ./cmd/levier / internal/db             PASS
migrate status（真实容器）                     PASS
migrate up（真实容器）                         PASS
```

### 2026-11-17：启动迁移策略统一

```text
levier serve
  连接数据库
  检查 pending migration
  pending > 0 -> 报错退出 1
  不自动执行 DDL

deploy/levier-entrypoint.sh
  /app/levier migrate up -config "$CONFIG"
  exec /app/levier serve -config "$CONFIG"
```

验证：

```text
go test ./...                                  PASS
docker compose build levier                    PASS
docker compose smoke                           PASS
空库 + levier serve -> pending 9，退出 1       PASS
```

### 2026-11-17：5A-3d 完成（daemon check）

新增：

```text
internal/daemoncheck/daemoncheck.go
cmd/levier/daemon.go
cmd/levier/daemon_test.go
cmd/mw-daemon-check/main.go   兼容入口，改为薄封装
```

命令：

```text
levier daemon check [--grpc addr] [--server url] [--daemon-id id] [--json]
```

行为：

```text
复用 daemon 协议一致性检查
创建临时 App / Run
检查 hello / task_assign / ack / completed / cancel / cancelled
--json 输出 {ok, checks, error}
退出码：0 成功；1 检查失败；2 用法错误；3 认证失败；4 网络失败；5 daemon id 冲突
```

验收：

```text
go test ./cmd/levier / internal/daemoncheck      PASS
daemon check --help                             PASS
daemon check 未知参数                           PASS
daemon check 错误 Admin token（退出 3）          PASS
daemon check 不可达 server（退出 4）             PASS
daemon check 重复 daemon id（退出 5）            PASS
真实 stack 协议一致性检查                       PASS
```

操作注意：

```text
检查使用预分配的 daemon id/token。
若同一 daemon id 已有真实 Harness 连接，服务端会拒绝重复连接。
本次验证临时停止 harness 释放 daemon-local-1，检查后已恢复。
```

### 2026-11-17：5A-4 完成（CLI 文档收敛）

新增：

```text
docs/CLI_REFERENCE.md
cmd/levier/flags.go
```

变更：

```text
CLI_REFERENCE.md 成为唯一用户参考
所有子命令 --help -> stdout，退出 0
未知参数 -> stderr，退出 2
--help 不打印 token 环境变量默认值
doctor 检查 pending migration
文档索引、README、MAP 指向 CLI_REFERENCE.md
CLI 设计/路线文档顶部指向用户参考
未实现的 config check/print、--quiet 标为计划项
```

验收：

```text
go test ./cmd/levier                            PASS
所有子命令 --help                               PASS（退出 0）
doctor --help                                   PASS
CLI_REFERENCE.md 与实现核对                     PASS
doctor 正常 stack                               PASS（7/7，退出 0）
doctor pending migration                        PASS（migrations fail，退出 1）
make lint                                       PASS
make structure-check                            PASS
```

### 2026-11-17：5B-1 完成（release workflow + checksums）

新增：

```text
scripts/build-release.sh
.github/workflows/release.yml
docs/RELEASE.md
Makefile release-dist
```

行为：

```text
tag v* 触发 release workflow
verify: make lint / make deps-check / make test
交叉编译 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64
生成 tar.gz / zip + checksums.txt
gh release create 或 gh release upload --clobber
prerelease tag（含 -）自动标记
```

本地验收：

```text
VERSION=v0.1.0-test sh scripts/build-release.sh   PASS
5 个压缩包 + checksums.txt                        PASS
sha256sum -c checksums.txt                         PASS
linux/amd64 levier version                         PASS
windows/amd64 levier.exe version                   PASS
Makefile release-dist                              PASS
```

产物命名：

```text
levier_v0.1.0-test_linux_amd64.tar.gz
levier_v0.1.0-test_linux_arm64.tar.gz
levier_v0.1.0-test_darwin_amd64.tar.gz
levier_v0.1.0-test_darwin_arm64.tar.gz
levier_v0.1.0-test_windows_amd64.zip
checksums.txt
```

### 2026-11-17：5B-2 完成（install.sh / install.ps1）

新增：

```text
scripts/install.sh
scripts/install.ps1
docs/INSTALL.md
```

行为：

```text
默认仓库 fluffyTighnari/levier
默认 latest release；支持固定 --version / -Version
下载 checksums.txt 并校验目标归档
校验失败立即退出
安装到 /usr/local/bin、~/.local/bin 或指定 prefix
不自动修改 shell / 用户 PATH
支持 --dry-run / -DryRun
支持 --base-url / -BaseUrl 用于镜像和本地验证
```

退出码：

```text
0  安装或 dry-run 成功
1  下载 / 校验 / 解压 / 写入失败
2  参数错误、平台不支持
```

本地验收：

```text
本地 HTTP 服务提供 dist/ 产物
install.sh 安装到 /tmp 并运行 levier version          PASS
install.ps1 安装到临时目录并运行 levier.exe version    PASS
install.sh --dry-run                                   PASS
install.sh --help                                      PASS（0）
install.sh 未知参数                                    PASS（2）
install.sh 错误 checksum                               PASS（1）
install.ps1 -DryRun                                    PASS
install.ps1 -Help                                      PASS（0）
install.ps1 -BaseUrl 缺少 -Version                     PASS（2）
install.ps1 错误 checksum                              PASS（1）
```

### 2026-11-17：5B-3 完成（GHCR 镜像）

变更：

```text
.github/workflows/release.yml  增加 images job
.dockerignore                  排除 .qta / .stfolder / .stignore / .DS_Store
docs/INSTALL.md                增加容器镜像章节
docs/RELEASE.md                增加 GHCR 镜像章节
```

镜像：

```text
ghcr.io/<owner>/levier:v0.1.0
ghcr.io/<owner>/levier-harness:v0.1.0
ghcr.io/<owner>/levier-mcp-runtime:v0.1.0
ghcr.io/<owner>/levier-agent:v0.1.0
```

规则：

```text
needs: verify
packages: write
GITHUB_TOKEN 登录 ghcr.io
平台 linux/amd64
稳定 tag 更新 latest
预发布 tag 不更新 latest
levier 镜像注入 VERSION / COMMIT / BUILD_TIME
```

本地验收：

```text
docker compose build levier mcp-runtime harness agent    PASS
levier 镜像 /app/levier version                           PASS
harness 镜像 --help                                       PASS
mcp-runtime 镜像 --help                                   PASS
release.yml YAML 解析                                     PASS
```

未执行：

```text
实际 GHCR push（本地无发布凭据）
```

### 2026-11-18：5B-4 完成（go install 验证）

新增：

```text
internal/version/version.go        debug.BuildInfo fallback
internal/version/version_test.go
docs/INSTALL.md                    go install 章节
.github/workflows/release.yml      public-only go-install job
```

验证方式：

```text
私有仓库 + 临时 Git URL rewrite
干净 GOPATH / GOMODCACHE / GOBIN
GOPRIVATE=github.com/fluffyTighnari/*
使用 v0.1.0-rc.5 tag
```

结果：

```text
go install github.com/fluffyTighnari/levier/cmd/levier@v0.1.0-rc.5  PASS
安装后 levier version -> v0.1.0-rc.5                              PASS
platform -> windows/amd64                                          PASS
```

说明：

```text
go install 不传 ldflags
新增 debug.BuildInfo fallback 后版本号正确
commit / built 在 go install 模式下仍可能为 unknown
公开匿名 go install 仍需仓库公开后补测
```

补充修复：

```text
tag 构建时 CI 需要本地 main 分支供 buf breaking 使用
main 构建时不能强制更新已检出的 main 分支
```
