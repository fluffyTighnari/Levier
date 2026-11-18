<!-- Levier — release readiness and next-phase plan. -->

# Levier TODO

> 当前决策：**暂不公开仓库，不创建正式 `v0.1.0`**。
> 5 CLI 与分发进入冻结状态；下一阶段是 **6 配置外部化**。
> 最后更新：2026-11-18。

---

## 0. 当前不做什么

```text
不把仓库改为 public
不创建正式 v0.1.0 tag
不继续新增 5B 功能
不删除当前测试 Release（保留为验证证据）
```

私有仓库中的 `v0.1.0-rc.1` 到 `v0.1.0-rc.5` 只用于内测，不是正式版本。

---

## 1. 5 CLI 与分发最终验收

### 1.1 已完成（实现与私有验证）

- [x] 5A CLI 命令面
- [x] `version` / `serve` / `doctor` / `status`
- [x] `app init/show/rotate-key`
- [x] `run submit/watch/cancel`
- [x] `migrate status/up`
- [x] `daemon check`
- [x] CLI 帮助、JSON、退出码、token 泄漏检查
- [x] `docs/CLI_REFERENCE.md`
- [x] 5B-1 5 平台 release 压缩包 + checksums
- [x] 5B-2 `install.sh` / `install.ps1`
- [x] 5B-3 四个 GHCR 镜像构建与推送配置
- [x] 5B-4 `go install` 私有认证验证与版本 fallback
- [x] 5B-5 `docs/INSTALL.md` 完整安装手册
- [x] GitHub Actions main / tag CI 全绿
- [x] `v0.1.0-rc.5` Release、checksums、installer、镜像任务通过

### 1.2 公开前必须完成

#### 仓库与历史

- [ ] 决定公开策略：
  - [ ] 新建干净的公开仓库（推荐）
  - [ ] 或重写现有仓库历史
- [ ] 从公开历史中移除 `.qta/`
- [ ] 决定旧提交作者是否重写
- [ ] 确认新提交作者为项目维护者身份
- [ ] 扫描历史提交中的 token / 密码 / API key
- [ ] 添加 `LICENSE`
- [ ] 添加 `SECURITY.md`（漏洞报告方式）
- [ ] 添加 `CONTRIBUTING.md`（可选，但建议）
- [ ] 检查 `.gitignore` 和 `.dockerignore`
- [ ] 确认 `.env`、`.env.local`、本地凭据未进入历史

#### 测试发布清理

- [ ] 删除 `v0.1.0-rc.1`
- [ ] 删除 `v0.1.0-rc.2`
- [ ] 删除 `v0.1.0-rc.3`
- [ ] 删除 `v0.1.0-rc.4`
- [ ] 删除 `v0.1.0-rc.5`
- [ ] 删除对应测试 Release
- [ ] 删除对应测试 GHCR 镜像版本
- [ ] 确认 `latest` 没有被测试版本覆盖

#### 仓库公开设置

- [ ] 仓库改为 public
- [ ] GHCR 四个 package 可见性改为 public
- [ ] 配置 branch protection
- [ ] 配置 tag protection / release tag 规则
- [ ] 确认 Actions 权限仍为最小权限
- [ ] 确认 package 匿名拉取可用

#### 文档

- [ ] 移除 `docs/INSTALL.md` 顶部“私有内测”提示
- [ ] 刷新 README 中的当前状态
- [ ] 确认 CLI_REFERENCE / INSTALL / RELEASE 链接
- [ ] 准备 release notes
- [ ] 确认没有内部地址、个人邮箱、测试 token

#### 正式发布

- [ ] 选择正式版本号，例如 `v0.1.0`
- [ ] 打 annotated tag
- [ ] 确认 release workflow 成功
- [ ] 确认 5 个平台资产 + checksums.txt
- [ ] 确认 4 个镜像 push 成功
- [ ] 确认公开 `go-install` job 成功

#### 公开后的最终验证

- [ ] Linux 干净环境：`install.sh` 匿名安装
- [ ] macOS 干净环境：`install.sh` 匿名安装
- [ ] Windows 干净环境：`install.ps1` 匿名安装
- [ ] `go install ...@v0.1.0` 匿名成功
- [ ] `docker pull` 匿名成功
- [ ] 安装后的 `levier version` 正确
- [ ] `sha256sum -c checksums.txt` 通过
- [ ] 运行一次 `levier status` / `doctor`
- [ ] 记录验证时间和机器平台

### 1.3 当前验证证据

```text
仓库：fluffyTighnari/Levier
可见性：private
main：00d6601 + 后续文档提交
最新测试 tag：v0.1.0-rc.5
Release：rc.2 / rc.3 / rc.4 / rc.5
```

已通过：

```text
main CI
tag CI
release workflow
release assets
checksums
Linux installer
Windows installer
私有认证 go install
四镜像构建
```

---

## 2. 下一阶段：6 配置外部化

### 2.1 目标

把“代码里能跑”变成“部署时可配置”：

```text
配置显式
来源清晰
默认值安全
密钥不落日志
启动时校验
部署不需要改代码
```

配置优先级统一为：

```text
命令行 flag
  > 环境变量
  > 配置文件
  > 内置默认值
```

### 2.2 当前状况

控制面：

```text
internal/config
levier.example.yaml
LEVIER_* 环境变量（部分覆盖）
```

执行侧：

```text
Harness 独立 YAML
MCP Runtime 参数 / 环境变量
Agent 环境变量
Compose 环境变量
```

问题：

```text
配置项分散
环境变量覆盖不完整
密钥和普通配置混在一起
示例中包含开发默认 token
config check / config print 还没有实现
不同组件的配置命名和校验不一致
```

### 2.3 6A：配置盘点与契约

- [ ] 列出所有配置项
- [ ] 按组件分组：control plane / harness / MCP / agent
- [ ] 标记类型、默认值、是否必填
- [ ] 标记是否敏感
- [ ] 标记是否支持环境变量
- [ ] 定义统一命名规则
- [ ] 定义 duration / size / URL / token 的格式
- [ ] 决定是否引入配置版本号，例如 `config_version: 1`
- [ ] 输出 `docs/CONFIG.md` 初版
- [ ] 输出完整环境变量矩阵

### 2.4 6B：控制面配置

- [ ] `internal/config` 支持完整外部化
- [ ] 补齐缺失的环境变量覆盖
- [ ] 未知字段继续拒绝
- [ ] 启动时输出可读的配置错误
- [ ] 生产模式拒绝示例默认 token
- [ ] 配置文件权限检查提示
- [ ] 数据库 URL 脱敏
- [ ] `daemon_tokens` 支持安全注入
- [ ] 明确 secret 与普通配置的边界
- [ ] 增加 `levier config check`
- [ ] 增加 `levier config print`
- [ ] `config print` 默认脱敏
- [ ] `config print --json` 稳定输出
- [ ] `config check` 退出码稳定

### 2.5 6C：执行侧配置

- [ ] Harness 配置完整外部化
- [ ] MCP Runtime 配置完整外部化
- [ ] Agent 配置完整外部化
- [ ] 统一 `LEVIER_*` 环境变量命名
- [ ] secret 文件权限与路径约定
- [ ] 日志中统一脱敏
- [ ] Compose 中不再写死开发 token
- [ ] 保留旧 `MIDDLEWARE_*` / `MW_*` 兼容窗口

### 2.6 6D：部署配置

- [ ] `.env.example` 区分普通配置与 secret
- [ ] Compose 使用外部配置 / secret 文件
- [ ] 提供最小生产配置样例
- [ ] 提供开发配置样例
- [ ] 不把真实密钥写进镜像
- [ ] 不把真实密钥写进 release
- [ ] 明确数据库、Admin token、daemon token 的来源
- [ ] 明确 MCP secret 的注入方式
- [ ] 记录 `migrate up -> serve` 启动顺序

### 2.7 6E：测试与文档

- [ ] 配置解析单元测试
- [ ] 环境变量优先级测试
- [ ] 未知字段测试
- [ ] 非法类型测试
- [ ] secret 脱敏测试
- [ ] `config check` 测试
- [ ] `config print` JSON 测试
- [ ] Compose 启动 smoke
- [ ] `docs/CONFIG.md`
- [ ] `docs/CLI_REFERENCE.md` 增加 config 命令
- [ ] `docs/INSTALL.md` 增加配置说明入口
- [ ] 配置迁移说明

### 2.8 6 阶段验收

```text
不改代码，只改配置，可以切换数据库 / 端口 / token / 日志 / worker 参数
错误配置启动时明确报错
config check 可在部署前执行
config print 不输出密钥
Compose 不包含真实 secret
日志不包含密钥
测试和文档覆盖全部配置项
```

### 2.9 暂不做

```text
外部 Secret Manager（Vault / KMS）
配置热更新
多租户配置中心
GUI 配置编辑
```

这些留到后续阶段。

---

## 3. 执行顺序与门禁

```text
当前
  └─ 5 CLI/分发冻结，继续私有

6A 配置盘点与契约
  └─ 门禁：配置清单、命名、secret 边界确认

6B 控制面配置
  └─ 门禁：config check / config print / 启动校验通过

6C 执行侧配置
  └─ 门禁：Harness / MCP / Agent 配置统一

6D 部署配置
  └─ 门禁：Compose 无真实 secret，最小生产样例可启动

6E 测试与文档
  └─ 门禁：测试、文档、lint、structure-check 全绿

公开前最终验收
  └─ 执行本文件第 1.2 节
  └─ 通过后才考虑 public + v0.1.0
```

---

## 4. 相关文档

```text
docs/CLI_REFERENCE.md
docs/INSTALL.md
docs/RELEASE.md
docs/CLI_DISTRIBUTION_PLAN.md
docs/MAP.md
```
