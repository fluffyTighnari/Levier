<!-- Levier — release readiness and next-phase plan. -->

# Levier TODO

> 当前决策：**暂不公开仓库，不创建正式 `v0.1.0`**。
> 5 CLI 与分发进入冻结状态；下一阶段是 **6 配置外部化**。
> 最后更新：2026-11-19。

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

- [x] 列出所有配置项
- [x] 按组件分组：control plane / harness / MCP / agent
- [x] 标记类型、默认值、是否必填
- [x] 标记是否敏感
- [x] 标记是否支持环境变量
- [ ] 定义统一命名规则（已有建议，待评审）
- [x] 定义 duration / size / URL / token 的格式
- [x] 决定引入配置版本号 `config_version: 1`
- [x] 输出 `docs/CONFIG.md` 初版
- [x] 输出完整环境变量矩阵

产出：

```text
docs/CONFIG.md
```

下一步：评审第 11 节“建议的契约”，冻结后再进入 6B。

### 2.4 6B：控制面配置

- [x] `internal/config` 支持完整外部化
- [x] 补齐缺失的环境变量覆盖
- [x] 未知字段继续拒绝
- [x] 启动时输出可读的配置错误
- [x] 生产模式拒绝示例默认 token
- [x] 配置文件权限检查提示
- [x] 数据库 URL 脱敏
- [x] `daemon_tokens` 支持安全注入
- [x] 明确 secret 与普通配置的边界
- [x] 增加 `levier config check`
- [x] 增加 `levier config print`
- [x] `config print` 默认脱敏
- [x] `config print --json` 稳定输出
- [x] `config check` 退出码稳定

### 2.5 6C：执行侧配置

- [x] Harness 配置完整外部化
- [x] MCP Runtime 配置完整外部化
- [x] Agent 配置完整外部化
- [x] 统一 `LEVIER_*` 环境变量命名
- [x] secret 文件权限与路径约定
- [x] 日志中统一脱敏
- [x] Compose 生产 overlay 不再写死开发 token（6D）
- [x] 保留旧 `MIDDLEWARE_*` / `MW_*` 兼容窗口

### 2.6 6D：部署配置

- [x] `.env.example` 区分普通配置与 secret
- [x] Compose 使用外部配置 / secret 文件
- [x] 提供最小生产配置样例
- [x] 提供开发配置样例
- [x] 不把真实密钥写进镜像
- [x] 不把真实密钥写进 release
- [x] 明确数据库、Admin token、daemon token 的来源
- [x] 明确 MCP secret 的注入方式
- [x] 记录 `migrate up -> serve` 启动顺序

> 本地 Compose smoke 已通过：`docker compose up -d --no-build` + `docker compose --profile smoke run --rm smoke`。

### 2.7 6E：测试与文档

- [x] 配置解析单元测试
- [x] 环境变量优先级测试
- [x] 未知字段测试
- [x] 非法类型测试
- [x] secret 脱敏测试
- [x] `config check` 测试
- [x] `config print` JSON 测试
- [x] Compose 启动 smoke
- [x] `docs/CONFIG.md`
- [x] `docs/CLI_REFERENCE.md` 增加 config 命令
- [x] `docs/INSTALL.md` 增加配置说明入口
- [x] 配置迁移说明

> 本地 Compose smoke 已通过；生产式 overlay 的配置检查已通过，容器 smoke 仍使用开发栈。

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

## 3. 下一阶段：7 服务自描述

计划文档：`docs/SELF_DESCRIPTION_PLAN.md`

- [x] 计划落盘
- [x] `public/service-meta/v1/schema.json`
- [x] `public/service-meta/v1/example.json`
- [x] `internal/selfdesc`
- [x] `GET /meta`
- [x] `levier describe`
- [x] `doctor` 检查 `/meta`
- [x] HTTP / CLI / 契约测试
- [x] Compose smoke 检查 `/meta`
- [x] CLI_REFERENCE / README / 公开契约说明

### 3.1 公开边界

```text
docs/       内部开发文档，不进入镜像 / release / HTTP
public/     对外暴露的契约和示例
GET /meta   运行时自描述内容
```

---

## 4. 下一阶段：8 状态总览

计划文档：`docs/STATUS_OVERVIEW_PLAN.md`

- [x] 计划落盘
- [x] `public/service-status/v1/schema.json`
- [x] `public/service-status/v1/example.json`
- [x] `internal/status`
- [x] `GET /status`
- [x] `levier status` 使用 `/status`
- [x] 旧 metrics 回退
- [x] HTTP / CLI / 契约测试
- [x] Compose smoke 检查 `/status`
- [x] CLI_REFERENCE / README / 公开契约说明

### 4.1 公开边界

```text
docs/       内部开发文档，不进入镜像 / release / HTTP
public/     对外暴露的契约和示例
GET /meta   静态能力自描述
GET /status 动态运行状态总览
```

---

## 5. 下一阶段：9 逐跳自检

计划文档：`docs/HOP_CHECK_PLAN.md`

- [x] 计划落盘
- [x] `internal/selfcheck`
- [x] `levier selfcheck` 只读模式
- [x] `--deep` 主动链路检查
- [x] hop 依赖和 skip 规则
- [x] JSON / 人类可读输出
- [x] secret 脱敏测试
- [x] CLI / 文档
- [x] 复用 daemoncheck / app_check 能力

> `--deep` 当前覆盖 daemon 协议、App WebSocket 和 MCP health；真实 local tool 走专用 `app_check.py`。

### 5.1 自检分层

```text
默认     只读检查：config / HTTP / DB / migration / meta / status / admin / daemon / app
--deep   主动检查：临时 App / Run / TaskAssign / events / WS / tool / MCP
```

### 5.2 公开边界

```text
docs/       内部开发文档
public/     对外契约和示例
/selfcheck  第一版不提供公共 HTTP 接口
```

---

## 6. 下一阶段：10 生命周期（进程与数据）

计划文档：`docs/LIFECYCLE_PLAN.md`

- [x] 计划落盘
- [x] 进程状态与启动/停止顺序
- [x] SIGTERM / SIGINT 优雅停止
- [x] 停止总超时与强制退出
- [x] Compose `stop_grace_period`
- [x] `internal/maintenance`
- [x] `levier maintenance status`
- [x] `maintenance prune --dry-run`
- [x] retention 默认关闭
- [x] session 清理 / 终态任务随 Run 级联
- [x] 备份 / 恢复流程
- [x] Admin / daemon / App key / DB password 轮换
- [x] Docker volume 生命周期说明

### 6.1 生命周期边界

```text
进程    由 Docker / systemd 负责外部重启
Levier  负责启动、就绪、优雅停止和恢复语义
数据    默认不自动删除；维护操作需显式 --yes
```

---

## 7. 执行顺序与门禁

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

7A 自描述契约
  └─ 门禁：public schema / example 冻结

7B 自描述核心
  └─ 门禁：selfdesc 单元测试通过

7C HTTP /meta
  └─ 门禁：无认证可读，且不泄露 secret

7D CLI / doctor
  └─ 门禁：describe / doctor 可用，JSON 可解析

7E smoke / 文档
  └─ 门禁：Compose smoke 出现 meta: ok，文档闭环

8A 状态契约
  └─ 门禁：public status schema / example 冻结

8B 状态聚合核心
  └─ 门禁：daemon / app / run / workflow 汇总测试通过

8C HTTP /status
  └─ 门禁：无认证可读，聚合快照不泄露标识

8D CLI status
  └─ 门禁：status / status --json 与 /status 一致

8E smoke / 文档
  └─ 门禁：Compose smoke 出现 status: ok，文档闭环

9A 自检契约
  └─ 门禁：hop 顺序、状态、退出码确认

9B 只读自检
  └─ 门禁：config / HTTP / DB / meta / status / admin 可诊断

9C 主动自检
  └─ 门禁：--deep 可验证 daemon / Run / WS / tool / MCP

9D 文档与 smoke
  └─ 门禁：selfcheck 文档、JSON、smoke 闭环

9E 验收
  └─ 门禁：全量测试和 CI 绿

10A 生命周期契约
  └─ 门禁：进程状态、退出码、数据保留规则冻结

10B 进程优雅停止
  └─ 门禁：SIGTERM / worker / HTTP / gRPC / 子进程清理通过

10C 数据维护（已完成）
  └─ 门禁：maintenance status / prune dry-run / 终态清理测试通过

10D 备份与轮换（已完成）
  └─ 门禁：备份恢复和 secret 轮换步骤可执行

10E 验收（已完成）
  └─ 门禁：smoke、selfcheck、文档、CI 全绿

公开前最终验收
  └─ 执行本文件第 1.2 节
  └─ 通过后才考虑 public + v0.1.0
```

---

## 8. 11 引导与凭据管理（第一版已实现）

计划文档：`docs/ONBOARDING_CREDENTIALS_PLAN.md`

目标是让第一次使用 Levier 的人从启动、创建 App、拿到凭据、验证接入到完成第一个 Run，只需要遵循一条清晰路径；同时统一 App 凭据的文件格式、权限、指纹和轮换行为。部署级 Admin / daemon / 数据库 / MCP 凭据继续沿用 10D 的边界和轮换方式。

- [x] 统一 App 凭据目录、版本、服务地址和权限
- [x] `app init` 创建后验证
- [x] `app check`
- [x] `app show` 指纹和本地凭据信息
- [x] `app rotate-key` 原子写回、备份和验证
- [x] `app list` 只读 Admin App 列表
- [x] Counter Agent 示例使用新凭据路径
- [x] 启动/接入主文档

本阶段仍不做 App archive/delete、多 key 重叠和新的 Secret Manager。

## 9. 相关文档

```text
docs/CLI_REFERENCE.md
docs/INSTALL.md
docs/RELEASE.md
docs/CONFIG.md
docs/CLI_DISTRIBUTION_PLAN.md
docs/MAP.md
docs/SELF_DESCRIPTION_PLAN.md
docs/STATUS_OVERVIEW_PLAN.md
docs/HOP_CHECK_PLAN.md
docs/LIFECYCLE_PLAN.md
docs/ONBOARDING_CREDENTIALS_PLAN.md
```
