# 从零开始接入 Levier（详细版）

> **第一次接入请看 [`QUICKSTART.md`](QUICKSTART.md)。**
>
> 本文保留为深入解释和排错手册。它刻意解释了凭据、secret、token 的区别；
> 这些不是黄金路径要求用户理解的内容。
>
> 注意：本文中的 `levier provider ...` 示例是历史入口，现已统一为
> `levier model ...`；`levier app ...` 仅保留给内部/自动化脚本。

这是一份给第一次接触这个项目的人看的操作指南。

你不需要先理解 Levier 的内部结构，也不需要读其他设计文档。我们一页只做一件事。

---

# 第 1 页：先跑一次，不接自己的项目

## 这一页结束时，你会得到什么

你会用项目自带的 Echo Runtime 跑一次最简单的任务：

```text
输入：hello
输出：hello
```

它不会调用模型，不会调用工具，也不会涉及 MCP。  
目的只是确认：

- Levier 控制面能启动
- App 凭据能创建
- 一个 Run 能提交
- 你能看到真实的运行结果

---

## 1. 安装 CLI，让它从任何目录都能用

CLI 只需要安装一次。安装完成后，你不再需要进入仓库根目录才能执行 `levier`。

### 方式 A：从仓库安装（开发时推荐）

先进入你 clone 下来的仓库：

```powershell
cd D:\Workspace\Code\middleware
make install
```

确认新终端里能找到命令：

```powershell
cd $env:TEMP
levier version
```

如果刚安装后当前终端仍找不到 `levier`，按 `make install` 输出的 PATH hint 操作，或者打开一个新的 PowerShell 窗口再试。

### 方式 B：安装发布版

Linux/macOS：

```sh
curl -fsSL \
  https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.sh \
  -o install.sh
sh install.sh
```

Windows PowerShell：

```powershell
Invoke-WebRequest `
  -Uri https://raw.githubusercontent.com/fluffyTighnari/levier/main/scripts/install.ps1 `
  -OutFile "$env:TEMP\levier-install.ps1"
powershell -NoProfile -ExecutionPolicy Bypass -File "$env:TEMP\levier-install.ps1"
```

安装器会：

- 下载并校验 CLI
- 把安装目录加入用户 PATH
- 在可用时安装 Compose 源码包
- 在找到 Python 时安装 App SDK（`from levier_app import LevierApp`）

安装器会提示 PATH 已更新。请打开一个新的终端，或者执行安装器打印的
`export PATH=...`，再从任意目录继续。

### 方式 C：暂时不安装

如果你只是想先在仓库里试一次，也可以继续使用：

```powershell
.\bin\levier.exe
```

Linux/macOS 对应：

```sh
./bin/levier
```

下面统一写 `levier`。使用方式 C 时，把它替换成你实际使用的路径。

---

## 2. 写全局配置

在任意目录执行一次：

```powershell
levier setup --server http://127.0.0.1:8080
```

它会写入用户级配置，不需要放在业务项目里：

```text
Windows  %APPDATA%\levier\config.yaml
Linux    ~/.config/levier/config.yaml
macOS    ~/Library/Application Support/levier/config.yaml
```

后面的 `status`、`app init`、`run submit` 都会自动读取这个配置。  
Admin token 不会写进这个文件；需要时用 `LEVIER_ADMIN_TOKEN` 环境变量提供。

查看当前配置：

```powershell
levier setup --show
```

隔离测试可以用 `LEVIER_HOME` 指定另一份配置目录。

---

## 3. 启动 Levier 和内置 Runtime

前提：电脑上已经安装了 **Docker Desktop**，并且 Docker Desktop 正在运行。

### 如果当前在仓库根目录

```powershell
docker compose up -d postgres levier mcp-runtime react-runtime echo-runtime
```

### 如果在任意业务目录

安装器已经准备好 Compose 源码包时：

```powershell
levier stack up --build
```

如果安装时使用了 `--no-stack`，或者你只 clone 了仓库，先执行一次：

```powershell
levier setup --stack-file D:\Workspace\Code\middleware\docker-compose.yml
levier stack up --build
```

这条命令会启动：

| 服务 | 作用 |
|---|---|
| `postgres` | 保存 Run、App、事件等数据 |
| `levier` | 控制面，负责接收请求和调度 |
| `mcp-runtime` | MCP 工具进程，这一页暂时不用 |
| `react-runtime` | 内置 ReAct Runtime，这一页暂时不用 |
| `echo-runtime` | 内置 Echo Runtime，这一页就用它 |

第一次启动可能需要等待几十秒。  
如果 Docker Desktop 刚打开，先等它完全启动再执行命令。

检查是否启动成功：

```powershell
levier status
```

在 Linux/macOS 上同样是：

```sh
levier status
```

你应该看到类似：

```text
server      http://127.0.0.1:8080
status      ok
health      ok
ready       ok
database    up
daemons     connected=2 healthy=2 capacity=8 used=0 available=8
```

重点是：

```text
status      ok
health      ok
ready       ok
```

如果这里是 `fail`，先不要继续。把完整输出发给维护者。

---

## 4. 记住三个地址和名称

这一页只需要关心三个东西：

```text
HTTP 地址：http://127.0.0.1:8080
内置 Echo Runtime 名称：echo1
管理员 token：admin-change-me
```

它们分别是什么：

| 名称 | 含义 |
|---|---|
| `http://127.0.0.1:8080` | 你提交 Run、查询状态时访问的控制面地址 |
| `echo1` | 一个已经启动的内置 Runtime 名字，它会原样回复用户输入 |
| `admin-change-me` | 开发环境默认管理员 token，用来创建 App 凭据 |

`admin-change-me` 是示例默认值。  
如果你们的部署修改了它，就要换成实际值。

---

## 5. 创建一个 App 凭据

执行：

```powershell
levier app init `
  --name my-first-app `
  --server http://127.0.0.1:8080 `
  --admin-token admin-change-me `
  --credentials-out .\data\my-first-app.json `
  --json
```

Linux/macOS 使用 `\` 换行：

```sh
levier app init \
  --name my-first-app \
  --server http://127.0.0.1:8080 \
  --admin-token admin-change-me \
  --credentials-out ./data/my-first-app.json \
  --json
```

### 每个参数是什么意思

| 参数 | 含义 |
|---|---|
| `app init` | 在 Levier 里创建一个 App 身份 |
| `--name my-first-app` | App 名字，可以理解成“这个业务系统的名字” |
| `--server http://127.0.0.1:8080` | 控制面地址 |
| `--admin-token admin-change-me` | 创建 App 需要管理员权限 |
| `--credentials-out .\data\my-first-app.json` | 把 App 的凭据写到这个文件 |
| `--json` | 用 JSON 输出，方便机器读取 |

### `--credentials` 是什么

注意：这里的创建参数叫：

```text
--credentials-out
```

后面提交 Run 时使用的参数叫：

```text
--credentials
```

区别是：

```text
--credentials-out   创建并写出凭据文件
--credentials       读取已有的凭据文件
```

创建成功后，你应该看到类似：

```json
{
  "app_id": "app_...",
  "app_name": "my-first-app",
  "server": "http://127.0.0.1:8080",
  "credentials": "D:\\Workspace\\Code\\middleware\\data\\my-first-app.json",
  "created": true
}
```

同时在项目目录下会出现：

```text
data/my-first-app.json
```

这个文件里面有一把 App API key。  
**不要把这个文件提交到 Git，不要发到聊天工具里，也不要把 key 打印到日志里。**

这一页先不用打开它。下一頁我们专门解释它和环境变量、secret 文件的区别。

---

## 6. 提交第一次 Run

现在执行：

```powershell
levier run submit `
  --credentials .\data\my-first-app.json `
  --runtime echo1 `
  --prompt "hello" `
  --watch
```

### 参数是什么意思

| 参数 | 含义 |
|---|---|
| `--credentials` | 使用刚才创建的 App 凭据 |
| `--runtime echo1` | 把任务交给内置 Echo Runtime |
| `--prompt "hello"` | 用户输入 |
| `--watch` | 提交后一直等到 Run 结束 |

你应该看到类似：

```text
run submitted: 01...
status: queued
status: completed
runtime_name: echo1
final: hello
```

这表示：

```text
Run 已提交 -> 被 echo1 接走 -> 执行完成 -> 最终回复是 hello
```

---

## 7. 看看这次 Run 发生了什么

查看刚刚的 Run：

```powershell
levier run show `
  --credentials .\data\my-first-app.json `
  --run-id 01... `
  --events
```

把 `01...` 换成上一步输出的真实 run id。

你会看到类似：

```text
run_id       01...
status       completed
runtime      echo1
[1] RUN_STATUS             daemon
[2] MESSAGE_COMPLETED      daemon
[3] RUN_STATUS             daemon
```

这三个事件的含义：

| 事件 | 含义 |
|---|---|
| `RUN_STATUS` | Run 开始或结束 |
| `MESSAGE_COMPLETED` | Runtime 产生了一条完整回复 |
| `RUN_STATUS` | Run 最终状态 |

Echo Runtime 不调用模型，所以这里没有 `TOOL_CALL_*` 或 `USAGE`。

---

## 8. 看日志

如果结果和预期不一致，可以看日志：

```powershell
docker compose logs --tail=100 levier
docker compose logs --tail=100 echo-runtime
```

排查顺序：

```text
1. levier status 是否 ok
2. app check 是否 PASS
3. run show 的 status 是什么
4. levier 和对应 runtime 的日志
```

检查 App：

```powershell
levier app check `
  --credentials .\data/my-first-app.json
```

应该看到：

```text
app check: PASS
```

---

## 这一页你要记住的三件事

```text
1. App 凭据是一个文件，不需要写进代码。
2. --credentials-out 负责创建它，--credentials 负责读取它。
3. 这一页不需要模型 key，因为 echo1 不调用模型。
```

---

## 如果你卡住了

请把下面三项原样发给维护者，不要自己猜：

```text
1. 你执行的那条完整命令
2. 终端完整输出
3. levier status 的输出
```

下一頁我们会做一件更接近真实业务的事：

> 打开 `data/my-first-app.json`，解释里面每个字段，并区分 App API key、环境变量和模型服务商 secret。

---

# 第 2 页：四个东西不要混

这一页只讲概念，但每一个都解释到底。

读完这一页，你应该能回答：

```text
App 凭据文件是什么？
--credentials 和 LEVIER_CREDENTIALS 是什么关系？
环境变量为什么不直接放 API key？
模型服务商的 key 放在哪里？
Runtime 的 token 又是什么？
```

---

## 1. 先打开你刚才创建的凭据文件

Windows PowerShell：

```powershell
Get-Content .\data/my-first-app.json
```

Linux/macOS：

```sh
cat ./data/my-first-app.json
```

你看到的内容类似这样（key 已用 `***` 代替）：

```json
{
  "credential_version": 1,
  "app_id": "app_01...",
  "app_name": "my-first-app",
  "api_key": "mw_***",
  "server": "http://127.0.0.1:8080",
  "created_at": "2026-11-20T00:00:00Z",
  "updated_at": "2026-11-20T00:00:00Z"
}
```

### 每个字段是什么

| 字段 | 含义 | 能不能公开 |
|---|---|---|
| `credential_version` | 文件格式版本，当前是 `1` | 可以 |
| `app_id` | App 的公开 ID | 可以 |
| `app_name` | App 名字 | 可以 |
| `api_key` | App 的访问密钥，用来证明“我是这个 App” | **不能** |
| `server` | 控制面地址，例如 `http://127.0.0.1:8080` | 可以 |
| `created_at` / `updated_at` | 创建和更新时间 | 可以 |

### 最重要的区别

这个文件里的 `api_key` 是：

```text
App API key
```

不是：

```text
模型服务商的 API key
```

它们完全不是一回事。

```text
App API key：App 对 Levier 说话时证明身份的钥匙
模型服务商 key：Levier 的 Runtime 调 DeepSeek/OpenAI/Claude 时使用的钥匙
```

App 只拿自己的 key，不接触模型服务商 key。

---

## 2. 为什么用文件，不直接写在代码里

如果你把 `api_key` 写在 Python 代码里：

```python
API_KEY = "mw_真实key"   # 不要这样做
```

问题：

- 很容易被提交到 Git
- 容易被日志打印
- 容易在多人之间复制
- 修改时还要改代码

所以 SDK 使用一个独立文件：

```text
data/my-first-app.json
```

代码只负责读取文件，不负责保存 key。

文件路径通过参数或环境变量告诉 SDK：

```text
--credentials .\data/my-first-app.json
```

或者：

```text
LEVIER_CREDENTIALS=.\data/my-first-app.json
```

---

## 3. `--credentials-out` 和 `--credentials`

这两个参数很容易混。

```text
--credentials-out    创建新凭据时使用，表示“写到哪里”
--credentials        读取已有凭据时使用，表示“从哪里读”
```

对应动作：

```powershell
# 写入：创建 App 并保存凭据
levier app init `
  --name my-first-app `
  --credentials-out .\data\my-first-app.json

# 读取：用已有 App 提交 Run
levier run submit `
  --credentials .\data\my-first-app.json `
  --runtime echo1 `
  --prompt "hello"
```

你可以把它理解成：

```text
--credentials-out = 办卡
--credentials     = 刷卡
```

---

## 4. `LEVIER_CREDENTIALS` 是什么

`LEVIER_CREDENTIALS` 不是 key 本身，它只是一个“默认文件路径”。

设置它以后，大部分命令可以省略 `--credentials`。

PowerShell：

```powershell
$env:LEVIER_CREDENTIALS = ".\data\my-first-app.json"
```

cmd：

```cmd
set LEVIER_CREDENTIALS=.\data/my-first-app.json
```

Linux/macOS：

```sh
export LEVIER_CREDENTIALS=./data/my-first-app.json
```

验证：

```powershell
levier app check
```

如果成功，就不需要每次都写 `--credentials`。

注意：上面的设置只对**当前终端窗口**有效。新开一个终端，需要重新设置。

### 优先级

```text
命令行显式 --credentials
    > LEVIER_CREDENTIALS
    > 系统默认凭据位置
```

如果显式参数和环境变量同时存在，以 `--credentials` 为准。

---

## 5. 环境变量到底是什么

环境变量可以理解成：

```text
给程序看的临时配置纸条
```

它适合放：

- 服务地址
- 日志级别
- Runtime 名字
- secret 文件的路径

它不适合放：

- 长期使用的 App key
- 可以复制传播的密码
- 需要被严格权限保护的文件内容

### 常用的三类

| 名称 | 作用 | 是否秘密 |
|---|---|---|
| `LEVIER_CREDENTIALS` | App 凭据文件路径 | 否，路径本身不秘密 |
| `LEVIER_SERVER` | 控制面 HTTP 地址 | 否 |
| `LEVIER_SECRET_DEEPSEEK_API_KEY` | 控制面读取的模型 key | **是** |
| `LEVIER_SECRETS_FILE` | 控制面读取的 secret 文件路径 | 否，路径本身不秘密 |

注意：

```text
.env 文件不会自动出现在你的 PowerShell 里。
```

`.env` 主要给 Docker Compose 使用。  
如果你在 PowerShell 里运行 `levier`，需要显式设置环境变量，或者使用命令行参数。

---

## 6. 模型服务商 key 放在哪里

当你要使用 React Runtime 和真实模型时，需要模型服务商 key。

它不是 App key，也不应该写进 App 凭据文件。

它由控制面读取，有两种方式。

### 方式 A：环境变量

```powershell
$env:LEVIER_SECRET_DEEPSEEK_API_KEY = "sk-..."
```

环境变量名由两部分组成：

```text
LEVIER_SECRET_ + 大写的 secret_ref
```

如果 provider 的 `secret_ref` 是：

```text
deepseek_api_key
```

那么控制面读取：

```text
LEVIER_SECRET_DEEPSEEK_API_KEY
```

### 方式 B：secret 文件

创建一个 JSON 文件，例如：

```json
{
  "deepseek_api_key": "sk-..."
}
```

然后在启动控制面时设置：

```powershell
$env:LEVIER_SECRETS_FILE = ".\model-secrets.json"
```

Linux/macOS 上，secret 文件要求只有当前用户可以读：

```sh
chmod 600 ./model-secrets.json
```

Windows 的权限模型不同，但仍然必须把这个文件当秘密保管，不要提交到 Git。

### 然后注册 provider

```powershell
levier provider add `
  --name deepseek `
  --adapter openai_compatible `
  --base-url https://api.deepseek.com `
  --default-model deepseek-flash `
  --secret-ref deepseek_api_key `
  --default
```

这里：

```text
--secret-ref deepseek_api_key
```

只是告诉控制面“去哪个名字下面找 key”。

它不会把 key 写进 provider 配置，也不会打印 key。

检查：

```powershell
levier provider check --name deepseek
```

---

### 6.1 最容易犯错的地方：`--secret-ref` 不是 key

这是目前最容易导致：

```text
error: model provider operation failed (INTERNAL)
```

的操作。

正确：

```text
--secret-ref deepseek_api_key
```

错误：

```text
--secret-ref sk-xxxxxxxxxxxxxxxx
```

区别是：

```text
secret_ref：一个名字，用来查找 secret
API key：真正的 sk-... 值
```

`secret_ref=deepseek_api_key` 的含义是：

```text
请控制面去 LEVIER_SECRET_DEEPSEEK_API_KEY 里找真正的 key
```

而不是：

```text
请把 sk-... 存进 provider 记录
```

如果你把 `sk-...` 填进了 `--secret-ref`，provider 记录里就会保存一个看起来像
key 的字符串，但控制面会把它当成“名字”去查环境变量，结果找不到。

你可以用下面的命令检查：

```powershell
levier provider show --name deepseek
```

正常应该看到：

```text
name              deepseek
adapter           openai_compatible
base_url          https://api.deepseek.com
default_model     deepseek-flash
secret_ref        deepseek_api_key
secret_configured true
enabled           true
default           true
```

如果看到：

```text
secret_ref        sk-...
secret_configured false
```

说明 `--secret-ref` 填错了。

### 6.2 修复已经填错的 provider

不用删除 provider，直接重新 upsert 一遍：

```powershell
levier provider add `
  --name deepseek `
  --adapter openai_compatible `
  --base-url https://api.deepseek.com `
  --default-model deepseek-flash `
  --models deepseek-flash,deepseek-reasoner `
  --secret-ref deepseek_api_key `
  --default
```

然后：

```powershell
levier provider check --name deepseek
```

应该看到：

```text
provider check: PASS deepseek
```

### 6.3 `secret_configured false` 怎么修

它表示：

```text
provider 记录存在
但控制面找不到 secret_ref 对应的值
```

这种情况下，key 必须出现在**控制面进程**能读到的环境里，或者控制面的
`LEVIER_SECRETS_FILE` 里。

#### 用 Docker Compose 时

把 key 写进项目根目录的 `.env`：

```text
LEVIER_SECRET_DEEPSEEK_API_KEY=sk-...
```

然后重建控制面容器，让新环境变量进入容器：

```powershell
docker compose up -d --force-recreate levier
```

不要只重新执行 `provider add`，因为已经运行的容器不会自动获得新的环境变量。

#### 用本地控制面进程时

在启动控制面的同一个终端里：

```powershell
$env:LEVIER_SECRET_DEEPSEEK_API_KEY = "sk-..."
```

然后重启控制面进程。

#### 用 secret 文件时

```json
{
  "deepseek_api_key": "sk-..."
}
```

```powershell
$env:LEVIER_SECRETS_FILE = ".\model-secrets.json"
```

同样需要重启控制面。

### 6.4 常见错误对照

| 输出 | 含义 | 先做什么 |
|---|---|---|
| `admin token is required` | 没有管理员 token | 设置 `LEVIER_ADMIN_TOKEN` |
| `UNAUTHORIZED` | 管理员 token 错误 | 检查 `.env` 里的 admin token |
| `MODEL_PROVIDER_NOT_FOUND` | provider 还没注册 | 执行 `provider add` |
| `BAD_MODEL_PROVIDER` | adapter、model 或 secret_ref 不合法 | 看 `provider show` |
| `secret_configured false` | 控制面找不到 key | 检查环境变量或 secret 文件 |
| `model provider operation failed (INTERNAL)` | 控制面旧版本，或 secret 配置方式不对 | 先执行 `provider show`，再检查容器是否旧版本 |

如果是旧容器，必须重建控制面镜像/容器，而不是继续改 provider 记录：

```powershell
docker compose up -d --build levier
```

---

## 7. Runtime 的 token 是什么

Runtime 连接 Levier 时也需要认证，但使用的是另一套 token：

```text
Runtime daemon token
```

它通常写在 Runtime 配置里：

```yaml
middleware:
  address: "127.0.0.1:9090"
  auth_token: "change-me"
```

或者由环境变量提供：

```text
LEVIER_RUNTIME_TOKEN
```

它和 App API key 的区别：

```text
App API key       -> 证明“我是哪个 App”
Runtime token     -> 证明“我是哪个 Runtime 实例”
模型 provider key -> 让 Runtime 调用模型服务商
```

三者不要互换。

---

## 8. 做一个小练习

先查看凭据信息（不会显示 key）：

```powershell
levier app show --credentials .\data/my-first-app.json
```

你应该看到：

```text
app_id              app_...
name                my-first-app
status              active
credentials         ...\data\my-first-app.json
key_fingerprint     ...
```

然后设置默认凭据路径：

```powershell
$env:LEVIER_CREDENTIALS = ".\data\my-first-app.json"
```

运行一次不带 `--credentials` 的 Echo Run：

```powershell
levier run submit `
  --runtime echo1 `
  --prompt "hello again" `
  --watch
```

如果它成功，说明你已经理解：

```text
--credentials 可以显式传入
LEVIER_CREDENTIALS 可以代替它
App 凭据文件不是模型 key
```

---

## 9. 这一页最容易犯的错

### 错误 1：把 `api_key` 当成 `sk-...`

不对。

```text
data/my-first-app.json 里的 api_key 是 App 身份
sk-... 是模型服务商 key
```

### 错误 2：把 `--credentials-out` 和 `--credentials` 用反

不对。

```text
创建 App 用 --credentials-out
提交 Run 用 --credentials
```

### 错误 3：把 key 写进 Git

不要。

```text
data/my-first-app.json
.env
model-secrets.json
```

都不应该提交。

### 错误 4：以为 `.env` 会自动被 PowerShell 读取

不会。

`.env` 主要是 Docker Compose 使用。  
在 PowerShell 里运行 CLI 时，需要设置环境变量或传参数。

---

## 10. 如果凭据文件丢了怎么办

先说结论：

```text
旧文件里的 api_key 不能恢复。
凭据文件不能从 app_id 或 app_name 反推出 key。
```

但是可以给同一个 App **换一把新 key**，并重新生成凭据文件。

不要这样做：

```text
levier app rotate-key --credentials ./data/my-first-app.json
```

因为 `rotate-key` 需要先读取旧凭据文件；文件丢了，它没有 App 身份可用。

正确的恢复命令是：

```powershell
levier app init `
  --name my-first-app `
  --server http://127.0.0.1:8080 `
  --admin-token admin-change-me `
  --credentials-out .\data\my-first-app.json `
  --rotate `
  --yes
```

Linux/macOS：

```sh
levier app init \
  --name my-first-app \
  --server http://127.0.0.1:8080 \
  --admin-token admin-change-me \
  --credentials-out ./data/my-first-app.json \
  --rotate \
  --yes
```

它做三件事：

```text
1. 找到已经存在的 App
2. 给这个 App 轮换一把新 key
3. 把新凭据写到 --credentials-out 指定的路径
```

旧 key 会立即失效。  
如果你还有别的程序在用旧 key，它们会认证失败，需要同步更新。

恢复后检查：

```powershell
levier app check --credentials .\data\my-first-app.json
```

应该看到：

```text
app check: PASS
```

### 为什么 `app init` 第一次没有直接返回 key

因为 App 名字已经存在时，Levier 不会再次返回原始 key。

这是安全设计：

```text
key 只在第一次创建时返回一次。
之后只能显式轮换。
```

所以：

```text
app init --rotate --yes  = 凭据丢失后的恢复方式
app rotate-key           = 已经拿着有效凭据文件时的轮换方式
```

---

## 这一页结束时你应该能说出来

```text
App 凭据文件包含什么
--credentials-out 和 --credentials 的区别
LEVIER_CREDENTIALS 只是路径
模型 key 和控制面 secret 的关系
Runtime token 和 App key 的区别
凭据文件丢失后如何用 app init --rotate 恢复
```

下一页开始接自己的 App：

> 写一个真正属于你的工具，让 React Runtime 调用它，然后用 CLI 再跑一次。

---

# 第 3 页：把 Python 函数变成 App 工具

这一页只做一件事：

> 让你自己的 Python 函数被 React Runtime 调用一次。

先不接 MCP，也不写自己的 Runtime。

---

## 1. 先确认两件事

执行：

```powershell
levier status
```

看到：

```text
health      ok
ready       ok
```

并且确认 React Runtime 在线：

```powershell
levier runtime list
```

应该能看到类似：

```text
runtime              instance             status
react1               daemon-local-1       online
```

如果 React Runtime 不在线，先回到第 1 页启动完整栈。

这一页还需要模型服务商已经配置好。  
如果没有配置，先回到第 2 页完成：

```text
provider add
provider check
```

这一页不会再解释 provider。

---

## 2. 安装 App SDK

如果使用 `install.sh` / `install.ps1` 并且安装时找到了 Python，这一步已经自动完成，可以跳过。

App SDK 现在是独立的 Python wheel，也可以从任意目录手动安装。

### 从仓库开发

使用 SDK 源码目录的绝对路径，不需要当前目录在仓库里：

```powershell
python -m pip install -e C:\Workspace\Code\middleware\sdk\python
```

Linux/macOS：

```sh
python3 -m pip install -e /path/to/middleware/sdk/python
```

### 从 release wheel

下载 release 里的 `levier_app-*.whl` 后，在任意目录执行：

```powershell
python -m pip install levier_app-0.1.0-py3-none-any.whl
```

如果有多个 wheel，放在同一个目录后：

```powershell
python -m pip install --find-links .\levier-wheels levier-app
```

这条命令做的事：

```text
把 levier_app 这个 Python 包安装到当前 Python 环境
```

安装完成后，Python 代码就可以写：

```python
from levier_app import LevierApp
```

App SDK 会自动读取 `levier setup` 写入的全局配置。  
所以不需要在每个业务项目里重复写 server 地址或凭据路径。

---

## 3. 创建自己的 App 文件

在项目根目录创建一个文件：

```text
my_first_app.py
```

内容如下：

```python
from levier_app import LevierApp

counter = {"value": 0}


def register_tools(app):
    @app.tool(
        "counter_add",
        description="Add an integer to the counter.",
        schema={
            "type": "object",
            "properties": {
                "delta": {"type": "integer"}
            },
            "required": ["delta"],
            "additionalProperties": False,
        },
        timeout_ms=10000,
    )
    def counter_add(arguments):
        counter["value"] += arguments["delta"]
        return {"value": counter["value"]}

    @app.tool(
        "counter_get",
        description="Read the current counter value.",
        schema={
            "type": "object",
            "properties": {},
            "additionalProperties": False,
        },
    )
    def counter_get(_arguments):
        return {"value": counter["value"]}


if __name__ == "__main__":
    raise SystemExit(LevierApp.run_cli(register_tools))
```

这一页只需要理解三件事：

```text
app.tool       把一个 Python 函数注册成工具
schema         告诉模型这个工具需要什么参数
handler        真正执行工具的函数
```

---

## 4. `@app.tool` 的参数

```python
@app.tool(
    "counter_add",
    description="Add an integer to the counter.",
    schema={...},
    timeout_ms=10000,
)
```

| 参数 | 含义 |
|---|---|
| `"counter_add"` | 工具名字，必须唯一 |
| `description` | 给模型看的说明，模型用它判断什么时候调用 |
| `schema` | 参数格式，告诉模型和 Levier 需要哪些字段 |
| `timeout_ms` | 最长允许运行多久，单位毫秒 |

### schema 是什么

```python
schema={
    "type": "object",
    "properties": {
        "delta": {"type": "integer"}
    },
    "required": ["delta"],
    "additionalProperties": False,
}
```

它表达的是：

```text
这个工具需要一个对象
对象里有一个 delta
delta 必须是整数
delta 必须有
不能传入其他字段
```

模型看到的是这个 schema，不是你的 Python 函数源码。

---

## 5. 启动自己的 App

在项目根目录执行：

```powershell
python .\my_first_app.py `
  --credentials .\data\my-first-app.json `
  --server http://127.0.0.1:8080
```

Linux/macOS：

```sh
python3 ./my_first_app.py \
  --credentials ./data/my-first-app.json \
  --server http://127.0.0.1:8080
```

注意：这个终端要一直开着。

这个命令做的是：

```text
读取 App 凭据
连接 Levier
注册 counter_add 和 counter_get 两个工具
等待工具调用
```

如果输出类似：

```text
connected
```

说明 App 已经连上控制面。

---

## 6. 让 React Runtime 调用你的工具

另开一个终端，在项目根目录执行：

```powershell
levier run submit `
  --credentials .\data\my-first-app.json `
  --runtime react1 `
  --provider deepseek `
  --model deepseek-flash `
  --prompt "Add 2, then add 3, then read the counter." `
  --watch
```

如果控制面已经设置了默认 provider，也可以省略：

```text
--provider
--model
```

运行成功后，你应该看到类似：

```text
run submitted: 01...
status: queued
status: running
status: completed
runtime_name: react1
final: 5
```

这里的 `final: 5` 来自你的 App 工具：

```text
模型调用 counter_add(2)
模型调用 counter_add(3)
模型调用 counter_get()
你的 handler 返回 5
模型把 5 作为最终回复
```

---

## 7. 确认工具真的被调用了

执行：

```powershell
levier run show `
  --credentials .\data\my-first-app.json `
  --run-id 01... `
  --events
```

把 `01...` 换成真实 run id。

你应该看到包含：

```text
TOOL_CALL_BEGIN
TOOL_CALL_END
```

其中：

| 事件 | 含义 |
|---|---|
| `TOOL_CALL_BEGIN` | 模型请求调用工具 |
| `TOOL_CALL_END` | 工具执行结束 |
| `MESSAGE_COMPLETED` | 模型产生了完整回复 |

如果只看到 `MESSAGE_COMPLETED`，没有 `TOOL_CALL_*`，通常是：

```text
模型没有选择调用工具
或者 App 没有注册成功
或者 prompt 没有表达清楚
```

先看 `run show` 的事件，不要靠猜。

---

## 8. 用户代码的边界

你的 `my_first_app.py` 只负责：

```text
业务状态：counter
工具名、description、schema
handler 逻辑
启动入口
```

SDK 负责：

```text
读取凭据
连接控制面
注册工具
接收 local_tool_call
执行 handler
返回 local_tool_result
断线重连
```

你不需要写 WebSocket、HTTP、消息序列号或重连逻辑。

---

## 9. 这一页最常见的错误

### 错误 1：App 没有启动

现象：

```text
TOOL_NOT_FOUND
APP_UNREACHABLE
```

处理：

```text
确认运行 my_first_app.py 的终端没有关闭
确认它输出了 connected
```

### 错误 2：工具名不一致

例如 App 注册的是：

```text
counter_add
```

但 prompt 或模型期望的是：

```text
add
```

工具名必须和 `@app.tool("...")` 完全一致。

### 错误 3：schema 写错

如果模型传的参数不符合 schema，工具可能不会被调用，或者 handler 会拿到空参数。

先检查：

```python
arguments
```

到底是什么，可以临时打印它。

### 错误 4：handler 返回了不能 JSON 序列化的对象

必须返回：

```text
dict
list
str
int
float
bool
None
```

不要直接返回自定义类实例。

### 错误 5：模型没有工具能力或 provider 没配好

如果 provider 检查失败，先回到第 2 页。

---

## 这一页结束时你应该能说出来

```text
@ app.tool 的三个核心部分：名字、description、schema
handler 接收 arguments 字典
工具必须由 App 进程提供
React Runtime 只是决定什么时候调用工具
模型不会直接执行 Python 函数
run show --events 能看到真实的工具调用
```

下一頁：

> MCP 工具和本地工具有什么区别，`mcp_refs` 到底怎么传。

---

# 第 4 页：MCP 工具怎么接

这一页的完整真实例子在：

[`MCP_EXAMPLE.md`](MCP_EXAMPLE.md)

请先按那个文件从第 1 步走到第 7 步：

```text
注册 mcp-echo
App 声明 mcp-echo
启动 App
提交一次明确调用 MCP 的 Run
run show --events
```

它使用项目里已经存在的：

```text
examples/mcp_echo/server.py
examples/mcp_echo/config.json
```

不需要你写 MCP client，也不需要你猜环境变量放在哪里。

MCP 例子跑通后，再回到第 5 页看已有项目怎么改。

---

# 第 5 页：已有 Python 项目怎么接

这一页只回答一个问题：

> 我已经有一个业务项目，不想重写它，应该改动哪里？

先给结论：

```text
不要重写业务代码
不要写 WebSocket
不要写 client.py
新增一个很薄的 adapter 文件
让业务函数变成工具
```

---

## 1. 最小改动清单

一个已有 Python 项目通常只需要改四件事：

```text
1. 安装 levier_app
2. 新增一个 adapter 文件
3. 设置 App 凭据路径
4. 用一个新进程运行 adapter
```

业务项目原本的模块、数据库访问、业务逻辑都保留。

---

## 2. 假设你原来有这个项目

例如：

```text
myproject/
  orders.py
  inventory.py
  main.py
```

`orders.py` 里有：

```python
def find_order(order_id: str) -> dict:
    ...
```

`inventory.py` 里有：

```python
def get_stock(sku: str) -> int:
    ...
```

这些函数不需要改。

---

## 3. 新增一个 adapter 文件

在项目里新增：

```text
myproject/levier_adapter.py
```

内容：

```python
from levier_app import LevierApp
from myproject.orders import find_order
from myproject.inventory import get_stock


def register_tools(app):
    @app.tool(
        "find_order",
        description="Find one order by order id.",
        schema={
            "type": "object",
            "properties": {
                "order_id": {"type": "string"}
            },
            "required": ["order_id"],
            "additionalProperties": False,
        },
    )
    def find_order_tool(arguments):
        return find_order(arguments["order_id"])

    @app.tool(
        "get_stock",
        description="Read available stock for one SKU.",
        schema={
            "type": "object",
            "properties": {
                "sku": {"type": "string"}
            },
            "required": ["sku"],
            "additionalProperties": False,
        },
    )
    def get_stock_tool(arguments):
        return {"stock": get_stock(arguments["sku"])}


if __name__ == "__main__":
    raise SystemExit(LevierApp.run_cli(register_tools))
```

这个文件只做适配：

```text
参数格式转换
调用已有业务函数
把结果变成 JSON 可序列化的值
```

不要把业务逻辑复制到这个文件。

---

## 4. 启动 adapter

先设置凭据：

```powershell
$env:LEVIER_CREDENTIALS = ".\data\my-first-app.json"
```

然后：

```powershell
python -m myproject.levier_adapter
```

如果你不想设置环境变量，也可以：

```powershell
python -m myproject.levier_adapter `
  --credentials .\data\my-first-app.json `
  --server http://127.0.0.1:8080
```

`run_cli` 负责：

```text
解析参数
加载凭据
连接 Levier
注册工具
维持 WebSocket
重连
处理工具调用
```

---

## 5. 已有项目的 HTTP 服务怎么办

现有 HTTP 服务继续作为 HTTP 服务存在。

推荐部署方式：

```text
现有业务服务
    |
    +-- 数据库 / 缓存 / 内部 API

Levier adapter
    |
    +-- 导入现有业务函数
    +-- 连接 Levier
```

adapter 是一个独立进程，不是替代现有 HTTP 服务。

它通常不需要新的入站端口，因为它主动连接控制面。

---

## 6. App SDK 公开面

已有项目只需要记住这些：

### 创建 App

```python
app = LevierApp.from_credentials(
    path=None,
    server=None,
    mcp_refs=None,
)
```

| 参数 | 含义 |
|---|---|
| `path` | 凭据文件；不传则使用 `LEVIER_CREDENTIALS` |
| `server` | 控制面 HTTP 地址；不传则从凭据或环境变量读取 |
| `mcp_refs` | 要使用的 MCP Server 名字；不传则使用 `LEVIER_APP_MCP_REFS` |

### 注册工具

```python
@app.tool(
    name,
    description=...,
    schema=...,
    timeout_ms=...,
)
```

### 启动

```python
LevierApp.run_cli(register_tools)
```

### Run 操作

```python
app.list_model_providers()
app.submit_run(prompt, runtime="react1", provider=..., model=...)
app.wait_run(run_id)
app.get_run(run_id)
app.get_events(run_id)
app.cancel_run(run_id)
```

`app.submit_run` 使用的凭据和控制面地址与 `LevierApp` 相同。

---

## 7. 不要自己写的东西

业务项目里不要再写：

```text
WebSocket 客户端
local_tool_call 消息循环
local_tool_result 封装
run submit 的 HTTP 请求
轮询 Run 状态
客户端重连
```

这些已经在 `levier_app` 里。

也不要新增 `client.py`、`submit_run.py`、`bootstrap.py` 这种启动脚本。

---

## 8. 高并发项目注意事项

当前 App SDK 的 tool handler 是同步函数。

不要在一个 handler 里做几秒以上的阻塞操作：

```python
def slow_tool(arguments):
    time.sleep(30)   # 会让当前 App 进程无法处理其他工具调用
    return ...
```

推荐模式：

```text
handler 立即返回 job_id
后台 worker 执行长任务
另一个工具查询 job 状态
```

这也符合“工具调用”的语义：模型需要快速拿到一个结果，而不是等一个长任务结束。

如果你的业务函数本身是 async，需要先确认你们的接入方式；不要直接在同步 handler 里调用
`asyncio.run()`，否则可能和现有事件循环冲突。

---

## 9. MCP 在已有项目里的位置

本地工具：

```text
已有 Python 函数 -> adapter 注册 -> App handler
```

MCP 工具：

```text
已有 MCP Server -> 注册 MCP -> App 声明 mcp_refs -> Runtime 调用
```

已有项目有 MCP Server 时，不要把它复制进 App 进程，也不要在 App 里写 MCP client。
按第 4 页注册 MCP，然后声明引用即可。

---

## 10. 接入检查清单

```text
[ ] 我能独立启动现有业务项目
[ ] 我只新增了一个 adapter 文件
[ ] adapter 可以 import 现有业务函数
[ ] LevierApp.run_cli 能启动并显示 connected
[ ] 我用 CLI 提交的 prompt 能触发本地工具
[ ] run show --events 能看到 TOOL_CALL_BEGIN/END
[ ] 需要 MCP 时，我使用的是 mcp_refs，而不是自己写 MCP client
```

下一頁：

> 自己的 Runtime 怎么写：最小 run(ctx)、什么时候用 ReactAgent、什么时候完全不用框架。

---

# 第 6 页：自己的 Runtime 怎么写

这一页只做一件事：

> 写一个最小的 Runtime，用 CLI 启动它，再提交一次 Run。

不require模型，不require工具。

先安装 Runtime SDK。使用安装器留下的 wheel 目录时，在任意目录执行：

```powershell
python -m pip install --find-links $env:LOCALAPPDATA\levier\python-wheels levier-runtime
```

Linux/macOS 使用 `~/.local/share/levier/python-wheels`（macOS 为
`~/Library/Application Support/levier/python-wheels`）。

也可以直接安装 release 里的 `levier_runtime-*.whl`。

从仓库开发时：

```powershell
python -m pip install -e C:\Workspace\Code\middleware\execution\runtime_sdk
```

---

## 1. 先看最小 Runtime 的代码

在项目根目录创建：

```text
my_runtime.py
```

内容：

```python
from levier_runtime import RunContext, Runtime


async def run(ctx: RunContext) -> str:
    content = "hello from my runtime"
    if ctx.input:
        content = ctx.input[-1].content
    await ctx.reply(f"you said: {content}")
    return "completed"


if __name__ == "__main__":
    Runtime.from_config("runtime.yaml").run(run)
```

这段代码只做：

```text
读取用户输入
通过 ctx.reply 返回一条消息
告诉 Levier：Run completed
```

它没有：

```text
LLM
ReAct
工具调用
MCP
```

这也是理解 Runtime SDK 最好的起点。

---

## 2. 创建 runtime.yaml

在项目根目录创建：

```text
runtime.yaml
```

内容：

```yaml
middleware:
  address: "127.0.0.1:9090"
  auth_token: "change-me"

runtime:
  name: "demo1"
  instance_id: "echo-local-1"
  type: "normal"
  max_concurrent_runs: 1

logging:
  level: "info"
```

### 字段解释

| 字段 | 含义 |
|---|---|
| `middleware.address` | 控制面的 gRPC 地址 |
| `middleware.auth_token` | Runtime 的 daemon token |
| `runtime.name` | App 提交 Run 时使用的名字，例如 `--runtime demo1` |
| `runtime.instance_id` | 这个 Runtime 进程的实例 ID |
| `runtime.type` | 当前使用 `normal` |
| `runtime.max_concurrent_runs` | 这个进程最多同时跑几个 Run |

### 为什么使用 `echo-local-1`

因为项目默认配置里已经有这个实例的 token：

```yaml
grpc:
  daemon_tokens:
    daemon-local-1: "change-me"
    echo-local-1: "change-me"
```

为了不修改控制面配置，这个例子复用了 `echo-local-1`。

如果你已经启动了内置的 Echo 容器，先停掉它，避免实例 ID 冲突：

```powershell
docker compose stop echo-runtime
```

重启时执行：

```powershell
docker compose start echo-runtime
```

如果你使用自己的实例 ID，必须在控制面配置的 `grpc.daemon_tokens` 里增加对应 token。

---

## 3. 启动自己的 Runtime

```powershell
python .\my_runtime.py
```

如果启动成功，终端会保持运行。

检查控制面是否看到它：

```powershell
levier runtime list
```

应该看到：

```text
runtime    instance          status
demo1      echo-local-1      online
```

注意：

```text
runtime.name 是 demo1
instance_id 是 echo-local-1
```

`runtime list` 查的是已经注册成功的 Runtime，不是 Docker 容器。

如果列表里没有 `demo1`，先看 Runtime 终端输出和控制面日志。

---

## 4. 提交一次 Run

另开终端：

```powershell
levier run submit `
  --credentials .\data\my-first-app.json `
  --runtime demo1 `
  --prompt "你好，这是我的 Runtime" `
  --watch
```

不需要 `--provider`，因为这个 Runtime 没有调用模型。

预期：

```text
run submitted: 01...
status: queued
status: completed
runtime_name: demo1
final: you said: 你好，这是我的 Runtime
```

---

## 5. 在 Runtime 里调用 App 工具

如果你想让自己的 Runtime 调用 App 的本地工具：

```python
async def run(ctx: RunContext) -> str:
    result = await ctx.call_tool(
        "counter_add",
        {"delta": 2},
    )
    if not result.ok:
        await ctx.error(result.error_code, result.error_message)
        return "failed"

    await ctx.reply(result.result_json)
    return "completed"
```

SDK 会自动处理：

```text
TOOL_CALL_BEGIN
TOOL_CALL_END
工具超时
工具错误
```

Runtime 作者不需要自己拼事件。

前提是：

```text
App 已连接
工具名和 App 注册的一致
Run 被分配给这个 Runtime
```

App 的本地工具来自 App 的 `@app.tool`。

---

## 6. 想使用 ReactAgent

如果你的 Runtime 想要标准 ReAct 循环，可以复用可选 helper：

```python
from levier_runtime.llm import ProviderRouter
from levier_runtime.react import ReactAgent
from levier_runtime import Runtime

agent = ReactAgent(ProviderRouter())

Runtime.from_config("runtime.yaml").run(agent)
```

ReactAgent 默认通过 `ctx.toolbox()` 访问本地和 MCP 工具，不需要额外的
MCP client 或工具路由层。

这时需要一个配置好的模型 provider。

如果你不想用 ReactAgent，就继续使用自己的 `run(ctx)`。SDK 不会强制你使用它。

---

## 7. Runtime CLI 的边界

内置 Echo 和 React 的启动参数：

```text
--config
--check
--print-config
--version
--log-level
```

自定义 Runtime 不一定要提供这些 CLI 参数。  
最小代码可以直接：

```python
Runtime.from_config("runtime.yaml").run(run)
```

如果你要做生产 Runtime，建议至少支持：

```text
--config
--check
```

这样部署时可以先检查配置，再启动。

---

## 8. 这一页最容易犯的错

### Runtime 没有出现在 `runtime list`

先检查：

```text
runtime.name 是什么
instance_id 是什么
middleware.address 是否正确
auth_token 是否和控制面 daemon token 一致
终端是否真的在运行
```

### Run 一直 queued

可能是：

```text
没有匹配 runtime_name 的 Runtime
Runtime 容量已满
Runtime 没有注册成功
```

先用：

```powershell
levier runtime list
```

### 调 App 工具返回 TOOL_NOT_FOUND

检查：

```text
App 是否在对端运行
@ app.tool 的名字是否一致
App 是否注册并连接了 WebSocket
```

### 调模型报 provider 错误

检查第 2 页的 provider 和 `provider check`。

---

## 这一页结束时你应该能说出来

```text
Runtime 的入口是 run(ctx)
Runtime 用自己的 runtime.yaml 连接控制面
runtime.name 决定 Run 被路由到哪个 Runtime
ctx.reply 返回消息
ctx.call_tool 调用 App 工具
ReactAgent 是可选的，不是必需的
```

下一頁：

> 所有 CLI 命令分别做什么，哪些是日常用的，哪些是运维用的。

---

# 第 7 页：CLI 速查和排障

这一页不是从头讲，而是放在手边查。

---

## 1. 先分清两种身份

| 身份 | 用什么 | 能做什么 |
|---|---|---|
| App 身份 | `--credentials` 文件 | 提交 Run、查看自己的 Run |
| Admin 身份 | `--admin-token` | 创建 App、管理 provider、管理 MCP、查看所有 Run |

不要混：

```text
App key 不能创建 App
Admin token 不能代替 App 提交 Run
```

---

## 2. 日常最常用的命令

```text
levier setup
levier version
levier status
levier doctor
levier app init
levier app check
levier run submit
levier run watch
levier run show
levier runtime list
levier stack up
```

### `levier setup`

```powershell
levier setup --server http://127.0.0.1:8080
levier setup --show
levier setup --stack-file C:\Workspace\Code\middleware\docker-compose.yml
```

写一次用户级配置。之后换到任意业务目录，`--server`、`--credentials`、
`--runtime-config`、`--mcp-socket` 都不需要反复传。

`setup` 不会保存 Admin token，也不会保存 App API key。

### `levier stack up`

```powershell
levier stack up --build
levier stack ps
levier stack logs --follow
levier stack down
```

使用 `setup --stack-file` 保存的文件，或者当前目录的 `docker-compose.yml`。  
命令会自动使用 Compose 文件所在目录作为项目目录，所以业务项目不需要复制 Dockerfile。

### `levier version`

```powershell
levier version
```

看 CLI 版本和平台。

不需要 token。

### `levier status`

```powershell
levier status
```

看控制面整体健康：

```text
HTTP 是否健康
数据库是否可用
Runtime 数量
App WebSocket 数量
Run 状态统计
```

适合先判断“系统是不是活着”。

### `levier doctor`

```powershell
levier doctor `
  --credentials .\data\my-first-app.json
```

比 `status` 更细，会检查：

```text
配置文件
数据库连接
数据库迁移
HTTP health/ready
服务元信息
Admin 权限
App 凭据
```

注意：`doctor` 是诊断命令，不是日常提交任务命令。

### `levier runtime list`

```powershell
levier runtime list
```

看已注册成功的 Runtime：

```text
react1
demo1
echo1
```

它不显示 Docker 容器列表。  
容器 running 但 Runtime 没有成功注册时，这里不会出现。

### `levier app init`

```powershell
levier app init `
  --name my-first-app `
  --credentials-out .\data\my-first-app.json `
  --mcp-ref mcp-echo
```

需要 Admin token。

它做的是：

```text
创建 App 身份
生成 App API key
写凭据文件
可选：声明 MCP 引用
```

`--credentials-out` 是写入路径。  
后续命令用 `--credentials` 读取同一个文件。

### `levier app check`

```powershell
levier app check `
  --credentials .\data\my-first-app.json
```

用 App 自己的 key 检查：

```text
凭据格式
App 状态
WebSocket token 是否可申请
```

不检查模型 provider，也不检查 Runtime 是否在线。

### `levier run submit`

```powershell
levier run submit `
  --credentials .\data\my-first-app.json `
  --runtime react1 `
  --provider deepseek `
  --model deepseek-flash `
  --prompt "Add 2, then add 3." `
  --watch
```

需要 App 凭据。

常用参数：

| 参数 | 含义 |
|---|---|
| `--credentials` | App 凭据文件 |
| `--runtime` | 目标 Runtime 名字 |
| `--prompt` | 用户输入 |
| `--provider` | 模型服务商名字；有默认 provider 时可省略 |
| `--model` | 模型名字；有默认模型时可省略 |
| `--watch` | 等到终态 |
| `--json` | 输出机器可读 JSON |

### `levier run watch`

```powershell
levier run watch `
  --run-id <id> `
  --credentials .\data\my-first-app.json
```

如果你已经提交过 Run，只等它结束。

注意：

```text
run watch --run-id <id>
```

不是：

```text
run watch --run-id <id> --prompt ...
```

`watch` 不需要 prompt。

### `levier run show`

```powershell
levier run show `
  --run-id <id> `
  --credentials .\data\my-first-app.json `
  --events
```

查看状态和事件。

`--events` 会显示：

```text
RUN_STATUS
MESSAGE_COMPLETED
TOOL_CALL_BEGIN
TOOL_CALL_END
ERROR
```

这是排查“模型有没有调用工具”的第一入口。

---

## 3. Provider 命令

```text
levier provider list
levier provider show
levier provider add
levier provider update
levier provider remove
levier provider check
```

### 查看

```powershell
levier provider list
levier provider show --name deepseek
```

### 创建或更新

```powershell
levier provider add `
  --name deepseek `
  --adapter openai_compatible `
  --base-url https://api.deepseek.com `
  --default-model deepseek-flash `
  --models deepseek-flash,deepseek-reasoner `
  --secret-ref deepseek_api_key `
  --default
```

### 检查

```powershell
levier provider check --name deepseek
```

### 删除

```powershell
levier provider remove `
  --name deepseek `
  --yes
```

Provider 命令需要 Admin token。

`--secret-ref` 是名字，不是 key：

```text
正确：deepseek_api_key
错误：sk-...
```

---

## 4. MCP 命令

```text
levier mcp list
levier mcp add
levier mcp remove
```

### 注册

```powershell
levier mcp add `
  --name mcp-echo `
  --transport stdio `
  --config-file .\examples\mcp_echo\config.json `
  --allowed-tools echo_text,add_numbers
```

### 查看

```powershell
levier mcp list
```

### 删除

```powershell
levier mcp remove `
  --name mcp-echo `
  --yes
```

MCP 命令需要 Admin token。

`mcp list` 成功不等于 MCP Server 启动成功；要看：

```powershell
docker compose logs mcp-runtime
```

---

## 5. App 命令

```text
levier app init
levier app show
levier app check
levier app list
levier app rotate-key
```

### `app show`

```powershell
levier app show `
  --credentials .\data\my-first-app.json
```

看 App 元数据和凭据指纹。  
不会显示 App key 原文。

### `app list`

```powershell
levier app list
```

需要 Admin token。看所有 App。

### `app rotate-key`

```powershell
levier app rotate-key `
  --credentials .\data\my-first-app.json `
  --yes
```

需要：

```text
Admin token
有效的旧凭据文件
```

如果凭据文件丢了，不能用这个命令，使用：

```powershell
levier app init `
  --name my-first-app `
  --credentials-out .\data\my-first-app.json `
  --rotate `
  --yes
```

---

## 6. 系统命令

### `levier serve`

```powershell
levier serve --config .\levier.example.yaml
```

启动控制面。  
如果使用 Docker Compose，控制面已经由 `levier` 容器启动，通常不需要手动执行。

### `levier doctor`

用于排查：

```text
配置
数据库
迁移
HTTP
Admin token
App 凭据
```

### `levier status`

用于快速看整体健康。

---

## 7. 运维命令

这些命令不是接 App 的日常路径，放在这里备查：

```text
levier admin ops ...
levier admin migrate ...
levier admin maintenance ...
levier admin config ...
levier admin daemon ...
levier admin describe
levier admin selfcheck
```

常用：

```powershell
# 数据库迁移状态
levier admin migrate status

# 执行迁移
levier admin migrate up

# 保留策略检查
levier admin maintenance status

# dry-run 清理
levier admin maintenance prune --dry-run
```

旧的顶层形式还兼容部署脚本，但新脚本应使用 `admin` 前缀。

---

## 8. 退出码

| 退出码 | 含义 |
|---|---|
| `0` | 成功 |
| `1` | 操作失败 |
| `2` | 参数错误 |
| `3` | 认证/授权失败 |
| `4` | 上游或可重试错误 |
| `5` | 拒绝或超时 |

脚本里判断成功优先看退出码，再看输出。

---

## 9. 排障决策表

| 现象 | 先查 |
|---|---|
| 命令找不到 | 新终端执行 `levier version`；检查安装器是否更新了 PATH |
| `status` 不健康 | Docker 是否启动，`docker compose logs levier` |
| `app init` 说 App exists | App 已存在；要新 key 用 `app init --rotate --yes` |
| `app check` 失败 | 凭据路径、文件是否存在、App 是否 active |
| `provider check` 失败 | `provider show` 的 `secret_ref` 是否写成了 key |
| `secret_configured false` | 控制面环境变量/secret 文件未生效，重建/重启 levier |
| `runtime list` 没有目标 Runtime | 容器是否旧镜像，runtime name/instance/token/socket |
| Run 一直 queued | 没有匹配 runtime、容量满、Runtime 未注册 |
| Run completed 但结果不对 | `run show --events` 看工具调用和最终消息 |
| `TOOL_NOT_FOUND` | App 是否在线，工具名是否一致 |
| `MCP_RUNTIME_UNAVAILABLE` | mcp-runtime 是否启动，socket 是否一致 |
| `MCP_CONNECT_FAILED` | 看 `docker compose logs mcp-runtime` |
| `SECRET_MISSING` | MCP 的 `${secret:...}` 是否有对应值 |
| `TOOL_DENIED` | 工具是否在 `allowed_tools` 里 |

---

## 10. 最后记住

```text
App key          -> 读自己的凭据文件
Admin token      -> 管理 App/provider/MCP
Provider key     -> 控制面 secret，不是 App key
Runtime token    -> Runtime 连接控制面的 token
mcp_refs         -> App 声明引用，不是 MCP client
runtime list     -> 已注册 Runtime，不是容器列表
run show         -> 看真实事件和工具调用
```

到这里，最小接入闭环已经完整：

```text
启动 -> 创建凭据 -> 配置 provider -> 启动 App -> 启动 Runtime -> 提交 Run -> 看事件
```
