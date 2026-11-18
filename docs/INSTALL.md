<!-- Levier — installation guide. -->

# 安装 Levier CLI

> 适用：5B 分发链路的用户安装手册。
> 相关文档：[CLI 参考](CLI_REFERENCE.md)、[发布流程](RELEASE.md)、[文档索引](README.md)。

> **当前项目仍处于私有内测阶段。**
> 仓库公开前，默认的 GitHub Release、`raw.githubusercontent.com`、GHCR 和公开
> `go install` 都需要认证；下面的匿名安装命令要等仓库公开后才真正可用。

---

## 1. 先选择安装方式

| 方式 | 适合谁 | 命令 |
|---|---|---|
| `install.sh` | Linux / macOS 用户 | `sh install.sh` |
| `install.ps1` | Windows 用户 | `powershell -File install.ps1` |
| `go install` | 已安装 Go 的开发者 | `go install ...@version` |
| 手动下载 | 需要审计、离线、特殊目录 | 下载 Release 压缩包 |
| 容器镜像 | 运行整套服务 | `docker pull ghcr.io/...` |

普通 CLI 用户优先使用安装器。
Go 开发者可以使用 `go install`。
需要运行 Levier + Harness + MCP Runtime 时使用容器。

---

## 2. 支持矩阵

CLI 二进制：

| OS | Arch | 格式 |
|---|---|---|
| Linux | amd64 | `.tar.gz` |
| Linux | arm64 | `.tar.gz` |
| macOS | amd64 | `.tar.gz` |
| macOS | arm64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

当前不发布：

```text
Windows arm64
32 位平台
```

容器镜像：

```text
linux/amd64
```

---

## 3. 安装前提

### Linux / macOS

需要：

```text
curl 或 wget
tar
sha256sum 或 shasum
```

### Windows

需要：

```text
Windows PowerShell 5.1+
Expand-Archive
Get-FileHash
```

### go install

需要：

```text
Go 1.26+
```

### 容器

需要：

```text
Docker
可选：Docker Compose
```

---

## 4. 版本与 latest

Release 有两种类型：

```text
稳定版：v0.1.0
预发布版：v0.1.0-rc.1
```

安装器默认使用 **latest stable**：

```text
最新稳定版
不包括 prerelease
```

如果要安装预发布或固定版本，必须显式指定：

```sh
sh install.sh --version v0.1.0-rc.1
```

如果目前只有 prerelease、没有稳定版，默认 latest 查询会失败。
这属于正常现象，请使用 `--version`。

生产环境建议固定版本，不要长期依赖 `latest`。

---

## 5. Linux / macOS：install.sh

### 5.1 下载脚本

```sh
curl -fsSL \
  https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh \
  -o /tmp/levier-install.sh
```

建议先检查脚本：

```sh
less /tmp/levier-install.sh
```

再执行：

```sh
sh /tmp/levier-install.sh
```

不要使用未检查的 `curl | sh` 作为生产安装方式。

### 5.2 固定版本

```sh
sh /tmp/levier-install.sh --version v0.1.0
```

### 5.3 自定义目录

```sh
sh /tmp/levier-install.sh \
  --version v0.1.0 \
  --prefix "$HOME/.local/bin"
```

默认目录：

```text
root 或 /usr/local/bin 可写 -> /usr/local/bin
否则                         -> ~/.local/bin
```

### 5.4 预览

```sh
sh /tmp/levier-install.sh \
  --version v0.1.0 \
  --prefix "$HOME/.local/bin" \
  --dry-run
```

`--dry-run` 只打印版本、平台、URL 和目标目录，不下载、不写入。

### 5.5 PATH

安装器不会自动修改 shell 配置。如果目录不在 `PATH`，会打印：

```sh
export PATH="$HOME/.local/bin:$PATH"
```

需要持久化时：

```sh
# bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc

# zsh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

---

## 6. Windows：install.ps1

### 6.1 下载脚本

```powershell
Invoke-WebRequest `
  -Uri https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.ps1 `
  -OutFile "$env:TEMP\levier-install.ps1"
```

### 6.2 安装

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File "$env:TEMP\levier-install.ps1"
```

固定版本：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File "$env:TEMP\levier-install.ps1" `
  -Version v0.1.0
```

指定目录：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File "$env:TEMP\levier-install.ps1" `
  -Version v0.1.0 `
  -Prefix "$env:USERPROFILE\.local\bin"
```

默认目录：

```text
%USERPROFILE%\.local\bin
```

预览：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File "$env:TEMP\levier-install.ps1" `
  -Version v0.1.0 `
  -DryRun
```

安装器不会自动修改用户 PATH，只打印目标目录。

---

## 7. go install

前提：

```text
仓库公开
Go 1.26+
存在 SemVer tag
```

安装：

```sh
go install github.com/fluffyTighnari/levier/cmd/levier@v0.1.0
```

安装位置：

```text
$GOBIN
或 $(go env GOPATH)/bin
```

确认：

```sh
"$(go env GOPATH)/bin/levier" version
```

如果设置了 `GOBIN`：

```sh
"$GOBIN/levier" version
```

说明：

```text
go install 不经过 release 压缩包
不需要 curl
不会执行 install.sh
GOBIN 目录需要自己加入 PATH
```

版本元数据：

```text
version 来自模块 tag
commit / built 可能显示 unknown
```

因为 `go install` 不会传入本项目的 Makefile ldflags。

私有仓库：

```text
使用 GOPRIVATE + 内部 Go proxy
或改走 release 二进制
```

---

## 8. 手动下载安装

Release 资产：

```text
levier_v0.1.0_linux_amd64.tar.gz
levier_v0.1.0_linux_arm64.tar.gz
levier_v0.1.0_darwin_amd64.tar.gz
levier_v0.1.0_darwin_arm64.tar.gz
levier_v0.1.0_windows_amd64.zip
checksums.txt
```

Linux / macOS：

```sh
tar -xzf levier_v0.1.0_linux_amd64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 0755 levier "$HOME/.local/bin/levier"
```

Windows：

```powershell
Expand-Archive .\levier_v0.1.0_windows_amd64.zip -DestinationPath .
.\levier.exe version
```

手动安装也必须先校验 checksum。

---

## 9. 容器镜像

发布镜像：

```text
ghcr.io/<owner>/levier:v0.1.0
ghcr.io/<owner>/levier-harness:v0.1.0
ghcr.io/<owner>/levier-mcp-runtime:v0.1.0
ghcr.io/<owner>/levier-agent:v0.1.0
```

稳定版还会更新：

```text
ghcr.io/<owner>/levier:latest
ghcr.io/<owner>/levier-harness:latest
ghcr.io/<owner>/levier-mcp-runtime:latest
ghcr.io/<owner>/levier-agent:latest
```

当前平台：

```text
linux/amd64
```

查看 CLI 版本：

```sh
docker run --rm \
  --entrypoint /app/levier \
  ghcr.io/<owner>/levier:v0.1.0 version
```

Compose 中使用镜像时，把对应服务的 `build:` 替换为 `image:`，并保留原有
`environment`、`ports`、`depends_on` 和 volume 配置。

服务端镜像启动顺序：

```text
migrate up -> serve
```

数据库和 Admin token 通过环境变量注入。

私有 GHCR 需要登录：

```sh
docker login ghcr.io
```

仓库和 package 公开后可以匿名拉取。

---

## 10. 校验

安装器始终：

```text
下载 checksums.txt
定位目标归档的 sha256
计算下载归档的 sha256
不一致立即退出
```

没有跳过校验的开关。

如果只下载了单个归档，手动校验：

Linux：

```sh
grep 'levier_v0.1.0_linux_amd64.tar.gz' checksums.txt | sha256sum -c -
```

macOS：

```sh
grep 'levier_v0.1.0_linux_amd64.tar.gz' checksums.txt | shasum -a 256 -c -
```

注意：

```text
checksum 用于发现损坏或误替换
它不是发布者签名
后续可增加 cosign / sigstore
```

---

## 11. 升级

### install.sh

重新执行安装器并指定新版本：

```sh
sh install.sh --version v0.2.0 --prefix "$HOME/.local/bin"
```

安装器会原子替换目标二进制。

### install.ps1

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\install.ps1 -Version v0.2.0
```

### go install

```sh
go install github.com/fluffyTighnari/levier/cmd/levier@v0.2.0
```

### 容器

```sh
docker pull ghcr.io/<owner>/levier:v0.2.0
```

生产环境使用不可变版本 tag，不建议长期使用 `latest`。

升级后验证：

```sh
levier version
```

---

## 12. 卸载

### Linux / macOS

```sh
rm -f "$HOME/.local/bin/levier"
```

如果安装在 `/usr/local/bin`：

```sh
sudo rm -f /usr/local/bin/levier
```

### Windows

```powershell
Remove-Item "$env:USERPROFILE\.local\bin\levier.exe"
```

### go install

```sh
bin="$(go env GOBIN)"; [ -n "$bin" ] || bin="$(go env GOPATH)/bin"
rm -f "$bin/levier"
```

### PATH

安装器不会删除 PATH。
如果手动添加过，需要自行编辑 shell / 用户环境变量。

---

## 13. 离线 / 镜像安装

安装器支持从镜像目录读取 release 文件：

```sh
sh install.sh \
  --version v0.1.0 \
  --base-url https://mirror.example.com/levier/v0.1.0 \
  --prefix "$HOME/.local/bin"
```

镜像目录必须与 GitHub Release 布局相同：

```text
levier_v0.1.0_linux_amd64.tar.gz
levier_v0.1.0_linux_arm64.tar.gz
levier_v0.1.0_darwin_amd64.tar.gz
levier_v0.1.0_darwin_arm64.tar.gz
levier_v0.1.0_windows_amd64.zip
checksums.txt
```

`--base-url` 必须和 `--version` 一起使用。

Windows：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\install.ps1 `
  -Version v0.1.0 `
  -BaseUrl https://mirror.example.com/levier/v0.1.0
```

Fork 或内部镜像可覆盖默认仓库：

```sh
sh install.sh --repo <owner>/<repo>
```

```powershell
.\install.ps1 -Repo <owner>/<repo>
```

`LEVIER_REPO` 环境变量也可覆盖 Unix 安装器的默认仓库。

---

## 14. 安装后验证

```sh
levier version
levier help
levier doctor
```

如果服务已经启动：

```sh
levier status
```

`doctor` 检查：

```text
配置
数据库
migration
healthz
readyz
Admin token
可选 App 凭据
```

---

## 15. 退出码

```text
0  安装成功或 dry-run 成功
1  下载、校验、解压、写入失败
2  参数错误、平台不支持、缺少必要选项
```

---

## 16. 常见错误

### GitHub 返回 404 或 401

```text
仓库仍是私有
或该版本 / Release 不存在
```

公开仓库后可以匿名下载；私有仓库需要认证或内部镜像。

### `could not resolve latest release`

```text
目前只有 prerelease，没有 stable release
使用 --version / -Version 指定版本
```

### `checksum mismatch`

```text
下载文件损坏，或镜像内容与 checksums.txt 不一致
删除临时文件后重试；不要跳过校验
```

### GHCR 返回 403

```text
package 仍是私有
先 docker login ghcr.io
或把 package 可见性改为 public
```

### `unsupported OS` / `unsupported architecture`

```text
Linux / macOS 使用 install.sh
Windows 使用 install.ps1
当前仅支持支持矩阵中的平台
```

### 安装成功但找不到 `levier`

```text
目标目录不在 PATH
按安装器输出的 PATH hint 操作
```

### `go install` 无权限

```text
私有仓库需要 GOPRIVATE / 内部 proxy
公开仓库确认 tag 存在
```

---

## 17. 安全约定

```text
不跳过 checksum
不把 token 写进下载 URL
不在未检查脚本时直接 curl | sh
生产使用不可变版本 tag
私有镜像先 docker login
```

后续增强：

```text
checksums 签名
cosign / sigstore
SBOM
```
