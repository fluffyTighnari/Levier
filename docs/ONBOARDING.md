<!-- Levier — onboarding: start the service and connect the first App. -->

# 从启动到第一个 Run

这份文档只做两件事：把 Levier 启动起来，以及把一个 App 安全地接入并完成第一个 Run。

---

## 1. 第一次启动 Levier

在仓库根目录准备本地配置：

```sh
cp .env.example .env
```

开发环境的默认值可以直接用于本地 smoke；生产环境不要使用示例 token 或密码。启动：

```sh
docker compose up --build -d
```

确认基础链路：

```sh
docker compose exec levier /app/levier selfcheck \
  --server http://127.0.0.1:8080
```

检查结果应包含：

```text
config
http /healthz
database
migrations
http /readyz
service /meta
service /status
daemon registered
selfcheck: PASS
```

此时 Levier 已经可以接收 App 请求。Admin token、daemon token、数据库密码和 MCP secret 仍然由部署配置管理；不要把它们写进 App 的长期运行环境。

---

## 2. 创建第一个 App

准备 Admin token：

```sh
export LEVIER_ADMIN_TOKEN=admin-change-me
```

开发环境默认 token 只用于本地。创建 App：

```sh
levier app init --name counter-agent
```

命令会完成：

```text
调用 Admin API 创建 App
取得一次性 API key
验证 key 可以访问该 App
写入 0600 凭据文件
输出凭据路径和 key 指纹
```

默认凭据路径：

```text
~/.config/levier/counter-agent.json
```

也可以用 `--credentials-out` 指定路径，或设置：

```sh
export LEVIER_CREDENTIALS=~/.config/levier/counter-agent.json
```

凭据文件包含 App ID、名称、服务地址和 API key。运行中的 App 只需要这个文件，不需要 Admin token。

---

## 3. 验证接入

```sh
levier app check --credentials ~/.config/levier/counter-agent.json
```

它会检查：

```text
凭据格式和版本
文件权限
服务端可达性
App key
App 状态
WebSocket token
```

看到：

```text
app check: PASS
```

表示凭据和接入链路都正常。

---

## 4. 启动示例 App

在仓库根目录：

```sh
LEVIER_CREDENTIALS=~/.config/levier/counter-agent.json \
  python3 examples/counter_agent/serve.py
```

首次启动会注册本地工具、取得 WebSocket token 并建立连接。保持它运行，然后在另一个终端提交 Run：

```sh
LEVIER_CREDENTIALS=~/.config/levier/counter-agent.json \
  levier run submit \
  --strategy echo \
  --prompt "hello" \
  --watch
```

看到终态 `completed` 即表示接入完成。

没有示例 App 时，也可以直接提交一个不依赖本地工具的 echo Run：

```sh
levier run submit \
  --credentials ~/.config/levier/counter-agent.json \
  --strategy echo \
  --prompt "hello" \
  --watch
```

---

## 5. 查看和轮换

查看 App 和本地凭据信息：

```sh
levier app show --credentials ~/.config/levier/counter-agent.json
levier app list
```

轮换 App key：

```sh
levier app rotate-key \
  --credentials ~/.config/levier/counter-agent.json \
  --yes
```

轮换会立即让旧 key 失效，更新凭据文件并保留受保护的 `.bak` 备份。轮换后重新运行 `app check`。

部署级凭据的轮换见 [`LIFECYCLE.md`](LIFECYCLE.md)。

---

## 6. 常见问题

如果 `app init` 提示凭据已存在：

```text
确认路径是否正确
需要覆盖时使用 --force
需要轮换已有 App 的 key 时使用 app rotate-key
```

如果 `app check` 提示文件权限错误：

```sh
chmod 600 ~/.config/levier/counter-agent.json
```

如果服务不可达，先运行：

```sh
levier selfcheck --server http://127.0.0.1:8080
```

如果 App 已创建但 key 没有写入，保留命令输出并重新轮换；不要删除数据库或凭据卷来“重新开始”。
