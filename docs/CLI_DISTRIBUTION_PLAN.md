<!-- Levier — CLI and distribution plan. -->

# 第 5 跳计划：CLI 与分发

| 项目 | 内容 |
|---|---|
| 状态 | 5A-1/5A-2/5A-3a 已完成；5A-3b/c/d、5B 未开始 |
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

```text
levier version
levier serve
levier config check
levier config print
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

先不做：

```text
levier tui
levier completion
levier upgrade
```

### 1.3 输出与退出码

```text
默认人类可读
--json 机器可读
--quiet 只输出结果
不打印密钥
```

```text
0  成功
1  运行失败
2  用法 / 配置错误
3  认证失败
4  网络 / 连接失败
5  状态不允许
```

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
5A-4  CLI 文档
```

5A-1 验收：

```sh
levier version
levier serve -config levier.example.yaml
levier -config levier.example.yaml
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

不自动修改 shell 配置，不跳过 checksum。Windows 另提供 `install.ps1`。

### 2.6 容器发布

```text
ghcr.io/<org>/levier:v0.1.0
ghcr.io/<org>/levier:latest
ghcr.io/<org>/levier-harness:v0.1.0
ghcr.io/<org>/levier-mcp-runtime:v0.1.0
ghcr.io/<org>/levier-agent:v0.1.0
```

先 linux/amd64，再补 arm64。

### 2.7 go install

前提：

```text
仓库公开
module path = github.com/<org>/levier
打了 v0.1.0 tag
```

私有仓库需要内部 Go proxy 或只走 release 二进制。

### 2.8 文档与安全

```text
docs/INSTALL.md
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
5A-4  CLI 文档
5B-1  release workflow + checksums
5B-2  install.sh / install.ps1
5B-3  GHCR 镜像
5B-4  go install 验证
5B-5  INSTALL 文档
```

## 4. 总验收

```text
[ ] levier version 输出版本/commit/平台
[ ] levier -config 兼容旧启动方式
[ ] levier serve 可用
[ ] levier doctor 能逐跳报错
[ ] levier status --json 可用
[ ] levier app init 幂等
[ ] levier run submit --watch 跑到 completed
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
