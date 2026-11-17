<!-- Levier — installation guide. -->

# 安装 Levier CLI

> 适用：5B 初版安装器。容器安装见后续 5B-3，`go install` 见 5B-4。

---

## 1. 支持矩阵

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

---

## 2. Linux / macOS

### 2.1 下载安装脚本

```sh
curl -fsSL \
  https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh \
  -o /tmp/levier-install.sh
```

建议先查看脚本再执行：

```sh
less /tmp/levier-install.sh
sh /tmp/levier-install.sh
```

不要使用未校验的 `curl | sh` 作为生产安装方式。

### 2.2 固定版本

```sh
sh /tmp/levier-install.sh --version v0.1.0
```

### 2.3 指定目录

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

### 2.4 预览

```sh
sh /tmp/levier-install.sh \
  --version v0.1.0 \
  --prefix "$HOME/.local/bin" \
  --dry-run
```

`--dry-run` 只打印版本、平台、URL 和目标目录，不下载、不写入。

### 2.5 PATH

安装器不会自动修改 shell 配置。如果目标目录不在 `PATH` 中，它会打印：

```sh
export PATH="$HOME/.local/bin:$PATH"
```

需要持久化时写入：

```sh
# bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc

# zsh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

---

## 3. Windows

在 PowerShell 中：

```powershell
Invoke-WebRequest `
  -Uri https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.ps1 `
  -OutFile "$env:TEMP\levier-install.ps1"

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

`-DryRun` 预览：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File "$env:TEMP\levier-install.ps1" `
  -Version v0.1.0 `
  -DryRun
```

安装器不会自动修改用户 PATH，只打印目标目录。

---

## 4. 校验

安装器始终：

```text
下载 checksums.txt
定位目标归档的 sha256
计算下载归档的 sha256
不一致立即退出
```

没有跳过校验的开关。

手动验证 release 归档：

```sh
cd dist
sha256sum -c checksums.txt
```

macOS：

```sh
cd dist
shasum -a 256 -c checksums.txt
```

---

## 5. 镜像 / 离线安装

安装器支持从镜像目录读取 release 文件：

```sh
sh install.sh \
  --version v0.1.0 \
  --base-url https://mirror.example.com/levier/v0.1.0 \
  --prefix "$HOME/.local/bin"
```

目录布局必须与 GitHub Release 相同：

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

默认仓库是 `fluffyTighnari/levier`。Fork 或内部镜像可覆盖：

```sh
sh install.sh --repo <owner>/<repo>
```

```powershell
.\install.ps1 -Repo <owner>/<repo>
```

`LEVIER_REPO` 环境变量也可覆盖 Unix 安装器的默认仓库。

---

## 6. 退出码

```text
0  安装成功或 dry-run 成功
1  下载、校验、解压、写入失败
2  参数错误、平台不支持、缺少必要选项
```

---

## 7. 卸载

Linux / macOS：

```sh
rm -f "$HOME/.local/bin/levier"
```

Windows：

```powershell
Remove-Item "$env:USERPROFILE\.local\bin\levier.exe"
```

如果手动把目录加入了 PATH，需要自行移除对应 PATH 配置。

---

## 8. 常见错误

### `unsupported OS`

```text
Linux / macOS 使用 install.sh
Windows 使用 install.ps1
Git Bash / MSYS 不是受支持的安装环境
```

### `unsupported architecture`

当前只发布：

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
```

### `checksum mismatch`

```text
下载文件损坏，或镜像内容与 checksums.txt 不一致
删除临时文件后重试；不要跳过校验
```

### 安装成功但 `levier` 找不到

```text
目标目录不在 PATH
按安装器输出的 PATH hint 操作
```

---

## 9. 容器镜像

5B-3 起，release tag 会发布 GHCR 镜像：

```text
ghcr.io/<owner>/levier:v0.1.0
ghcr.io/<owner>/levier-harness:v0.1.0
ghcr.io/<owner>/levier-mcp-runtime:v0.1.0
ghcr.io/<owner>/levier-agent:v0.1.0
```

稳定版本同时更新：

```text
ghcr.io/<owner>/levier:latest
ghcr.io/<owner>/levier-harness:latest
ghcr.io/<owner>/levier-mcp-runtime:latest
ghcr.io/<owner>/levier-agent:latest
```

当前发布平台：

```text
linux/amd64
```

镜像也可以在 Compose 中通过 `image:` 引用，替代本地 `build:`。

查看 CLI 版本（覆盖 entrypoint）：

```sh
docker run --rm \
  --entrypoint /app/levier \
  ghcr.io/<owner>/levier:v0.1.0 version
```

服务端镜像仍然按：

```text
migrate up -> serve
```

启动，数据库和 Admin token 通过环境变量注入。

