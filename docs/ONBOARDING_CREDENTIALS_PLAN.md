<!-- Levier — Phase 11 onboarding and credential management plan. -->

# 11. 引导与凭据管理计划

> 状态：第一版已实现（凭据契约、App 引导、`app check`、`app list` 和文档）；App archive/delete 与多 key 重叠仍不在本阶段。
> 目标不是增加一套新的 secret 名词，而是让第一次使用 Levier 的人能安全地完成两件事：启动系统并确认它准备好；创建 App 凭据、接入 App、验证接入成功。

---

## 1. 最终要达到的效果

### 1.1 新用户第一次启动

用户拿到仓库后，不需要先读控制面源码，也不需要猜每个 token 属于谁。按一条文档路径完成：

```text
准备本地配置
→ docker compose up
→ levier selfcheck
→ 创建第一个 App
→ 运行一次 echo Run
```

启动后，用户能明确看到：

```text
服务是否健康
数据库和迁移是否正常
Harness 是否注册
App 是否已认证
一次 Run 是否真正完成
```

生产环境的 Admin token、daemon token、数据库密码和 MCP secret 仍然属于部署凭据，由部署方式负责注入和轮换。本阶段不会把这些凭据搬到 App 凭据文件里，也不会实现第二个 Secret Manager。

### 1.2 App 第一次接入

App 开发者只使用一个入口：

```sh
levier app init --name counter-agent
```

命令完成后，得到一个明确的凭据文件路径，而不是屏幕上的临时 key。文件默认写到用户配置目录下的 `levier/`，权限为 0600，包含 App 身份、服务地址和可识别的凭据版本信息。

开发者用同一个文件启动 App：

```sh
LEVIER_CREDENTIALS=~/.config/levier/counter-agent.json \
  python3 examples/counter_agent/app.py
```

然后运行一个只读检查：

```sh
levier app check --credentials ~/.config/levier/counter-agent.json
```

这个命令应该回答：

```text
凭据文件格式是否正确
文件权限是否安全
服务端是否可连接
App key 是否仍然有效
App 是否处于 active 状态
是否能够取得 WebSocket token
```

检查通过后，开发者不需要再查别的文档，就能提交第一个 Run。

### 1.3 凭据生命周期

凭据文件可以被查看，但不能把 key 打印到终端或日志。

用户能够看到：

```text
App ID 和名称
服务地址
凭据文件版本
凭据文件路径
最近更新时间
key 的不可逆指纹
```

用户能够在确认后轮换 key：

```text
新 key 只返回一次
旧 key 立即失效
凭据文件先写临时文件，再原子替换
旧文件保留为受保护的备份
轮换后立即验证新 key
```

本阶段仍然不支持多把 key 同时有效。轮换是立即切换，因此命令必须在服务端轮换前完成本地文件可写性检查，并在轮换后立即落盘；如果落盘失败，必须明确告诉用户旧 key 已失效以及如何用新 key 恢复，不能静默失败。

### 1.4 两类凭据边界

本阶段会把两类东西明确分开：

```text
部署凭据
  Admin token、daemon token、数据库密码、MCP secret
  属于运维环境，由部署配置和 secret 文件管理

App 凭据
  app_id、App API key、服务地址
  属于一个 App，写入本地凭据文件，用于运行时的 HTTP / WebSocket 认证
```

用户不应该把 Admin token 放进 App 的长期运行环境，也不应该用 App key 管理其他 App。

---

## 2. 当前的主要缺口

当前能力已经能工作，但入口和契约不一致：

- Go CLI 和 Counter Agent 示例使用了不同的默认凭据目录和字段习惯；旧目录 `middleware` 与新的 `levier` 命名并存。
- Go CLI 读取凭据时没有像 Python 示例那样检查文件权限。
- 凭据文件没有版本字段，也没有保存服务地址；用户换一个服务端或排查接入问题时要重新猜参数。
- `app init` 创建 App 后没有自动验证 key；创建成功不等于接入成功。
- 没有 `app check`，用户只能分别运行 `app show`、`doctor` 或手工请求 WebSocket token。
- `app show` 展示的是服务端 App 元数据，不能看到本地凭据文件的版本、指纹和路径。
- Admin API 没有 App 列表；运维排查时只能查数据库。
- 文档把基础启动、App 接入、凭据轮换分散在多个页面，第一次接入的人需要自己拼路径。

---

## 3. 需要做的改变

### 3.1 统一凭据文件契约

定义一份带版本的 App 凭据文件，建议字段为：

```json
{
  "credential_version": 1,
  "app_id": "app_...",
  "app_name": "counter-agent",
  "server": "http://127.0.0.1:8080",
  "api_key": "mw_...",
  "created_at": "2026-11-19T00:00:00Z",
  "updated_at": "2026-11-19T00:00:00Z"
}
```

`server` 不是秘密，保存它可以让 `app check`、`app show` 和示例程序在用户不重复传参数的情况下完成验证。读取旧文件时，只要求 `app_id`、`app_name` 和 `api_key`；缺少新字段仍可读，下一次写入时升级格式。

默认路径统一到：

```text
~/.config/levier/<app-name>.json
```

`LEVIER_CREDENTIALS` 继续作为显式覆盖。旧的 `~/.config/middleware/` 文件仍可读取，但新文件不再写到旧目录。

所有写入都必须：

```text
目录 0700
文件 0600
临时文件写完、同步后再原子替换
轮换时生成受保护的备份
不把 key 写进日志、错误信息或命令输出
```

Linux / macOS 上读到 group 或 world 可读的凭据文件时直接拒绝；Windows 无法可靠检查 POSIX 权限，只给出安全警告。

### 3.2 CLI 的引导职责

`levier app init` 不再只是“把返回的 key 写进文件”。它会在成功前完成：

```text
检查凭据文件是否已存在
检查目标目录是否可写
创建或找到 App
取得 key
原子写入凭据文件
立即验证 key 能访问该 App
提示下一步启动 App 和运行 app check
```

已有 App 且服务端不返回 key 时，仍然提示用户运行 `app rotate-key`；不会假装初始化成功。

新增 `levier app check`，默认只读：

```sh
levier app check --credentials <path>
```

它读取本地文件，完成本地文件检查和一次服务端验证；如果服务端支持，再检查 WebSocket token。它不创建 Run，不修改工具，也不打印 key。需要更深的链路检查时，再通过显式选项打开，不把“接入检查”变成会写数据的命令。

`levier app show` 保持服务端 App 元数据查询，同时展示本地凭据文件的服务地址、版本、更新时间和 key 指纹。key 本身永远不输出。

`levier app rotate-key` 在轮换前检查目标文件可写，轮换后立即写回并验证新 key。备份文件仍受 0600 保护。

`levier doctor` 和 `levier run` 复用同一套凭据读取和权限检查，避免不同命令对同一个文件给出不同判断。

### 3.3 服务端增加安全的 App 发现能力

增加一个只读的 Admin App 列表接口和 CLI：

```text
GET /admin/apps
levier app list
```

返回 App ID、名称、状态、创建时间、更新时间，不返回 key 或 key hash。它用于运维确认有哪些 App，以及为后续轮换找到准确的 App ID。

本阶段不新增 App archive、delete 或恢复接口。需要撤销某个 App 的访问时，使用 key 轮换，让旧 key 立即失效；完整的 App 生命周期管理留到后续阶段。

### 3.4 示例程序与文档统一

Counter Agent 的 bootstrap 和运行示例改用统一凭据目录、统一字段和同一个 CLI 流程。Python 示例仍然保留自己的 HTTP 引导脚本，但默认输出目录和文件结构必须与 Go CLI 一致。

新增一份主引导文档，建议为：

```text
docs/ONBOARDING.md
```

它只分两条路径：

```text
第一次启动 Levier（运维）
第一次接入一个 App（开发者）
```

`README`、`APP_ONBOARDING.md`、`CLI_REFERENCE.md` 和 `CONFIG.md` 都指向这份主文档；不再让用户自己从多个页面拼出接入顺序。

### 3.5 测试与验收

需要覆盖：

```text
新凭据文件写入和读取
旧凭据文件兼容读取
权限检查与拒绝
app init 创建、已有 App、轮换失败和文件写入失败
app check 的成功、认证失败和服务不可达
app rotate-key 的备份、原子替换和旧 key 失效
app list 只返回安全元数据
示例程序使用统一凭据文件
没有 key 出现在日志、JSON 或普通输出中
```

最终在全新 Compose 环境中验证完整路径：

```text
启动
→ 创建 App
→ 写凭据文件
→ app check 通过
→ App 启动并取得 WS token
→ 提交 echo Run 并完成
→ 轮换 key
→ 旧 key 返回 401
→ 新 key 仍可认证
→ CI、lint、smoke 和 selfcheck 全部通过
```

---

## 4. 不做什么

本阶段不做：

```text
Vault / KMS 或新的 Secret Manager
自动轮换 key
多把 App key 同时有效
App archive / delete / restore
GUI 凭据管理
把 Admin token、daemon token、数据库密码搬进 App 凭据文件
```

这些边界是为了让本阶段真正解决“第一次接入”和“日常凭据使用”，而不是把凭据管理扩散成完整的租户或密钥平台。

---

## 5. 实施顺序

1. 冻结凭据文件格式、默认路径、权限规则和两类凭据边界。
2. 抽出 Go 和 Python 都能遵循的读写、指纹、权限和原子替换规则。
3. 改造 `app init`、`app show`、`app rotate-key`，新增 `app check`。
4. 增加只读 Admin App 列表和 `app list`。
5. 统一示例程序、默认目录和主引导文档。
6. 补齐单元测试、CLI 测试、Compose 验收和 secret 扫描。

---

## 6. 已采用的默认决策

第一版直接采用以下默认值：

```text
默认目录使用 ~/.config/levier/
凭据文件保存服务地址
本阶段只做 App 列表，不做 archive/delete
轮换保持立即失效，不做双 key 重叠
Admin/daemon/数据库/MCP secret 继续沿用 10D 的部署与轮换方式
```
