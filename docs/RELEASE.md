<!-- Levier — release process. -->

# 发布与校验

> 适用：5B-1 release 二进制 + checksums。
> 用户安装说明见 [`INSTALL.md`](INSTALL.md)；本文面向维护者。

---

## 1. 发布物

每个 `v*` tag 生成：

```text
levier_v0.1.0_linux_amd64.tar.gz
levier_v0.1.0_linux_arm64.tar.gz
levier_v0.1.0_darwin_amd64.tar.gz
levier_v0.1.0_darwin_arm64.tar.gz
levier_v0.1.0_windows_amd64.zip
checksums.txt
```

压缩包根目录只包含：

```text
levier
或
levier.exe
```

`checksums.txt` 格式：

```text
<sha256>  <archive-name>
```

---

## 2. 本地构建

```sh
make release-dist
```

或显式指定版本：

```sh
VERSION=v0.1.0 \
COMMIT="$(git rev-parse --short HEAD)" \
BUILT="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
sh scripts/build-release.sh
```

输出目录：

```text
dist/
```

脚本要求：

```text
go
tar
sha256sum 或 shasum
zip 或 python3（Windows 压缩包）
```

---

## 3. 校验

Linux：

```sh
cd dist
sha256sum -c checksums.txt
```

macOS：

```sh
cd dist
shasum -a 256 -c checksums.txt
```

抽查版本信息：

```sh
tar -xzf levier_v0.1.0_linux_amd64.tar.gz -C /tmp
/tmp/levier version
```

预期：

```text
levier v0.1.0
commit: <short sha>
built: <UTC timestamp>
```

---

## 4. GitHub Release

发布前：

```text
CI 通过
工作区干净
版本号已确认
```

打 tag：

```sh
git tag -a v0.1.0 -m "Levier v0.1.0"
git push origin v0.1.0
```

tag 必须形如 `vMAJOR.MINOR.PATCH`；预发布可用 `v0.1.0-rc.1`。

`.github/workflows/release.yml` 会：

```text
1. verify：make lint / make deps-check / make test
2. 构建 5 个平台压缩包
3. 生成 checksums.txt
4. gh release create 或 gh release upload --clobber
5. images job：构建并推送 4 个 GHCR 镜像
```

tag 含 `-`（例如 `v0.1.0-rc.1`）时自动标记为 prerelease。

失败重跑：

```text
workflow 会重新构建并覆盖同名 release assets
不会删除已有 release
```

### 4.1 GHCR 镜像

同一个 release workflow 的 `images` job 发布：

```text
ghcr.io/<owner>/levier:v0.1.0
ghcr.io/<owner>/levier-harness:v0.1.0
ghcr.io/<owner>/levier-mcp-runtime:v0.1.0
ghcr.io/<owner>/levier-agent:v0.1.0
```

稳定 tag 额外发布 `:latest`；预发布 tag 不更新 `latest`。

当前平台：

```text
linux/amd64
```

实现：

```text
docker/setup-buildx-action
docker/login-action -> ghcr.io
docker/metadata-action -> version / latest / OCI labels
docker/build-push-action -> push
```

权限：

```text
packages: write
GITHUB_TOKEN 登录 ghcr.io
```

---

## 5. 版本元数据

构建时注入：

```text
internal/version.Version = VERSION
internal/version.Commit  = short commit
internal/version.Built   = UTC build time
```

`levier version` 输出：

```text
levier v0.1.0
commit: abc1234
built: 2026-11-17T00:00:00Z
go: go1.26.x
platform: linux/amd64
```

---

## 6. 安全约定

```text
不把 token 写入构建参数或 artifact
release workflow 使用最小权限 contents: write
生产安装必须校验 sha256
镜像使用不可变版本 tag
后续可增加 cosign / sigstore 签名
```

---

## 7. 本地验收

```sh
go test ./...
make lint
make release-dist
cd dist && sha256sum -c checksums.txt
```
